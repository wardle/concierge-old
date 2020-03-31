// Package cav provides specific integrations for Cardiff and Vale University Health Board
// CAV provides a ("PMS") web service that provides some endpoints, but one of those endpoints
// is simply used as a transport itself for a non-WSDL defined API.
//
// The stubs were generated using https://github.com/hooklift/gowsdl from the WSDL file
package cav

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/wardle/concierge/apiv1"
	"github.com/wardle/concierge/cav/soappms"
	"github.com/wardle/concierge/identifiers"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// PMSService represents the Cardiff and Vale Patient Management System (PMS) service.
// This is thread-safe.
type PMSService struct {
	username string
	password string
	timeout  time.Duration

	tokenMu      sync.RWMutex
	token        string
	tokenExpires time.Time
}

// NewPMSService creates a new (thread-safe) PMS Service with the specified timeout
func NewPMSService(username string, password string, timeout time.Duration) *PMSService {
	if len(username) == 0 || len(password) == 0 {
		log.Fatal("no username / password for CAV PMS service")
	}
	return &PMSService{
		username: username,
		password: password,
		timeout:  timeout,
	}
}

// ResolveIdentifier provides an identifier/value resolution service for CAV CRNs
func (pms *PMSService) ResolveIdentifier(ctx context.Context, id *apiv1.Identifier) (proto.Message, error) {
	if id.GetSystem() != identifiers.CardiffAndValeCRN {
		log.Printf("cav: unable to resolve identifier: incorrect 'system'. expected: '%s' got:'%s'", identifiers.CardiffAndValeCRN, id.GetSystem())
		return nil, fmt.Errorf("unable to resolve identifier: incorrect 'system'. expected: '%s' got:'%s'", identifiers.CardiffAndValeCRN, id.GetSystem())
	}
	return pms.FetchPatient(ctx, id.GetValue())
}

// FetchPatient fetches patient data from the CAV PAS (PMS)
// This query returns multiple rows for a single patient because of the address history
func (pms *PMSService) FetchPatient(ctx context.Context, crn string) (*apiv1.Patient, error) {
	ctx, cancelFunc := context.WithTimeout(ctx, pms.timeout)
	defer cancelFunc()
	token, err := pms.authenticationToken(ctx)
	if err != nil {
		return nil, err
	}
	log.Printf("fetching patient with CRN %s, token: %s", crn, token)
	sql, err := createSQLFetchPatientByCRN(crn)
	if err != nil {
		return nil, err
	}
	pts, err := performSQL(ctx, token, sql)
	if err != nil {
		return nil, err
	}
	if len(pts) == 0 {
		return nil, fmt.Errorf("No patient found with identifier '%s'", crn)
	}
	return parsePatientAndAddresses(pts)
}

// PatientsForClinics returns the patients scheduled for the specified clinics on the specified dates
func (pms *PMSService) PatientsForClinics(ctx context.Context, date time.Time, clinics []*apiv1.Identifier) ([]*apiv1.Patient, error) {
	ctx, cancelFunc := context.WithTimeout(ctx, pms.timeout)
	defer cancelFunc()
	token, err := pms.authenticationToken(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*apiv1.Patient, 0)
	for _, clinicCode := range clinics {
		if clinicCode.GetSystem() != identifiers.CardiffAndValeClinicCode {
			log.Printf("cav: unable fetch clinic patients. invalid system identifier. expected '%s', got: '%s'", identifiers.CardiffAndValeClinicCode, clinicCode.GetSystem())
		}
		sql, err := createSQLFetchPatientsForClinic(clinicCode.GetValue(), date)
		if err != nil {
			return nil, err
		}
		rows, err := performSQL(ctx, token, sql)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			pt, err := parsePatient(row)
			if err != nil {
				log.Printf("cav: failed to parse patient: %+v", pt)
				continue
			}
			result = append(result, pt)
		}
	}
	return result, nil
}

