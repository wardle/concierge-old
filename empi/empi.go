// Package empi provides a lightweight wrapper around the NHS Wales' EMPI service
package empi

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"net/url"
	"strings"
	"text/template"
	"time"

	"github.com/wardle/concierge/apiv1"

	"github.com/patrickmn/go-cache"
)

// Endpoint represents a specific SOAP server providing access to "enterprise master patient index" (EMPI) data
type Endpoint int

// A list of endpoints
const (
	UnknownEndpoint     Endpoint = iota // unknown
	ProductionEndpoint                  // production server
	TestingEndpoint                     // user acceptance testing
	DevelopmentEndpoint                 // development
)

var endpointURLs = [...]string{
	"",
	"https://mpilivequeries.cymru.nhs.uk/PatientDemographicsQueryWS.asmx",
	"https://mpitest.cymru.nhs.uk/PatientDemographicsQueryWS.asmx",
	"http://ndc06srvmpidev2.cymru.nhs.uk:23000/PatientDemographicsQueryWS.asmx",
}

var endpointNames = [...]string{
	"unknown",
	"production",
	"testing",
	"development",
}

var endpointCodes = [...]string{
	"",
	"P",
	"U",
	"T",
}

// LookupEndpoint returns an endpoint for (P)roduction, (T)esting or (D)evelopment
func LookupEndpoint(s string) Endpoint {
	s2 := strings.ToUpper(s)
	switch {
	case strings.HasPrefix(s2, "P"):
		return ProductionEndpoint
	case strings.HasPrefix(s2, "T"):
		return TestingEndpoint
	case strings.HasPrefix(s2, "D"):
		return DevelopmentEndpoint
	}
	return UnknownEndpoint
}

// URL returns the default URL of this endpoint
func (ep Endpoint) URL() string {
	return endpointURLs[ep]
}

// ProcessingID returns the processing ID for this endpoint
func (ep Endpoint) ProcessingID() string {
	return endpointCodes[ep]
}

// Name returns the name of this endpoint
func (ep Endpoint) Name() string {
	return endpointNames[ep]
}

// Invoke invokes a simple request on the endpoint for the specified authority and identifier
func Invoke(endpointURL string, processingID string, empiOrgCode string, identifier string) {
	ctx := context.Background()
	auth := lookupFromEmpiOrgCode(empiOrgCode)
	if auth == AuthorityUnknown {
		log.Fatalf("empi: unsupported authority: %s", empiOrgCode)
	}
	pt, err := performRequest(ctx, endpointURL, processingID, auth, identifier)
	if err != nil {
		log.Fatal(err)
	}
	if pt == nil {
		log.Printf("empi: patient %s/%s not found", empiOrgCode, identifier)
		return
	}
	fmt.Print(protojson.Format(pt))
}

// App represents the EMPI application
type App struct {
	Endpoint       Endpoint
	EndpointURL    string       // override URL for the specified endpoint
	Cache          *cache.Cache // may be nil if not caching
	Fake           bool
	TimeoutSeconds int
}

// ResolveIdentifier provides an identifier/value resolution service
func (app *App) ResolveIdentifier(ctx context.Context, id *apiv1.Identifier) (proto.Message, error) {
	return app.GetEMPIRequest(ctx, id)
}

// GetEMPIRequest fetches a patient matching the identifier specified
func (app *App) GetEMPIRequest(ctx context.Context, req *apiv1.Identifier) (*apiv1.Patient, error) {
	var email string
	if headers, ok := metadata.FromIncomingContext(ctx); ok {
		emails := headers["from"]
		if len(emails) > 0 {
			email = emails[0]
		}
	}
	authority, ok := uriLookup[req.System]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "invalid authority: %s", req.System)
	}
	empiCode := authority.empiOrganisationCode()
	log.Printf("empi: request from '%s' for %s/%s - mapped to %d (%s)", email, req.System, req.Value, authority, empiCode)

	if empiCode == "" {
		return nil, status.Errorf(codes.InvalidArgument, "unsupported authority: %s (%s)", req.System, authority)
	}
	return app.GetInternalEMPIRequest(ctx, &apiv1.Identifier{
		System: authority.empiOrganisationCode(),
		Value:  req.Value,
	})
}

