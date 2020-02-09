package main

import (
	"bytes"
	"encoding/xml"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"text/template"
	"time"
)

const (
	devEndpointURL  = "http://ndc06srvmpidev2.cymru.nhs.uk:23000/PatientDemographicsQueryWS.asmx"
	testEndpointURL = "https://mpitest.cymru.nhs.uk/PatientDemographicsQueryWS.asmx"
	liveEndpointURL = ""
)

var serverTest = flag.Bool("test", false, "use test server (https://mpitest.cymru.nhs.uk/PatientDemographicsQueryWS.asmx)")
var serverDev = flag.Bool("dev", true, "use dev server (http://ndc06srvmpidev2.cymru.nhs.uk:23000/PatientDemographicsQueryWS.asmx)")
var serverLive = flag.Bool("live", false, "use live server (?)")
var nnn = flag.String("nnn", "", "NHS number to fetch e.g. 7253698428, 7705820730, 6145933267")

// unset http_proxy
// unset https_proxy
func main() {
	httpProxy, exists := os.LookupEnv("http_proxy")		// give warning if proxy set, as we don't need a proxy
	if exists {
		log.Printf("warning: http proxy set to %s\n", httpProxy)
	}
	httpsProxy, exists := os.LookupEnv("https_proxy")
	if exists {
		log.Printf("warning: https proxy set to %s\n", httpsProxy)
	}
	flag.Parse()
	var endpointURL string
	if *serverDev {
		endpointURL = devEndpointURL
	} 
	if *serverTest {
		endpointURL = testEndpointURL
	} 
	if *serverLive {
		endpointURL = liveEndpointURL
	}

	// handle a command-line test with a specified NHS number
	if *nnn != "" {
		envelope, err := performRequest(endpointURL, *nnn)
		if err != nil {
			panic(err)
		}
		pt, err := envelope.ToPatient()
		if err != nil {
			panic(err)
		}
		log.Printf("result for %s: %+v\n", *nnn, pt)
	}
}

func performRequest(endpointURL string, nnn string) (*Envelope, error) {
	data, err := NewNhsNumberRequest(nnn, "221", "100")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", endpointURL, bytes.NewReader(data))
	if err != nil {
		panic(err)
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
	var envelope Envelope
	log.Printf("response: %v",string(body))
	err = xml.Unmarshal(body, &envelope)
	if err != nil {
		return nil, err
	}
	return &envelope, nil
}

// NhsNumberRequest is used to populate the template to make the XML request
type NhsNumberRequest struct {
	NhsNumber            string
	SendingApplication   string
	SendingFacility      string
	ReceivingApplication string
	ReceivingFacility    string
	DateTime             string
}

// NewNhsNumberRequest returns a correctly formatted XML request to search by NHS number
// sender : 221 (PatientCare)
// receiver: 100 (NHS Wales EMPI)
func NewNhsNumberRequest(nnn string, sender string, receiver string) ([]byte, error) {
	layout := "20060102150405" // YYYYMMDDHHMMSS
	now := time.Now().Format(layout)
	data := NhsNumberRequest{
		NhsNumber:            nnn,
		SendingApplication:   sender,
		SendingFacility:      sender,
		ReceivingApplication: receiver,
		ReceivingFacility:    receiver,
		DateTime:             now,
	}
	t, err := template.New("nhs-number-request").Parse(nhsNumberRequestTemplate)
	if err != nil {
		return nil, err
	}
	log.Printf("created request: %+v", data)
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Identifier represents an organisation's identifier for this patient
type Identifier struct {
	Authority string
	ID        string
}

// Address represents an address for this patient
type Address struct {
	Address1 string
	Address2 string
	Address3 string
	Address4 string
	Postcode string
	DateFrom time.Time		// valid from
	DateTo time.Time			// valid to
}

// Patient is a patient
type Patient struct {
	Lastname    string
	Firstnames  string
	Title       string
	DateBirth   time.Time
	DateDeath   time.Time
	Surgery		string
	GeneralPractitioner string
	Identifiers []Identifier
	Addresses   []Address
}

// ToPatient creates a "Patient" from the XML returned from the EMPI service
func (e *Envelope) ToPatient() (*Patient, error) {
	pt := new(Patient)
	pt.Lastname = e.surname()
	pt.Firstnames = e.firstnames()
	pt.Title = e.title()
	pt.DateBirth = e.dateBirth()
	pt.DateDeath = e.dateDeath()
	pt.Identifiers = e.identifiers()
	pt.Addresses = e.addresses()
	pt.Surgery = e.surgery()
	pt.GeneralPractitioner = e.generalPractitioner()
	return pt, nil
}

func (e *Envelope) surname() string {
	names := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID5
	if len(names) > 0 {
		return names[0].XPN1.FN1.Text
	}
	return ""
}

func (e *Envelope) firstnames() string {
	names := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID5
	if len(names) > 0 {
		return names[0].XPN2.Text
	}
	return ""
}

func (e *Envelope) title() string {
	names := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID5
	if len(names) > 0 {
		return names[0].XPN5.Text
	}
	return ""
}

func (e *Envelope) sex() string {
	return e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID8.Text
}

func (e *Envelope) dateBirth() time.Time {
	dob := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID7.TS1.Text
	if len(dob) > 0 {
		d, err := parseDate(dob)
		if err == nil {
			return d
		}
	}
	return time.Time{}
}

func (e *Envelope) dateDeath() time.Time {
	dod := e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PID.PID29.TS1.Text
	if len(dod) > 0 {
		d, err := parseDate(dod)
		if err == nil {
			return d
		}
	}
	return time.Time{}
}

func (e *Envelope) surgery() string {
	return e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PD1.PD13.XON3.Text
}

func (e *Envelope) generalPractitioner() string {
	return e.Body.InvokePatientDemographicsQueryResponse.RSPK21.RSPK21QUERYRESPONSE.PD1.PD14.XCN1.Text
}

func (e *Envelope) identifiers() []Identifier {
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

func (e *Envelope) addresses() []Address {
	result := make([]Address,  0)
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
			DateTo: dateTo,
		})
	}
	return result
}

func parseDate(d string) (time.Time, error) {
	layout := "20060102" // reference date is : Mon Jan 2 15:04:05 MST 2006
	if len(d) > 8 {
		d = d[:8]
	}
	return time.Parse(layout, d)
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
			   <PT.1 >P</PT.1>
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

// Envelope is a struct generated by https://www.onlinetool.io/xmltogo/ from the XML returned from the server.
// However, this doesn't take into account the possibility of repeating fields for certain PID entries.
// See https://hl7-definition.caristix.com/v2/HL7v2.5.1/Segments/PID
// which documents that the following can be repeated: PID3 PID4 PID5 PID6 PID9 PID10 PID11 PID13 PID14 PID21 PID22 PID26 PID32
// Therefore, these have been manually added as []struct rather than struct.
// Also, added PID.29 for date of death
type Envelope struct {
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