// PublishDocument publishes the document into the CAV document repository
func (pms *PMSService) PublishDocument(ctx context.Context, r *apiv1.PublishDocumentRequest) (*apiv1.PublishDocumentResponse, error) {
	cavID, ok := r.GetPatient().GetIdentifierForSystem(identifiers.CardiffAndValeCRN)
	if !ok {
		log.Printf("cav: unable to publish document '%s|%s' as no CRN identified for Cardiff and Vale", r.GetId().GetSystem(), r.GetId().GetValue())
		return nil, fmt.Errorf("unable to publish document - no valid Cardiff and Vale identifier")
	}
	if r.GetData().GetContentType() != "application/pdf" {
		log.Printf("cav: unable to publish document '%s|%s': wrong content-type expected: 'application/pdf' got: '%s'", r.GetId().GetSystem(), r.GetId().GetValue(), r.GetData().GetContentType())
		return nil, fmt.Errorf("unable to publish document - incorrect content-type '%s'", r.GetData().GetContentType())
	}
	// check that this CRN is correct by fetching against live PAS - basic sanity check in case wrong CRN
	pt, err := pms.FetchPatient(ctx, cavID.GetValue())
	if err != nil {
		return nil, err
	}
	if !proto.Equal(r.GetPatient().GetBirthDate(), pt.GetBirthDate()) || r.GetPatient().GetLastname() != pt.GetLastname() || r.GetPatient().GetGender() != pt.GetGender() {
		log.Printf("cav: unable to publish document '%s|%s': patient details don't match PAS", r.GetId().GetSystem(), r.GetId().GetValue())
		f := protojson.MarshalOptions{Indent: "", Multiline: false}
		log.Printf("cav: request: %s", f.Format(r.GetPatient()))
		log.Printf("cav: pas    : %s", f.Format(pt))
		return nil, errors.New("unable to publish document: patient demographics don't match that in PAS")
	}
	uid := r.GetId().GetSystem() + "|" + r.GetId().GetValue()
	ctx, cancelFunc := context.WithTimeout(ctx, pms.timeout)
	defer cancelFunc()
	docID, err := performReceiveFileByCRN(ctx, cavID.GetValue(), uid, "GENERAL LETTER", r.GetTitle(), r.GetData().GetData())
	if err != nil {
		return nil, err
	}
	return &apiv1.PublishDocumentResponse{Id: &apiv1.Identifier{System: identifiers.CardiffAndValeDocID, Value: docID}}, nil
}

// parseDate parses a CAV PMS date - format is "yyyy/MM/dd"
func parseDate(d string) (*timestamp.Timestamp, error) {
	if len(d) == 0 {
		return nil, nil
	}
	layout := "2006/01/02" // reference date is : Mon Jan 2 15:04:05 MST 2006
	t, err := time.Parse(layout, d)
	if err != nil || t.IsZero() {
		return nil, err
	}
	return ptypes.TimestampProto(t)
}

// parseDate parses a CAV PMS datetime - format is "yyyy/MM/dd hh:mm:ss"
func parseDateTime(d string) (*timestamp.Timestamp, error) {
	if len(d) == 0 {
		return nil, nil
	}
	layout := "2006/01/02 15:04:05" // reference date is : Mon Jan 2 15:04:05 MST 2006
	t, err := time.Parse(layout, d)
	if err != nil || t.IsZero() {
		return nil, err
	}
	return ptypes.TimestampProto(t)
}

// authenticationToken (lazily) returns a valid authentication token
func (pms *PMSService) authenticationToken(ctx context.Context) (string, error) {
	pms.tokenMu.Lock()
	defer pms.tokenMu.Unlock()
	now := time.Now()
	if pms.token != "" && now.Before(pms.tokenExpires) {
		log.Printf("cavpms: using cached authentication token, expires %s", pms.tokenExpires)
		return pms.token, nil
	}
	token, err := authenticate(ctx, pms.username, pms.password)
	if err != nil {
		return "", err
	}
	pms.token = token
	pms.tokenExpires = now.Add(10 * time.Minute)
	log.Printf("cavpms: obtained new authentication token, expires %s", pms.tokenExpires)
	return token, nil
}

// Authenticate authenticates against CAV PMS, returning an authentication token
func authenticate(ctx context.Context, username string, password string) (string, error) {
	lr := &loginRequest{Username: username, Password: password, Database: "vpmslive.world", UserString: "concierge"}
	lrs, err := createLoginRequestXML(lr)
	if err != nil {
		return "", err
	}
	var loginResponse GetDataResponse
	if err := performGetData(ctx, lrs, &loginResponse); err != nil {
		return "", err
	}
	success := loginResponse.Method.Summary.Success
	if success == "true" && loginResponse.Method.Summary.Rowcount == "1" {
		token := loginResponse.Method.Row[0].Column[0].Value
		return token, nil
	}
	log.Printf("cavpms login error: %s", loginResponse.Method.Message)
	return "", status.Error(codes.PermissionDenied, "Could not login to CAV PMS")
}