// GetInternalEMPIRequest fetches a patient using raw authority and identifier codes
func (app *App) GetInternalEMPIRequest(ctx context.Context, req *apiv1.Identifier) (*apiv1.Patient, error) {
	start := time.Now()
	key := req.System + "/" + req.Value
	pt, found := app.getCache(key)
	if found {
		log.Printf("empi: serving request for %s/%s from cache in %s", req.System, req.Value, time.Since(start))
		return pt, nil
	}
	authority := lookupFromEmpiOrgCode(req.System)
	if authority == AuthorityUnknown {
		log.Printf("empi: unsupported authority: %s", req.System)
		return nil, status.Errorf(codes.InvalidArgument, "unsupported authority: %s", req.System)
	}
	var valid bool
	if valid, req.Value = authority.ValidateIdentifier(req.Value); !valid {
		return nil, status.Errorf(codes.InvalidArgument, "invalid %s number: %s", req.System, req.Value)
	}
	if app.Fake {
		log.Printf("empi: returning fake result for %s/%s", req.System, req.Value)
		return performFake(authority, req.Value)
	}
	ctx, cancelFunc := context.WithTimeout(ctx, time.Duration(app.TimeoutSeconds)*time.Second)
	pt, err := performRequest(ctx, app.EndpointURL, app.Endpoint.ProcessingID(), authority, req.Value)
	cancelFunc()
	if err != nil {
		if urlError, ok := err.(*url.Error); ok {
			if urlError.Timeout() {
				return nil, status.Errorf(codes.DeadlineExceeded, "NHS Wales' EMPI service did not respond within deadline (%d sec)", app.TimeoutSeconds)
			}
		}
		return nil, err
	}
	if pt == nil {
		return nil, status.Errorf(codes.NotFound, "patient %s/%s not found", req.System, req.Value)
	}
	log.Printf("empi: response for %s: %s", req.Value, protojson.MarshalOptions{}.Format(pt))
	return pt, nil
}

func (app *App) getCache(key string) (*apiv1.Patient, bool) {
	if app.Cache == nil {
		return nil, false
	}
	if o, found := app.Cache.Get(key); found {
		return o.(*apiv1.Patient), true
	}
	return nil, false
}

func (app *App) setCache(key string, value *apiv1.Patient) {
	if app.Cache == nil {
		return
	}
	app.Cache.Set(key, value, cache.DefaultExpiration)
}

func performFake(authority Authority, identifier string) (*apiv1.Patient, error) {
	dob, err := ptypes.TimestampProto(time.Date(1960, 01, 01, 00, 00, 00, 0, time.UTC))
	if err != nil {
		return nil, err
	}
	return &apiv1.Patient{
		Lastname:            "DUMMY",
		Firstnames:          "ALBERT",
		Title:               "DR",
		Gender:              apiv1.Gender_MALE,
		BirthDate:           dob,
		Surgery:             "W95010",
		GeneralPractitioner: "G9342400",
		Identifiers: []*apiv1.Identifier{
			&apiv1.Identifier{
				System: authority.empiOrganisationCode(),
				Value:  identifier,
			},
			&apiv1.Identifier{
				System: "103",
				Value:  "M1147907",
			},
			&apiv1.Identifier{
				System: CardiffAndValeURI,
				Value:  "X234567",
			},
		},

		Addresses: []*apiv1.Address{
			&apiv1.Address{
				Address1: "59 Robins Hill",
				Address2: "Brackla",
				Address3: "Bridgend",
				Postcode: "CF31 2PJ",
				Country:  "WALES",
			},
		},
		Telephones: []*apiv1.Telephone{
			&apiv1.Telephone{
				Number:      "02920747747",
				Description: "Home",
			},
		},
		Emails: []string{"test@test.com"},
	}, nil
}

func performRequest(context context.Context, endpointURL string, processingID string, authority Authority, identifier string) (*apiv1.Patient, error) {
	start := time.Now()
	data, err := NewIdentifierRequest(strings.ToUpper(identifier), authority, "221", "100", processingID)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(context, "POST", endpointURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-type", "text/xml; charset=\"utf-8\"")
	req.Header.Set("SOAPAction", "http://apps.wales.nhs.uk/mpi/InvokePatientDemographicsQuery")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var e envelope
	log.Printf("empi: response (%s): %v", time.Since(start), string(body))
	err = xml.Unmarshal(body, &e)
	if err != nil {
		return nil, err
	}
	return e.ToPatient()
}

