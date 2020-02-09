package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/gorilla/mux"
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
	"",
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

func lookupEndpoint(s string) Endpoint {
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

var endpoint = flag.String("endpoint", "D", "(P)roduction, (T)esting or (D)evelopment")
var nnn = flag.String("nnn", "", "NHS number to fetch e.g. 7253698428, 7705820730, 6145933267")
var logger = flag.String("log", "", "logfile to use")
var serve = flag.Bool("serve", false, "whether to start a REST server")
var port = flag.Int("port", 8080, "port to use")
var cacheMinutes = flag.Int("cache", 5, "cache expiration in minutes, 0=no cache")
var fake = flag.Bool("fake", false, "run a fake service")
var timeoutSeconds = flag.Int("timeout", 2, "timeout in seconds for external services")

// unset http_proxy
// unset https_proxy
func main() {
	flag.Parse()
	if *logger != "" {
		f, err := os.OpenFile(*logger, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			fmt.Printf("fatal error. couldn't open log file ('%s'): %s", *logger, err)
			os.Exit(1)
		}
		log.SetOutput(f)
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}
	httpProxy, exists := os.LookupEnv("http_proxy") // give warning if proxy set, as we don't need a proxy
	if exists {
		log.Printf("warning: http proxy set to %s\n", httpProxy)
	}
	httpsProxy, exists := os.LookupEnv("https_proxy")
	if exists {
		log.Printf("warning: https proxy set to %s\n", httpsProxy)
	}
	ep := lookupEndpoint(*endpoint)
	if endpointURLs[ep] == "" {
		log.Fatalf("error: unknown or unsupported endpoint: %s", *endpoint)
	}

	// handle a command-line test with a specified NHS number
	if *nnn != "" {
		ctx := context.Background()
		pt, err := performRequest(ctx, endpointURLs[ep], endpointCodes[ep], *nnn)
		if err != nil {
			panic(err)
		}
		if pt == nil {
			log.Printf("Not Found")
			return
		}
		if err := json.NewEncoder(os.Stdout).Encode(pt); err != nil {
			panic(err)
		}
		return
	}

	if *serve {
		sigs := make(chan os.Signal, 1) // channel to receive OS signals
		signal.Notify(sigs, os.Interrupt, os.Kill, syscall.SIGTERM)
		go func() {
			s := <-sigs
			log.Printf("RECEIVED SIGNAL: %s", s)
			os.Exit(1)
		}()
		app := new(App)
		app.Endpoint = ep
		app.Router = mux.NewRouter().StrictSlash(true)
		app.Fake = *fake
		app.TimeoutSeconds = *timeoutSeconds
		if *cacheMinutes != 0 {
			app.Cache = cache.New(time.Duration(*cacheMinutes)*time.Minute, time.Duration(*cacheMinutes*2)*time.Minute)
		}
		app.Router.HandleFunc("/users/{user}/nnn/{nnn}", app.getNhsNumber).Methods("GET")
		log.Printf("starting REST server: port:%d cache:%dm timeout:%ds endpoint:(%s)%s",
			*port, *cacheMinutes, *timeoutSeconds, endpointNames[ep], endpointURLs[ep])
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), app.Router))
		return
	}
	flag.PrintDefaults()
}

// App represents the application
type App struct {
	Endpoint       Endpoint
	Router         *mux.Router
	Cache          *cache.Cache // may be nil if not caching
	Fake           bool
	TimeoutSeconds int
}

func (a *App) getCache(key string) (*Patient, bool) {
	if a.Cache == nil {
		return nil, false
	}
	if o, found := a.Cache.Get(key); found {
		return o.(*Patient), true
	}
	return nil, false
}

func (a *App) setCache(key string, value *Patient) {
	if a.Cache == nil {
		return
	}
	a.Cache.Set(key, value, cache.DefaultExpiration)
}