func performSQL(ctx context.Context, token string, sql string) ([]map[string]string, error) {
	sqlXML, err := createSQLRequestXML(token, sql)
	if err != nil {
		return nil, err
	}
	var sqlResponse GetDataResponse
	if err := performGetData(ctx, sqlXML, &sqlResponse); err != nil {
		return nil, err
	}
	success := sqlResponse.Method.Summary.Success
	if success == "false" {
		log.Printf("cavpms: sql error: %s", sqlResponse.Method.Message)
		return nil, fmt.Errorf("CAV PMS error: %s", sqlResponse.Method.Message)
	}
	count, err := strconv.ParseInt(sqlResponse.Method.Summary.Rowcount, 10, 64)
	if err != nil {
		log.Printf("cavpms: failed to parse rowcount: %s  got:%s", err, sqlResponse)
		return nil, fmt.Errorf("Incorrect format returned from CAV PMS webservice")
	}
	rows := make([]map[string]string, count)
	for i, row := range sqlResponse.Method.Row {
		r := make(map[string]string)
		for _, col := range row.Column {
			r[col.Name] = col.Text
		}
		rows[i] = r
	}
	return rows, nil
}

// performGetData performs a "GetData" operation on the underlying CAV PMS service, which acts
// as a transport for the actual operation, codified within the xmlData
func performGetData(ctx context.Context, xmlData string, result interface{}) error {
	data := &url.Values{
		"XmlDataBlockIn": []string{xmlData},
	}
	endpointURL := "http://cav-wcp02.cardiffandvale.wales.nhs.uk/PmsInterface/WebService/PMSInterfaceWebService.asmx/GetData"
	return performRequest(ctx, endpointURL, data.Encode(), result)
}

// this uses a SOAP call, because the HTTP POST failed to work with base64 encoding for some reason
func performReceiveFileByCRN(ctx context.Context, crn string, uid string, key string, source string, pdfData []byte) (string, error) {
	soap := soappms.NewPMSInterfaceWebServiceSoap("http://cav-wcp02.cardiffandvale.wales.nhs.uk/PmsInterface/WebService/PMSInterfaceWebService.asmx", false, nil)
	fileType := ".pdf"
	data := []byte(base64.StdEncoding.EncodeToString(pdfData))
	response, err := soap.ReceiveFileByCrn(&soappms.ReceiveFileByCrn{
		BfsId:       "test",
		Crn:         crn,
		Key:         key,
		Source:      source,
		FileType:    fileType,
		FileContent: data,
	})
	if err != nil {
		log.Printf("cav: publish document error: %s", err)
		return "", err
	}
	if len(response.ErrorMessage) > 0 {
		return "", fmt.Errorf("error publishing document: %s", response.ErrorMessage)
	}
	return response.DocId, nil
	/*
		data := &url.Values{
			"crn":         []string{crn},                                        // patient CRN
			"bfsId":       []string{uid},                                        // unique document identifier
			"key":         []string{key},                                        // agreed code word - e.g. "GENERAL LETTER"
			"source":      []string{source},                                     // description of document
			"fileContent": []string{base64.StdEncoding.EncodeToString(pdfData)}, // the file data
			"fileType":    []string{".pdf"},                                     // filetype, but an extension, not mimetype
		}
		post := fmt.Sprintf("%s", data.Encode())
		endpointURL := "http://cav-wcp02.cardiffandvale.wales.nhs.uk/PmsInterface/WebService/PMSInterfaceWebService.asmx/ReceiveFileByCrn"
		response := new(AcknowledgementResponse)
		if err := performRequest(ctx, endpointURL, post, &response); err != nil {
			return "", err
		}
		if len(response.ErrorMessage) > 0 {
			return "", fmt.Errorf("PMS webservice error: %s", response.ErrorMessage)
		}
		return response.DocId, nil
	*/
}

func performRequest(ctx context.Context, endpointURL string, post string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "POST", endpointURL, strings.NewReader(post))
	if err != nil {
		log.Printf("error in POST request: %s", err)
		return err
	}
	req.Header.Set("Content-type", "application/x-www-form-urlencoded")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("error publishing document. client.do: %s", err)
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		log.Printf("cav: server error publishing document: %+v", resp)
		log.Printf("body: %v", string(body))
		return errors.New("error publishing document: remote service error")
	}
	return xml.Unmarshal(body, result)
}