// IdentifierRequest is used to populate the template to make the XML request
type IdentifierRequest struct {
	Identifier           string
	Authority            string
	AuthorityType        string
	SendingApplication   string
	SendingFacility      string
	ReceivingApplication string
	ReceivingFacility    string
	DateTime             string
	MessageControlID     string //for MSH.10 -  a UUID
	ProcessingID         string //for MSH.11 - P/U/T production/testing/development
}

// NewIdentifierRequest returns a correctly formatted XML request to search by an identifier, such as NHS number
// sender : 221 (PatientCare)
// receiver: 100 (NHS Wales EMPI)
func NewIdentifierRequest(identifier string, authority Authority, sender string, receiver string, processingID string) ([]byte, error) {
	layout := "20060102150405" // YYYYMMDDHHMMSS
	now := time.Now().Format(layout)
	data := IdentifierRequest{
		Identifier:           identifier,
		Authority:            authority.empiOrganisationCode(),
		AuthorityType:        authority.typeCode(),
		SendingApplication:   sender,
		SendingFacility:      sender,
		ReceivingApplication: receiver,
		ReceivingFacility:    receiver,
		DateTime:             now,
		MessageControlID:     uuid.New().String(),
		ProcessingID:         processingID,
	}
	t, err := template.New("identifier-request").Parse(identifierRequestTemplate)
	if err != nil {
		return nil, err
	}
	log.Printf("empi request: %+v", data)
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ToPatient creates a "Patient" from the XML returned from the EMPI service
func (e *envelope) ToPatient() (*apiv1.Patient, error) {
	pt := new(apiv1.Patient)
	pt.Lastname = e.surname()
	pt.Firstnames = e.firstnames()
	if pt.Lastname == "" && pt.Firstnames == "" {
		return nil, nil
	}
	pt.Title = e.title()
	switch e.gender() {
	case "M":
		pt.Gender = apiv1.Gender_MALE
	case "F":
		pt.Gender = apiv1.Gender_FEMALE
	default:
		pt.Gender = apiv1.Gender_UNKNOWN
	}
	pt.BirthDate = e.dateBirth()
	if dd := e.dateDeath(); dd != nil {
		pt.Deceased = &apiv1.Patient_DeceasedDate{DeceasedDate: dd}
	}
	pt.Identifiers = e.identifiers()
	pt.Addresses = e.addresses()
	pt.Surgery = e.surgery()
	pt.GeneralPractitioner = e.generalPractitioner()
	pt.Telephones = e.telephones()
	pt.Emails = e.emails()
	return pt, nil
}

func (e *envelope) surname() string {
	names := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID5
	if len(names) > 0 {
		return names[0].XPN1.FN1.Text
	}
	return ""
}

func (e *envelope) firstnames() string {
	names := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID5
	var sb strings.Builder
	if len(names) > 0 {
		sb.WriteString(names[0].XPN2.Text) // given name - XPN.2
		sb.WriteString(" ")
		sb.WriteString(names[0].XPN3.Text) // second or further given names - XPN.3
	}
	return strings.TrimSpace(sb.String())
}

func (e *envelope) title() string {
	names := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID5
	if len(names) > 0 {
		return names[0].XPN5.Text
	}
	return ""
}

func (e *envelope) gender() string {
	return e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID8.Text
}

func (e *envelope) dateBirth() *timestamp.Timestamp {
	dob := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID7.TS1.Text
	if len(dob) > 0 {
		d, err := parseDate(dob)
		if err == nil {
			return d
		}
	}
	return nil
}

func (e *envelope) dateDeath() *timestamp.Timestamp {
	dod := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID29.TS1.Text
	if len(dod) > 0 {
		d, err := parseDate(dod)
		if err == nil {
			return d
		}
	}
	return nil
}

func (e *envelope) surgery() string {
	return e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PD1.PD13.XON3.Text
}

func (e *envelope) generalPractitioner() string {
	return e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PD1.PD14.XCN1.Text
}

func (e *envelope) identifiers() []*apiv1.Identifier {
	result := make([]*apiv1.Identifier, 0)
	ids := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID3
	for _, id := range ids {
		authority := id.CX4.HD1.Text
		identifier := id.CX1.Text
		if authority != "" && identifier != "" {
			system := authority
			if a := lookupFromEmpiOrgCode(system); a.ToURI() != "" {
				system = a.ToURI()
			}
			result = append(result, &apiv1.Identifier{
				System: system,
				Value:  identifier,
			})
		}
	}
	return result
}

func (e *envelope) addresses() []*apiv1.Address {
	result := make([]*apiv1.Address, 0)
	addresses := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID11
	for _, address := range addresses {
		dateFrom, _ := parseDate(address.XAD13.Text)
		dateTo, _ := parseDate(address.XAD14.Text)
		result = append(result, &apiv1.Address{
			Address1: address.XAD1.SAD1.Text,
			Address2: address.XAD2.Text,
			Address3: address.XAD3.Text,
			Country:  address.XAD4.Text,
			Postcode: address.XAD5.Text,
			Period: &apiv1.Period{
				Start: dateFrom,
				End:   dateTo,
			},
		})
	}
	return result
}

func (e *envelope) telephones() []*apiv1.Telephone {
	result := make([]*apiv1.Telephone, 0)
	pid13 := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID13
	for _, telephone := range pid13 {
		num := telephone.XTN1.Text
		if num != "" {
			result = append(result, &apiv1.Telephone{
				Number:      num,
				Description: telephone.LongName,
			})
		}
	}
	pid14 := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID14
	for _, telephone := range pid14 {
		num := telephone.XTN1.Text
		if num != "" {
			result = append(result, &apiv1.Telephone{
				Number:      num,
				Description: telephone.LongName,
			})
		}
	}
	return result
}

// sanity check for emails
var rxEmail = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+\\/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

func (e *envelope) emails() []string {
	result := make([]string, 0)
	pid13 := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID13
	for _, telephone := range pid13 {
		email := telephone.XTN4.Text
		if email != "" && len(email) < 255 && rxEmail.MatchString(email) {
			result = append(result, email)
		}
	}
	pid14 := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID14
	for _, telephone := range pid14 {
		email := telephone.XTN4.Text
		if email != "" && len(email) < 255 && rxEmail.MatchString(email) {
			result = append(result, email)
		}
	}
	return result
}

func parseDate(d string) (*timestamp.Timestamp, error) {
	layout := "20060102" // reference date is : Mon Jan 2 15:04:05 MST 2006
	if len(d) > 8 {
		d = d[:8]
	}
	t, err := time.Parse(layout, d)
	if err != nil || t.IsZero() {
		return nil, err
	}
	return ptypes.TimestampProto(t)
}

var identifierRequestTemplate = `
<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:mpi="http://apps.wales.nhs.uk/mpi/" xmlns="urn:hl7-org:v2xml">
<soapenv:Header/>
<soapenv:Body>
   <mpi:InvokePatientDemographicsQuery>

	  <QBP_Q21>

		 <MSH>
			 <!--Field Separator -->
			<MSH.1>|</MSH.1>
			<!-- Encoding Characters -->
			<MSH.2>^~\&amp;</MSH.2>
			<!-- Sending Application -->
			<MSH.3 >
			   <HD.1>{{.SendingApplication}}</HD.1>
			</MSH.3>
			<!-- Sending Facility -->
			<MSH.4 >
			   <HD.1>{{.SendingFacility}}</HD.1>
			</MSH.4>
			<!-- Receiving Application -->
			<MSH.5>
			   <HD.1>{{.ReceivingApplication}}</HD.1>
			</MSH.5>
			<!-- Receiving Application -->
			<MSH.6>
			   <HD.1>{{.ReceivingFacility}}</HD.1>
			</MSH.6>
			<!-- Date / Time of message YYYYMMDDHHMMSS -->
			<MSH.7>
			   <TS.1>{{.DateTime}}</TS.1>
			</MSH.7>
			<!-- Message Type -->
			<MSH.9>
			   <MSG.1 >QBP</MSG.1>
			   <MSG.2 >Q22</MSG.2>
			   <MSG.3 >QBP_Q21</MSG.3>
			</MSH.9>
			<!-- Message Control ID -->
			<MSH.10>{{.MessageControlID}}</MSH.10>
			<MSH.11>
			   <PT.1 >{{.ProcessingID}}</PT.1>
			</MSH.11>
			<!-- Version Id -->
			<MSH.12>
			   <VID.1 >2.5</VID.1>
			</MSH.12>
			<!-- Country Code -->
			<MSH.17 >GBR</MSH.17>
		 </MSH>

		 <QPD>
			<QPD.1 >
			   <!--Message Query Name :-->
			   <CE.1>IHE PDQ Query</CE.1>
			</QPD.1>
			<!--Query Tag:-->
			<QPD.2>PatientQuery</QPD.2>
		  <!--Demographic Fields:-->
			<!--Zero or more repetitions:-->
			<QPD.3>
			   <!--PID.3 - Patient Identifier List:-->
			   <QIP.1>@PID.3.1</QIP.1>
			   <QIP.2>{{.Identifier}}</QIP.2>
			</QPD.3>
			<QPD.3>
			   <!--PID.3 - Patient Identifier List:-->
			   <QIP.1>@PID.3.4</QIP.1>
			   <QIP.2>{{.Authority}}</QIP.2>
			</QPD.3>
			<QPD.3>
			   <!--PID.3 - Patient Identifier List:-->
			   <QIP.1>@PID.3.5</QIP.1>
			   <QIP.2>{{.AuthorityType}}</QIP.2>
			</QPD.3>
		 </QPD>

		 <RCP>
			<!--Query Priority:-->
			<RCP.1 >I</RCP.1>
			<!--Quantity Limited Request:-->
			<RCP.2 >
			   <CQ.1>50</CQ.1>
			</RCP.2>

		 </RCP>

	  </QBP_Q21>
   </mpi:InvokePatientDemographicsQuery>
</soapenv:Body>
</soapenv:Envelope>
`

// envelope is a struct generated by https://www.onlinetool.io/xmltogo/ from the XML returned from the server.
// However, this doesn't take into account the possibility of repeating fields for certain PID entries.
// See https://hl7-definition.caristix.com/v2/HL7v2.5.1/Segments/PID
// which documents that the following can be repeated: PID3 PID4 PID5 PID6 PID9 PID10 PID11 PID13 PID14 PID21 PID22 PID26 PID32
// Therefore, these have been manually added as []struct rather than struct.
// Also, added PID.29 for date of death
type envelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Text    string   `xml:",chardata"`
	Soap    string   `xml:"soap,attr"`
	Xsi     string   `xml:"xsi,attr"`
	Xsd     string   `xml:"xsd,attr"`
	Body    struct {
		Text                                   string `xml:",chardata"`
		InvokePatientDemographicsQueryResponse struct {
			Text   string `xml:",chardata"`
			Xmlns  string `xml:"xmlns,attr"`
			RSPK21 struct {
				Text  string `xml:",chardata"`
				Xmlns string `xml:"xmlns,attr"`
				MSH   struct {
					Text string `xml:",chardata"`
					MSH1 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						LongName string `xml:"LongName,attr"`
					} `xml:"MSH.1"`
					MSH2 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						LongName string `xml:"LongName,attr"`
					} `xml:"MSH.2"`
					MSH3 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						Table    string `xml:"Table,attr"`
						LongName string `xml:"LongName,attr"`
						HD1      struct {
							Text     string `xml:",chardata"`
							Type     string `xml:"Type,attr"`
							Table    string `xml:"Table,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"HD.1"`
					} `xml:"MSH.3"`
					MSH4 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						Table    string `xml:"Table,attr"`
						LongName string `xml:"LongName,attr"`
						HD1      struct {
							Text     string `xml:",chardata"`
							Type     string `xml:"Type,attr"`
							Table    string `xml:"Table,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"HD.1"`
					} `xml:"MSH.4"`
					MSH5 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						Table    string `xml:"Table,attr"`
						LongName string `xml:"LongName,attr"`
						HD1      struct {
							Text     string `xml:",chardata"`
							Type     string `xml:"Type,attr"`
							Table    string `xml:"Table,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"HD.1"`
					} `xml:"MSH.5"`
					MSH6 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						Table    string `xml:"Table,attr"`
						LongName string `xml:"LongName,attr"`
						HD1      struct {
							Text     string `xml:",chardata"`
							Type     string `xml:"Type,attr"`
							Table    string `xml:"Table,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"HD.1"`
					} `xml:"MSH.6"`
					MSH7 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						LongName string `xml:"LongName,attr"`
						TS1      struct {
							Text     string `xml:",chardata"`
							Type     string `xml:"Type,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"TS.1"`
					} `xml:"MSH.7"`
					MSH9 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						LongName string `xml:"LongName,attr"`
						MSG1     struct {
							Text     string `xml:",chardata"`
							Type     string `xml:"Type,attr"`
							Table    string `xml:"Table,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"MSG.1"`
						MSG2 struct {
							Text     string `xml:",chardata"`
							Type     string `xml:"Type,attr"`
							Table    string `xml:"Table,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"MSG.2"`
						MSG3 struct {
							Text     string `xml:",chardata"`
							Type     string `xml:"Type,attr"`
							Table    string `xml:"Table,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"MSG.3"`
					} `xml:"MSH.9"`
					MSH10 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						LongName string `xml:"LongName,attr"`
					} `xml:"MSH.10"`
					MSH11 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						LongName string `xml:"LongName,attr"`
						PT1      struct {
							Text     string `xml:",chardata"`
							Type     string `xml:"Type,attr"`
							Table    string `xml:"Table,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"PT.1"`
					} `xml:"MSH.11"`
					MSH12 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						LongName string `xml:"LongName,attr"`
						VID1     struct {
							Text     string `xml:",chardata"`
							Type     string `xml:"Type,attr"`
							Table    string `xml:"Table,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"VID.1"`
					} `xml:"MSH.12"`
					MSH17 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						Table    string `xml:"Table,attr"`
						LongName string `xml:"LongName,attr"`
					} `xml:"MSH.17"`
					MSH19 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						LongName string `xml:"LongName,attr"`
						CE1      struct {
							Text     string `xml:",chardata"`
							Type     string `xml:"Type,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"CE.1"`
					} `xml:"MSH.19"`
				} `xml:"MSH"`
				MSA struct {
					Text string `xml:",chardata"`
					MSA1 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						Table    string `xml:"Table,attr"`
						LongName string `xml:"LongName,attr"`
					} `xml:"MSA.1"`
					MSA2 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						LongName string `xml:"LongName,attr"`
					} `xml:"MSA.2"`
				} `xml:"MSA"`
				QAK struct {
					Text string `xml:",chardata"`
					QAK1 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						LongName string `xml:"LongName,attr"`
					} `xml:"QAK.1"`
					QAK2 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						Table    string `xml:"Table,attr"`
						LongName string `xml:"LongName,attr"`
					} `xml:"QAK.2"`
				} `xml:"QAK"`
				QPD struct {
					Text string `xml:",chardata"`
					QPD1 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						Table    string `xml:"Table,attr"`
						LongName string `xml:"LongName,attr"`
						CE1      struct {
							Text     string `xml:",chardata"`
							Type     string `xml:"Type,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"CE.1"`
					} `xml:"QPD.1"`
					QPD2 struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						LongName string `xml:"LongName,attr"`
					} `xml:"QPD.2"`
					QPD3 []struct {
						Text     string `xml:",chardata"`
						Item     string `xml:"Item,attr"`
						Type     string `xml:"Type,attr"`
						LongName string `xml:"LongName,attr"`
						QIP1     struct {
							Text     string `xml:",chardata"`
							Type     string `xml:"Type,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"QIP.1"`
						QIP2 struct {
							Text     string `xml:",chardata"`
							Type     string `xml:"Type,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"QIP.2"`
					} `xml:"QPD.3"`
				} `xml:"QPD"`
				RSPK21QUERYRESPONSE struct {
					Text string `xml:",chardata"`
					PID  struct {
						Text string `xml:",chardata"`
						PID1 struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"PID.1"`
						PID3 []struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							LongName string `xml:"LongName,attr"`
							CX1      struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"CX.1"`
							CX4 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								Table    string `xml:"Table,attr"`
								LongName string `xml:"LongName,attr"`
								HD1      struct {
									Text     string `xml:",chardata"`
									Type     string `xml:"Type,attr"`
									Table    string `xml:"Table,attr"`
									LongName string `xml:"LongName,attr"`
								} `xml:"HD.1"`
							} `xml:"CX.4"`
							CX5 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								Table    string `xml:"Table,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"CX.5"`
						} `xml:"PID.3"`
						PID5 []struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							LongName string `xml:"LongName,attr"`
							XPN1     struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
								FN1      struct {
									Text     string `xml:",chardata"`
									Type     string `xml:"Type,attr"`
									LongName string `xml:"LongName,attr"`
								} `xml:"FN.1"`
							} `xml:"XPN.1"`
							XPN2 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XPN.2"`
							XPN3 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XPN.3"`
							XPN5 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XPN.5"`
							XPN7 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								Table    string `xml:"Table,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XPN.7"`
						} `xml:"PID.5"`
						PID7 struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							LongName string `xml:"LongName,attr"`
							TS1      struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"TS.1"`
						} `xml:"PID.7"`
						PID8 struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							Table    string `xml:"Table,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"PID.8"`
						PID9 []struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							LongName string `xml:"LongName,attr"`
							XPN7     struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								Table    string `xml:"Table,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XPN.7"`
						} `xml:"PID.9"`
						PID11 []struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							LongName string `xml:"LongName,attr"`
							XAD1     struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
								SAD1     struct {
									Text     string `xml:",chardata"`
									Type     string `xml:"Type,attr"`
									LongName string `xml:"LongName,attr"`
								} `xml:"SAD.1"`
							} `xml:"XAD.1"`
							XAD2 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XAD.2"`
							XAD3 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XAD.3"`
							XAD4 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XAD.4"`
							XAD5 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XAD.5"`
							XAD7 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								Table    string `xml:"Table,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XAD.7"`
							XAD13 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								Table    string `xml:"Table,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XAD.13"`
							XAD14 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								Table    string `xml:"Table,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XAD.14"`
						} `xml:"PID.11"`
						PID13 []struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							LongName string `xml:"LongName,attr"`
							XTN1     struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XTN.1"`
							XTN2 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								Table    string `xml:"Table,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XTN.2"`
							XTN4 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XTN.4"`
						} `xml:"PID.13"`
						PID14 []struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							LongName string `xml:"LongName,attr"`
							XTN1     struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XTN.1"`
							XTN2 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								Table    string `xml:"Table,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XTN.2"`
							XTN4 struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XTN.4"`
						} `xml:"PID.14"`
						PID15 struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							Table    string `xml:"Table,attr"`
							LongName string `xml:"LongName,attr"`
							CE1      struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"CE.1"`
						} `xml:"PID.15"`
						PID16 struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							Table    string `xml:"Table,attr"`
							LongName string `xml:"LongName,attr"`
							CE1      struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"CE.1"`
						} `xml:"PID.16"`
						PID17 struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							Table    string `xml:"Table,attr"`
							LongName string `xml:"LongName,attr"`
							CE1      struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"CE.1"`
						} `xml:"PID.17"`
						PID22 struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							Table    string `xml:"Table,attr"`
							LongName string `xml:"LongName,attr"`
							CE1      struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"CE.1"`
						} `xml:"PID.22"`
						PID24 struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							Table    string `xml:"Table,attr"`
							LongName string `xml:"LongName,attr"`
						} `xml:"PID.24"`
						PID28 struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							Table    string `xml:"Table,attr"`
							LongName string `xml:"LongName,attr"`
							CE1      struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"CE.1"`
						} `xml:"PID.28"`
						PID29 struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							LongName string `xml:"LongName,attr"`
							TS1      struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"TS.1"`
						} `xml:"PID.29"`
					} `xml:"PID"`
					PD1 struct {
						Text string `xml:",chardata"`
						PD13 struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							LongName string `xml:"LongName,attr"`
							XON3     struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XON.3"`
						} `xml:"PD1.3"`
						PD14 struct {
							Text     string `xml:",chardata"`
							Item     string `xml:"Item,attr"`
							Type     string `xml:"Type,attr"`
							LongName string `xml:"LongName,attr"`
							XCN1     struct {
								Text     string `xml:",chardata"`
								Type     string `xml:"Type,attr"`
								LongName string `xml:"LongName,attr"`
							} `xml:"XCN.1"`
						} `xml:"PD1.4"`
					} `xml:"PD1"`
				} `xml:"RSP_K21.QUERY_RESPONSE"`
			} `xml:"RSP_K21"`
		} `xml:"InvokePatientDemographicsQueryResponse"`
	} `xml:"Body"`
}