func (a *App) getNhsNumber(w http.ResponseWriter, r *http.Request) {
	user := mux.Vars(r)["user"]
	nnn := mux.Vars(r)["nnn"]
	log.Printf("request by user: '%s' for nnn: '%s': %+v", user, nnn, r)
	start := time.Now()
	pt, found := a.getCache(nnn)
	var err error
	if !found {
		if !a.Fake {
			ctx, _ := context.WithTimeout(context.Background(), time.Duration(a.TimeoutSeconds)*time.Second)
			pt, err = performRequest(ctx, endpointURLs[a.Endpoint], endpointCodes[a.Endpoint], nnn)
		} else {
			pt, err = performFake(nnn)
		}
		if err != nil {
			log.Printf("error: %s", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.setCache(nnn, pt)
	} else {
		log.Printf("serving request for %s from cache in %s", nnn, time.Since(start))
	}
	if pt == nil {
		log.Printf("patient with identifier %s not found", nnn)
		http.NotFound(w, r)
		return
	}
	log.Printf("result: %+v", pt)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if err := json.NewEncoder(w).Encode(pt); err != nil {
		log.Printf("error: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func performFake(nnn string) (*Patient, error) {
	dob := time.Date(1960, 01, 01, 00, 00, 00, 0, time.UTC)

	return &Patient{
		Lastname:            "DUMMY",
		Firstnames:          "ALBERT",
		Title:               "DR",
		Sex:                 "M",
		DateBirth:           &dob,
		Surgery:             "W95010",
		GeneralPractitioner: "G9342400",
		Identifiers: []Identifier{
			Identifier{
				Authority: "NHS",
				ID:        nnn,
			},
			Identifier{
				Authority: "103",
				ID:        "M1147907",
			},
		},
		Addresses: []Address{
			Address{
				Address1: "59 Robins Hill",
				Address2: "Brackla",
				Address3: "BRIDGEND",
				Postcode: "CF31 2PJ",
			},
		},
		Telephones: []Telephone{
			Telephone{
				Number:      "02920747747",
				Description: "Work number",
			},
		},
		EmailAddresses: []string{"test@test.com"},
	}, nil
}

func performRequest(context context.Context, endpointURL string, processingID string, nnn string) (*Patient, error) {
	start := time.Now()
	data, err := NewNhsNumberRequest(nnn, "221", "100", processingID)
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
	log.Printf("response (%s): %v", time.Since(start), string(body))
	err = xml.Unmarshal(body, &e)
	if err != nil {
		return nil, err
	}
	return e.ToPatient()
}

// NhsNumberRequest is used to populate the template to make the XML request
type NhsNumberRequest struct {
	NhsNumber            string
	SendingApplication   string
	SendingFacility      string
	ReceivingApplication string
	ReceivingFacility    string
	DateTime             string
	ProcessingID         string //for MSH.11 - P/U/T production/testing/development
}

// NewNhsNumberRequest returns a correctly formatted XML request to search by NHS number
// sender : 221 (PatientCare)
// receiver: 100 (NHS Wales EMPI)
func NewNhsNumberRequest(nnn string, sender string, receiver string, processingID string) ([]byte, error) {
	layout := "20060102150405" // YYYYMMDDHHMMSS
	now := time.Now().Format(layout)
	data := NhsNumberRequest{
		NhsNumber:            nnn,
		SendingApplication:   sender,
		SendingFacility:      sender,
		ReceivingApplication: receiver,
		ReceivingFacility:    receiver,
		DateTime:             now,
		ProcessingID:         processingID,
	}
	t, err := template.New("nhs-number-request").Parse(nhsNumberRequestTemplate)
	if err != nil {
		return nil, err
	}
	log.Printf("request: %+v", data)
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Identifier represents an organisation's identifier for this patient
type Identifier struct {
	Authority string `json:"authority"`
	ID        string `json:"identifier"`
}

// Address represents an address for this patient
type Address struct {
	Address1 string     `json:"address1"`
	Address2 string     `json:"address2"`
	Address3 string     `json:"address3"`
	Address4 string     `json:"address4"`
	Postcode string     `json:"postcode"`
	DateFrom *time.Time `json:"dateFrom"` // valid from
	DateTo   *time.Time `json:"dateTo"`   // valid to
}

// Patient is a patient
type Patient struct {
	Lastname            string       `json:"lastName"`
	Firstnames          string       `json:"firstNames"`
	Title               string       `json:"title"`
	Sex                 string       `json:"sex"`
	DateBirth           *time.Time   `json:"dateBirth"`
	DateDeath           *time.Time   `json:"dateDeath"`
	Surgery             string       `json:"surgery"`
	GeneralPractitioner string       `json:"generalPractitioner"`
	Identifiers         []Identifier `json:"identifiers"`
	Addresses           []Address    `json:"addresses"`
	Telephones          []Telephone  `json:"telephones"`
	EmailAddresses      []string     `json:"emailAddresses"`
}

type Telephone struct {
	Number      string `json:"telephone"`
	Description string `json:"description"`
}

// ToPatient creates a "Patient" from the XML returned from the EMPI service
func (e *envelope) ToPatient() (*Patient, error) {
	pt := new(Patient)
	pt.Lastname = e.surname()
	pt.Firstnames = e.firstnames()
	if pt.Lastname == "" && pt.Firstnames == "" {
		return nil, nil
	}
	pt.Title = e.title()
	pt.Sex = e.sex()
	pt.DateBirth = e.dateBirth()
	pt.DateDeath = e.dateDeath()
	pt.Identifiers = e.identifiers()
	pt.Addresses = e.addresses()
	pt.Surgery = e.surgery()
	pt.GeneralPractitioner = e.generalPractitioner()
	pt.Telephones = e.telephones()
	pt.EmailAddresses = e.emailAddresses()
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
	if len(names) > 0 {
		return names[0].XPN2.Text
	}
	return ""
}

func (e *envelope) title() string {
	names := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID5
	if len(names) > 0 {
		return names[0].XPN5.Text
	}
	return ""
}

func (e *envelope) sex() string {
	return e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID8.Text
}

func (e *envelope) dateBirth() *time.Time {
	dob := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID7.TS1.Text
	if len(dob) > 0 {
		d, err := parseDate(dob)
		if err == nil {
			return d
		}
	}
	return nil
}

func (e *envelope) dateDeath() *time.Time {
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

func (e *envelope) identifiers() []Identifier {
	result := make([]Identifier, 0)
	ids := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID3
	for _, id := range ids {
		authority := id.CX4.HD1.Text
		identifier := id.CX1.Text
		if authority != "" && identifier != "" {
			result = append(result, Identifier{
				Authority: authority,
				ID:        identifier,
			})
		}
	}
	return result
}

func (e *envelope) addresses() []Address {
	result := make([]Address, 0)
	addresses := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID11
	for _, address := range addresses {
		dateFrom, _ := parseDate(address.XAD13.Text)
		dateTo, _ := parseDate(address.XAD14.Text)
		result = append(result, Address{
			Address1: address.XAD1.SAD1.Text,
			Address2: address.XAD2.Text,
			Address3: address.XAD3.Text,
			Address4: address.XAD4.Text,
			Postcode: address.XAD5.Text,
			DateFrom: dateFrom,
			DateTo:   dateTo,
		})
	}
	return result
}

func (e *envelope) telephones() []Telephone {
	result := make([]Telephone, 0)
	pid13 := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID13
	for _, telephone := range pid13 {
		num := telephone.XTN1.Text
		if num != "" {
			result = append(result, Telephone{
				Number:      num,
				Description: telephone.LongName,
			})
		}
	}
	pid14 := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID14
	for _, telephone := range pid14 {
		num := telephone.XTN1.Text
		if num != "" {
			result = append(result, Telephone{
				Number:      num,
				Description: telephone.LongName,
			})
		}
	}
	return result
}

func (e *envelope) emailAddresses() []string {
	result := make([]string, 0)
	pid13 := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID13
	for _, telephone := range pid13 {
		email := telephone.XTN4.Text
		if email != "" {
			result = append(result, email)
		}
	}
	pid14 := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID14
	for _, telephone := range pid14 {
		email := telephone.XTN4.Text
		if email != "" {
			result = append(result, email)
		}
	}
	return result
}

func parseDate(d string) (*time.Time, error) {
	layout := "20060102" // reference date is : Mon Jan 2 15:04:05 MST 2006
	if len(d) > 8 {
		d = d[:8]
	}
	t, err := time.Parse(layout, d)
	if err != nil || t.IsZero() {
		return nil, err
	}
	return &t, nil
}

var nhsNumberRequestTemplate = `
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
			<MSH.10>PDQ Message</MSH.10>
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
			   <QIP.2>{{.NhsNumber}}</QIP.2>
			</QPD.3>
			<QPD.3>
			   <!--PID.3 - Patient Identifier List:-->
			   <QIP.1>@PID.3.4</QIP.1>
			   <QIP.2>NHS</QIP.2>
			</QPD.3>
			<QPD.3>
			   <!--PID.3 - Patient Identifier List:-->
			   <QIP.1>@PID.3.5</QIP.1>
			   <QIP.2>NH</QIP.2>
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