type loginRequest struct {
	Username   string
	Password   string
	Database   string
	UserString string
}

func createLoginRequestXML(r *loginRequest) (string, error) {
	var lrXML = `<request><method name="Login"><parameter name="username">{{.Username}}</parameter>
	<parameter name="password">{{.Password}}</parameter>
	<parameter name="database">{{.Database}}</parameter>
	<parameter name="userString">{{.UserString}}</parameter>
	</method></request>`
	t, err := template.New("login-request").Parse(lrXML)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, r); err != nil {
		return "", err
	}
	return string(buf.Bytes()), nil
}

type sqlRequest struct {
	Token   string
	SQLText string
}

// createSQLRequestXML generates the request XML to execute SQL via the GetData webservice endpoint,
// which is, in effect, acting as a transport within a transport with a transport.
func createSQLRequestXML(token string, sqlText string) (string, error) {
	var sqlXML = `<request authenticationToken="{{.Token}}"><method name="SqlTableCall">
	<parameter name="sql"><![CDATA[{{.SQLText}}]]></parameter>
	</method></request>`
	r := sqlRequest{Token: token, SQLText: sqlText}
	t, err := template.New("sql-request").Parse(sqlXML)
	if err != nil {
		return "", nil
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, r); err != nil {
		return "", err
	}
	return string(buf.Bytes()), nil
}

// GetDataResponse is the response from the GetData action
type GetDataResponse struct {
	XMLName xml.Name `xml:"response"`
	Text    string   `xml:",chardata"`
	Method  struct {
		Text    string `xml:",chardata"`
		Name    string `xml:"name,attr"`
		Message string `xml:"message"`
		Summary struct {
			Text     string `xml:",chardata"`
			Success  string `xml:"success,attr"`
			Rowcount string `xml:"rowcount,attr"`
		} `xml:"summary"`
		Row []struct {
			Text   string `xml:",chardata"`
			Column []struct {
				Text  string `xml:",chardata"`
				Name  string `xml:"name,attr"`
				Value string `xml:"value,attr"`
			} `xml:"column"`
		} `xml:"row"`
	} `xml:"method"`
}

// AcknowledgementResponse is the return from a "ReceiveFileByCrn" web service request
type AcknowledgementResponse struct {
	XMLName      xml.Name `xml:"Acknowledgement"`
	Text         string   `xml:",chardata"`
	Xmlns        string   `xml:"xmlns,attr"`
	DocId        string   `xml:"DocId"`
	ErrorMessage string   `xml:"ErrorMessage"`
}

type pmsCRN struct {
	Type string // single digit representing "type" of identifier e.g. "A"
	CRN  string // the actual identifier e.g. "123456"
}

// a CRN is of the format A123456 or A123456X, where X is an optional check digit
func parseCRN(crn string) (*pmsCRN, error) {
	crn = strings.ToUpper(crn)
	switch len(crn) {
	case 8:
		crn = crn[0:6]
		fallthrough
	case 7:
		return &pmsCRN{Type: string(crn[0]), CRN: crn[1:7]}, nil
	default:
		return nil, fmt.Errorf("Invalid CRN: '%s'", crn)
	}
}

func createSQLFetchPatientByCRN(crn string) (string, error) {
	params, err := parseCRN(crn)
	if err != nil {
		return "", err
	}
	t, err := template.New("sql-patient-by-crn").Parse(sqlFetchPatientByCRN)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, params); err != nil {
		return "", err
	}
	return string(buf.Bytes()), nil
}

var sqlFetchPatientByCRN = `SELECT People.ID, NHS_NO AS NHS_NUMBER, 
to_char(DATE_LAST_CHANGED, 'yyyy/mm/dd hh:mi:ss') as DATE_LAST_MODIFIED,
PATIENT_IDENTIFIERS.PAID_TYPE || PATIENT_IDENTIFIERS.ID as HOSPITAL_ID, 
TITLE, People.SURNAME AS LAST_NAME, People.FIRST_FORENAME, People.SECOND_FORENAME, OTHER_FORENAMES, 
SEX, to_char(DOB,'yyyy/mm/dd') AS DATE_BIRTH, to_char(DOD,'yyyy/mm/dd') AS DATE_DEATH,
HOME_PHONE_NO, WORK_PHONE_NO, 
ADDRESS1,ADDRESS2,ADDRESS3,ADDRESS4, POSTCODE, 
to_char(LOCATIONS.DATE_FROM, 'yyyy/mm/dd') as DATE_FROM,
to_char(LOCATIONS.DATE_TO, 'yyyy/mm/dd') as DATE_TO, 
COUNTRY_OF_BIRTH, ETHNIC_ORIGIN, MARITAL_STATUS, OCCUPATION, PLACE_OF_BIRTH, PLACE_OF_DEATH, 
HEALTHCARE_PRACTITIONERS.national_no AS GP_ID, 
EXTERNAL_ORGANISATIONS.national_no AS GPPR_ID
FROM	EXTERNAL_ORGANISATIONS, HEALTHCARE_PRACTITIONERS, LOCATIONS, PEOPLE, PATIENT_IDENTIFIERS
WHERE	PATIENT_IDENTIFIERS.PAID_TYPE = '{{.Type}}'
AND PATIENT_IDENTIFIERS.ID = '{{.CRN}}'
AND PATIENT_IDENTIFIERS.CRN = 'Y'
AND PATIENT_IDENTIFIERS.MAJOR_FLAG = 'Y'
AND PEOPLE.ID = PATIENT_IDENTIFIERS.PATI_ID
AND LOCATIONS.ORGA_PERS_ID (+) = PEOPLE.ID
AND HEALTHCARE_PRACTITIONERS.PERS_ID (+) = PEOPLE.GP_ID
AND EXTERNAL_ORGANISATIONS.ID (+) = PEOPLE.GPPR_ID
ORDER BY LOCATIONS.DATE_FROM DESC`

func parsePatientAndAddresses(rows []map[string]string) (*apiv1.Patient, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	pt, err := parsePatient(rows[0])
	if err != nil {
		return nil, err
	}
	pt.Addresses = make([]*apiv1.Address, 0)
	for _, row := range rows {
		address := new(apiv1.Address)
		address.Address1 = row["ADDRESS1"]
		address.Address2 = row["ADDRESS2"]
		address.Address3 = row["ADDRESS3"]
		address.Country = row["ADDRESS4"]
		address.Postcode = row["POSTCODE"]
		from, _ := parseDate(row["DATE_FROM"])
		to, _ := parseDate(row["DATE_TO"])
		address.Period = &apiv1.Period{Start: from, End: to}
		pt.Addresses = append(pt.Addresses, address)
	}
	return pt, nil
}

func parsePatient(row map[string]string) (*apiv1.Patient, error) {
	pt := new(apiv1.Patient)
	pt.Lastname = row["LAST_NAME"]
	firstNames := make([]string, 0)
	if len(row["FIRST_FORENAME"]) > 0 {
		firstNames = append(firstNames, row["FIRST_FORENAME"])
	}
	if len(row["SECOND_FORENAME"]) > 0 {
		firstNames = append(firstNames, row["SECOND_FORENAME"])
	}
	if len(row["OTHER_FORENAMES"]) > 0 {
		firstNames = append(firstNames, row["OTHER_FORENAMES"])
	}
	pt.Firstnames = strings.Join(firstNames, " ")
	switch row["SEX"] {
	case "M":
		pt.Gender = apiv1.Gender_MALE
	case "F":
		pt.Gender = apiv1.Gender_FEMALE
	default:
		pt.Gender = apiv1.Gender_UNKNOWN
	}
	var err error
	pt.BirthDate, err = parseDate(row["DATE_BIRTH"])
	if err != nil {
		return nil, err
	}
	dateDeath, err := parseDate(row["DATE_DEATH"])
	if err != nil {
		return nil, err
	}
	if dateDeath != nil {
		pt.Deceased = &apiv1.Patient_DeceasedDate{DeceasedDate: dateDeath}
	}
	pt.Identifiers = make([]*apiv1.Identifier, 0)
	pt.Identifiers = append(pt.Identifiers, &apiv1.Identifier{System: identifiers.CardiffAndValeCRN, Value: row["HOSPITAL_ID"]})
	if nnn := row["NHS_NUMBER"]; len(nnn) > 0 {
		pt.Identifiers = append(pt.Identifiers, &apiv1.Identifier{System: identifiers.NHSNumber, Value: nnn})
	}
	pt.Title = row["TITLE"]
	pt.Telephones = make([]*apiv1.Telephone, 0)
	if tel := row["HOME_PHONE_NO"]; len(tel) > 0 {
		pt.Telephones = append(pt.Telephones, &apiv1.Telephone{Number: tel, Description: "Home"})
	}
	if tel := row["WORK_PHONE_NO"]; len(tel) > 0 {
		pt.Telephones = append(pt.Telephones, &apiv1.Telephone{Number: tel, Description: "Work"})
	}
	pt.GeneralPractitioner = row["GP_ID"]
	pt.Surgery = row["GPPR_ID"]
	return pt, nil
}

type patientsForClinic struct {
	ClinicCode string
	DateString string
}

func createSQLFetchPatientsForClinic(clinicCode string, date time.Time) (string, error) {
	params := &patientsForClinic{
		ClinicCode: clinicCode,
		DateString: date.Format("2006/01/02"),
	}
	t, err := template.New("sql-patients-for-clinic").Parse(sqlFetchPatientsForClinic)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, params); err != nil {
		return "", err
	}
	return string(buf.Bytes()), nil
}

var sqlFetchPatientsForClinic = `SELECT People.ID, NHS_NO AS NHS_NUMBER,
to_char(DATE_LAST_CHANGED, 'yyyy/mm/dd hh:mi:ss') as
DATE_LAST_MODIFIED,
PATIENT_IDENTIFIERS.PAID_TYPE ||
PATIENT_IDENTIFIERS.ID as HOSPITAL_ID, 
TITLE, People.SURNAME AS LAST_NAME, 
People.FIRST_FORENAME, People.SECOND_FORENAME, OTHER_FORENAMES, 
SEX,
to_char(DOB,'yyyy/mm/dd') AS DATE_BIRTH,
to_char(DOD,'yyyy/mm/dd') AS DATE_DEATH,
HOME_PHONE_NO, WORK_PHONE_NO,
ADDRESS1,ADDRESS2,ADDRESS3,ADDRESS4, POSTCODE,
to_char(LOCATIONS.DATE_FROM, 'yyyy/mm/dd') as DATE_FROM,
to_char(LOCATIONS.DATE_TO, 'yyyy/mm/dd') as DATE_TO, 
GP_ID, GPPR_ID, COUNTRY_OF_BIRTH, ETHNIC_ORIGIN,
MARITAL_STATUS, OCCUPATION,
PLACE_OF_BIRTH, PLACE_OF_DEATH,
HEALTHCARE_PRACTITIONERS.national_no AS GP_ID,
EXTERNAL_ORGANISATIONS.national_no AS GPPR_ID
FROM EXTERNAL_ORGANISATIONS,
HEALTHCARE_PRACTITIONERS, LOCATIONS, PEOPLE,
PATIENT_IDENTIFIERS, BOOKED_SLOTS, ACT_CLIN_SESSIONS,
OUTPATIENT_CLINICS
WHERE OUTPATIENT_CLINICS.SHORTNAME = '{{.ClinicCode}}'
AND ACT_CLIN_SESSIONS.OUCL_ID = OUTPATIENT_CLINICS.OUCL_ID
AND ACT_CLIN_SESSIONS.SESSION_DATE = To_Date('{{.DateString}}', 'yyyy/mm/dd')
AND ACT_CLIN_SESSIONS.DATE_CANCD IS NULL
AND BOOKED_SLOTS.ACS_ID = ACT_CLIN_SESSIONS.ACS_ID
AND PATIENT_IDENTIFIERS.PATI_ID = BOOKED_SLOTS.PATI_ID
AND PATIENT_IDENTIFIERS.CRN = 'Y'
AND PATIENT_IDENTIFIERS.MAJOR_FLAG = 'Y'
AND PEOPLE.ID = PATIENT_IDENTIFIERS.PATI_ID
AND LOCATIONS.ORGA_PERS_ID (+) = PEOPLE.ID
AND LOCATIONS.DATE_TO (+) IS NULL
AND HEALTHCARE_PRACTITIONERS.PERS_ID (+) = PEOPLE.GP_ID
AND EXTERNAL_ORGANISATIONS.ID (+) = PEOPLE.GPPR_ID
ORDER BY LAST_NAME`
