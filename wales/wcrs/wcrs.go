package wcrs

import (
	"bytes"
	"crypto/tls"
	"encoding/xml"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"time"
)

// against "unused imports"
var _ time.Time
var _ xml.Name

type GUIDtype string

type TimeSeriesStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 TimeSeriesStructure"`

	// The identifier of the 'subject' of the time series E.g. this could be the NHS number of the patient for whom a time series is required.
	Subject *IdentifierStructure `xml:"Subject,omitempty"`

	// E.g. for pathology this could be the profile type e.g. 'Full Blood Count'
	Category *TypeStructure `xml:"Category,omitempty"`

	// The type of data point in the time series e.g. for Pathology using the 'Full Blood Count' category the data point type might be 'White Cell Count'
	Type *TypeStructure `xml:"Type,omitempty"`

	Units *TypeStructure `xml:"Units,omitempty"`

	DataPoint []*TimeSeriesDataPointStructure `xml:"DataPoint,omitempty"`
}

type TimeSeriesDataPointStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 TimeSeriesDataPointStructure"`

	DateTime time.Time `xml:"DateTime,omitempty"`

	Value string `xml:"Value,omitempty"`

	Range []*TimeSeriesDataPointRangeStructure `xml:"Range,omitempty"`
}

type TimeSeriesDataPointRangeStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 TimeSeriesDataPointRangeStructure"`

	Type *TypeStructure `xml:"Type,omitempty"`

	HighValue string `xml:"HighValue,omitempty"`

	LowValue string `xml:"LowValue,omitempty"`
}

type AddressStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 AddressStructure"`

	AddressLine1 string `xml:"AddressLine1,omitempty"`

	AddressLine2 string `xml:"AddressLine2,omitempty"`

	AddressLine3 string `xml:"AddressLine3,omitempty"`

	AddressLine4 string `xml:"AddressLine4,omitempty"`

	Postcode string `xml:"Postcode,omitempty"`

	// Address type could be:
	// current
	// past
	// carer
	// care home
	// holiday
	// temporary
	// etc..
	Type *TypeStructure `xml:"Type,omitempty"`

	StartDate time.Time `xml:"StartDate,omitempty"`

	EndDate time.Time `xml:"EndDate,omitempty"`
}

type CareAgentStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 CareAgentStructure"`

	Identifier *IdentifierStructure `xml:"Identifier,omitempty"`

	PersonName *PersonNameStructure `xml:"PersonName,omitempty"`

	OrganisationName string `xml:"OrganisationName,omitempty"`

	// Typically this type is to distinguish between the Agents as organisations, groups, individuals etc.
	RoleType *TypeStructure `xml:"RoleType,omitempty"`

	// Within roleType (e.g. person) this provides roles such as nurse, physio, doctor etc.
	RoleSubtype *TypeStructure `xml:"RoleSubtype,omitempty"`

	Address *AddressStructure `xml:"Address,omitempty"`

	ContactInformation []*ContactInformationStructure `xml:"ContactInformation,omitempty"`
}

type ContactInformationStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 ContactInformationStructure"`

	// Contact info type could be:
	// land line
	// mobile pone
	// email
	// pager
	// land line: carer
	// etc..
	ContactInformationType *TypeStructure `xml:"ContactInformationType,omitempty"`

	Data string `xml:"Data,omitempty"`
}

type IdentifierStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 IdentifierStructure"`

	Domain string `xml:"Domain,omitempty"`

	Value string `xml:"Value,omitempty"`
}

type PatientCharacteristicStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 PatientCharacteristicStructure"`

	CharacteristicEstablished time.Time `xml:"CharacteristicEstablished,omitempty"`

	Description string `xml:"Description,omitempty"`

	CharacteristicEstablishedBy *CareAgentStructure `xml:"CharacteristicEstablishedBy,omitempty"`

	// This would be a clinical code if known:
	// TypeId: the clinical code itself
	// Domain: the coding system. e.g. Snomed 2006, Read 2
	// Rubric: the rubric used by the coding systems authors to describe this code. e.g. Common cold, Non-insulin dependent diabetes mellitus etc.
	CharacteristicType *TypeStructure `xml:"CharacteristicType,omitempty"`

	Sensitivity *TypeStructure `xml:"Sensitivity,omitempty"`

	SourceSystem string `xml:"SourceSystem,omitempty"`
}

type PatientIdentificationInformationStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 PatientIdentificationInformationStructure"`

	PatientIdentifier []*IdentifierStructure `xml:"PatientIdentifier,omitempty"`

	Name *PersonNameStructure `xml:"Name,omitempty"`

	DateOfBirth time.Time `xml:"DateOfBirth,omitempty"`

	DateOfDeath time.Time `xml:"DateOfDeath,omitempty"`

	Address *AddressStructure `xml:"Address,omitempty"`

	Sex *TypeStructure `xml:"Sex,omitempty"`
}

type PersonNameStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 PersonNameStructure"`

	GivenName string `xml:"GivenName,omitempty"`

	FamilyName string `xml:"FamilyName,omitempty"`

	OtherNames string `xml:"OtherNames,omitempty"`

	KnownAs string `xml:"KnownAs,omitempty"`

	Title *TypeStructure `xml:"Title,omitempty"`

	// For example: married name, professional name, maiden name
	Type *TypeStructure `xml:"Type,omitempty"`

	EndDate time.Time `xml:"EndDate,omitempty"`

	StartDate time.Time `xml:"StartDate,omitempty"`
}

type TypeStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 TypeStructure"`

	// The actual code.
	TypeId string `xml:"TypeId,omitempty"`

	// This is likely to be anamespace, GUID or OID assigned to, say, a clinical coding system.
	Domain string `xml:"Domain,omitempty"`

	Rubric string `xml:"Rubric,omitempty"`
}

type ServiceEndpointStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 ServiceEndpointStructure"`

	Endpoint *AnyURI `xml:"Endpoint,omitempty"`

	WSDL *AnyURI `xml:"WSDL,omitempty"`
}

type ErrorResponse struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 ErrorResponse"`

	ErrorCode *TypeStructure `xml:"ErrorCode,omitempty"`

	ErrorText string `xml:"ErrorText,omitempty"`

	ErrorDiagnosticTest string `xml:"ErrorDiagnosticTest,omitempty"`
}

type DocumentAttributeStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 DocumentAttributeStructure"`

	Attribute string `xml:"Attribute,omitempty"`

	Namespace *AnyURI `xml:"Namespace,omitempty"`

	Value string `xml:"Value,omitempty"`

	ValueDomain string `xml:"ValueDomain,omitempty"`
}

type DocumentStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 DocumentStructure"`

	// The unique identifier of the 'logical document' i.e. the unique identifier of the document supersession set.
	DocumentSupersessionSetId *GUIDtype `xml:"DocumentSupersessionSetId,omitempty"`

	// The sequence number of the head document in the supersession set.
	HeadDocumentSequenceNumber int32 `xml:"HeadDocumentSequenceNumber,omitempty"`

	// The presence of this element with a value of 'true' indicates that this document supersession set has been revoked.  The absence of the element or a value of 'false' indicates that the document supersession set has not been revoked.
	Revoked bool `xml:"Revoked,omitempty"`

	// The document version of interest i.e. the version that is being viewed or acted upon at a given time.  This will usually be the head document in the supersession set.
	DocumentVersion *DocumentVersionStructure `xml:"DocumentVersion,omitempty"`

	History *DocumentHistoryStructure `xml:"History,omitempty"`

	// Zero or more identifier(s) for a single subject (e.g. a patient) to which the document pertains.
	SubjectIdentifier []*IdentifierStructure `xml:"SubjectIdentifier,omitempty"`

	// Zero or more tasks associated with the document.
	Task []*TaskStructure `xml:"Task,omitempty"`

	RelatedDocument []*RelatedDocumentStructure `xml:"RelatedDocument,omitempty"`
}

type DocumentVersionHeaderStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 DocumentVersionHeaderStructure"`

	DocumentId *GUIDtype `xml:"DocumentId,omitempty"`

	DocumentSupersessionSetId *GUIDtype `xml:"DocumentSupersessionSetId,omitempty"`

	// The sequence number of the document within the supersession set.  E.g. the first document in the set would have sequence number 1, the second 2 etc.
	SetSequenceNumber int32 `xml:"SetSequenceNumber,omitempty"`

	// The date and time the document was saved.
	DocumentDateTime time.Time `xml:"DocumentDateTime,omitempty"`

	EventDateTime time.Time `xml:"EventDateTime,omitempty"`

	// The MIME type of the document, taken from the subset of supported MIME types for the National Document Repository.
	// e.g.
	// application/xml
	// application/winword
	MIMEtype string `xml:"MIMEtype,omitempty"`

	// The version number of the document.
	VersionNumber string `xml:"VersionNumber,omitempty"`

	// A text description associated with this version of the document.
	VersionDescription string `xml:"VersionDescription,omitempty"`

	// A code indicating the sensitivity status of the document.
	SensitivityTypeCode string `xml:"SensitivityTypeCode,omitempty"`

	// Optional code that will allow the calling application to determine how to render that particular document. For example, if the document was an XML instance then the code could be used to look-up an XSL transform for the instance.
	RenderingCode string `xml:"RenderingCode,omitempty"`

	// A code designating the repository that holds the actual document instance e.g. the binary document.  This allows for documents to be 'registered' in the document repository but for the actual document to reside in a different repository, e.g. existing legacy document stores. If the document is stored in a different location to the National Document Repository instance then the retrieval service layer will be responsible for retrieving the document from the alternative location. From the client application's perspective there will be a single service for retrieving documents, even if some of those documents are stored in alternative locations.
	LocationCode string `xml:"LocationCode,omitempty"`

	// The presence of this element with a value of 'true' indicates that this document version has been revoked.  The absence of the element or a value of 'false' indicates that the document version has not been revoked.
	Revoked bool `xml:"Revoked,omitempty"`

	// A set of zero or more attribute and value pairs each holding a specific item of metadata about the document.
	DocumentAttribute []*DocumentAttributeStructure `xml:"DocumentAttribute,omitempty"`

	HistoryRecord []*HistoryRecordStructure `xml:"HistoryRecord,omitempty"`

	SubjectDemographicsAsRecorded *SubjectDemographicsStructure `xml:"SubjectDemographicsAsRecorded,omitempty"`
}

type HistoryRecordStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 HistoryRecordStructure"`

	ActionTypeCode string `xml:"ActionTypeCode,omitempty"`

	ActioneeId *IdentifierStructure `xml:"ActioneeId,omitempty"`

	ActioneeApplication *IdentifierStructure `xml:"ActioneeApplication,omitempty"`

	DateTime time.Time `xml:"DateTime,omitempty"`

	Comments string `xml:"Comments,omitempty"`
}

type TaskStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 TaskStructure"`

	TaskId *GUIDtype `xml:"TaskId,omitempty"`

	TaskTypeCode string `xml:"TaskTypeCode,omitempty"`

	Active bool `xml:"Active,omitempty"`

	Completed bool `xml:"Completed,omitempty"`

	AssigneeId *IdentifierStructure `xml:"AssigneeId,omitempty"`
}

type DocumentVersionStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 DocumentVersionStructure"`

	Header *DocumentVersionHeaderStructure `xml:"Header,omitempty"`

	Body *DocumentVersionBodyStructure `xml:"Body,omitempty"`
}

type DocumentVersionBodyStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 DocumentVersionBodyStructure"`

	// The base-64 encoded binary document.
	DocumentBase64 []byte `xml:"DocumentBase64,omitempty"`

	DocumentXML *AnyXMLStructure `xml:"DocumentXML,omitempty"`
}

type AnyXMLStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 AnyXMLStructure"`
}

type DocumentHistoryStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 DocumentHistoryStructure"`

	DocumentVersionHeader []*DocumentVersionHeaderStructure `xml:"DocumentVersionHeader,omitempty"`
}

type SubjectDemographicsStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 SubjectDemographicsStructure"`

	SubjectIdentifier []*IdentifierStructure `xml:"SubjectIdentifier,omitempty"`

	FamilyName string `xml:"FamilyName,omitempty"`

	GivenName string `xml:"GivenName,omitempty"`

	DateOfBirth time.Time `xml:"DateOfBirth,omitempty"`

	SexCode string `xml:"SexCode,omitempty"`

	AddressLine1 string `xml:"AddressLine1,omitempty"`

	AddressLine2 string `xml:"AddressLine2,omitempty"`

	AddressLine3 string `xml:"AddressLine3,omitempty"`

	AddressLine4 string `xml:"AddressLine4,omitempty"`

	Postcode string `xml:"Postcode,omitempty"`
}

type RelatedDocumentStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 RelatedDocumentStructure"`

	DocumentSupersessionSetId *GUIDtype `xml:"DocumentSupersessionSetId,omitempty"`

	RelationshipTypeCode string `xml:"RelationshipTypeCode,omitempty"`

	// A code indicating whether the relationship is 'FROM' the related document or 'TO' the related document.  E.g. if the relationship type is 'parent-child' and the direction is 'TO' then the document containing the reference to the related document is the parent of the related document.
	RelationshipDirection string `xml:"RelationshipDirection,omitempty"`
}

type CredentialsStructure struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 CredentialsStructure"`

	ApplicationId *IdentifierStructure `xml:"ApplicationId,omitempty"`

	UserId *IdentifierStructure `xml:"UserId,omitempty"`

	// An optional comment that will be stored in the audit trail.
	HistoryComment string `xml:"HistoryComment,omitempty"`
}

type StoreDocumentRequest struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 StoreDocumentRequest"`

	Credentials *CredentialsStructure `xml:"Credentials,omitempty"`

	DocumentVersion *DocumentVersionStructure `xml:"DocumentVersion,omitempty"`

	// If present and set to false, then the document will be updated in non-superseding mode.  That means that the existing head document will be updated, without creating a new head document version. If not present or set to true, then the default supersession behaviour will occur.  That means a new head document version will be created in the supersession set.
	Supersede bool `xml:"Supersede,omitempty"`
}

type StoreDocumentResponse struct {
	XMLName xml.Name `xml:"http://www.wales.nhs.uk/namespaces/MessageRelease2 StoreDocumentResponse"`

	Success bool `xml:"Success,omitempty"`

	DocumentId *GUIDtype `xml:"DocumentId,omitempty"`

	DocumentSupersessionSetId *GUIDtype `xml:"DocumentSupersessionSetId,omitempty"`
}

type StoreDocumentPortType struct {
	client *SOAPClient
}

func NewStoreDocumentPortType(url string, tls bool, auth *BasicAuth) *StoreDocumentPortType {
	if url == "" {
		url = ""
	}
	client := NewSOAPClient(url, tls, auth)

	return &StoreDocumentPortType{
		client: client,
	}
}

func NewStoreDocumentPortTypeWithTLSConfig(url string, tlsCfg *tls.Config, auth *BasicAuth) *StoreDocumentPortType {
	if url == "" {
		url = ""
	}
	client := NewSOAPClientWithTLSConfig(url, tlsCfg, auth)

	return &StoreDocumentPortType{
		client: client,
	}
}

func (service *StoreDocumentPortType) AddHeader(header interface{}) {
	service.client.AddHeader(header)
}

// Backwards-compatible function: use AddHeader instead
func (service *StoreDocumentPortType) SetHeader(header interface{}) {
	service.client.AddHeader(header)
}

// Error can be either of the following types:
//
//   - fault1

func (service *StoreDocumentPortType) StoreDocument(request *StoreDocumentRequest) (*StoreDocumentResponse, error) {
	response := new(StoreDocumentResponse)
	err := service.client.Call("StoreDocument", request, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

var timeout = time.Duration(30 * time.Second)

func dialTimeout(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, timeout)
}

type SOAPEnvelope struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	Header  *SOAPHeader
	Body    SOAPBody
}

type SOAPHeader struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Header"`

	Items []interface{} `xml:",omitempty"`
}

type SOAPBody struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`

	Fault   *SOAPFault  `xml:",omitempty"`
	Content interface{} `xml:",omitempty"`
}

type SOAPFault struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault"`

	Code   string `xml:"faultcode,omitempty"`
	String string `xml:"faultstring,omitempty"`
	Actor  string `xml:"faultactor,omitempty"`
	Detail string `xml:"detail,omitempty"`
}

const (
	// Predefined WSS namespaces to be used in
	WssNsWSSE string = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"
	WssNsWSU  string = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd"
	WssNsType string = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordText"
)

type WSSSecurityHeader struct {
	XMLName   xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ wsse:Security"`
	XmlNSWsse string   `xml:"xmlns:wsse,attr"`

	MustUnderstand string `xml:"mustUnderstand,attr,omitempty"`

	Token *WSSUsernameToken `xml:",omitempty"`
}

type WSSUsernameToken struct {
	XMLName   xml.Name `xml:"wsse:UsernameToken"`
	XmlNSWsu  string   `xml:"xmlns:wsu,attr"`
	XmlNSWsse string   `xml:"xmlns:wsse,attr"`

	Id string `xml:"wsu:Id,attr,omitempty"`

	Username *WSSUsername `xml:",omitempty"`
	Password *WSSPassword `xml:",omitempty"`
}

type WSSUsername struct {
	XMLName   xml.Name `xml:"wsse:Username"`
	XmlNSWsse string   `xml:"xmlns:wsse,attr"`

	Data string `xml:",chardata"`
}

type WSSPassword struct {
	XMLName   xml.Name `xml:"wsse:Password"`
	XmlNSWsse string   `xml:"xmlns:wsse,attr"`
	XmlNSType string   `xml:"Type,attr"`

	Data string `xml:",chardata"`
}

type BasicAuth struct {
	Login    string
	Password string
}

type SOAPClient struct {
	url     string
	tlsCfg  *tls.Config
	auth    *BasicAuth
	headers []interface{}
}

// **********
// Accepted solution from http://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-golang
// Author: Icza - http://stackoverflow.com/users/1705598/icza

const (
	letterBytes   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

func randStringBytesMaskImprSrc(n int) string {
	src := rand.NewSource(time.Now().UnixNano())
	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}
	return string(b)
}

// **********

func NewWSSSecurityHeader(user, pass, mustUnderstand string) *WSSSecurityHeader {
	hdr := &WSSSecurityHeader{XmlNSWsse: WssNsWSSE, MustUnderstand: mustUnderstand}
	hdr.Token = &WSSUsernameToken{XmlNSWsu: WssNsWSU, XmlNSWsse: WssNsWSSE, Id: "UsernameToken-" + randStringBytesMaskImprSrc(9)}
	hdr.Token.Username = &WSSUsername{XmlNSWsse: WssNsWSSE, Data: user}
	hdr.Token.Password = &WSSPassword{XmlNSWsse: WssNsWSSE, XmlNSType: WssNsType, Data: pass}
	return hdr
}

func (b *SOAPBody) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	if b.Content == nil {
		return xml.UnmarshalError("Content must be a pointer to a struct")
	}

	var (
		token    xml.Token
		err      error
		consumed bool
	)

Loop:
	for {
		if token, err = d.Token(); err != nil {
			return err
		}

		if token == nil {
			break
		}

		switch se := token.(type) {
		case xml.StartElement:
			if consumed {
				return xml.UnmarshalError("Found multiple elements inside SOAP body; not wrapped-document/literal WS-I compliant")
			} else if se.Name.Space == "http://schemas.xmlsoap.org/soap/envelope/" && se.Name.Local == "Fault" {
				b.Fault = &SOAPFault{}
				b.Content = nil

				err = d.DecodeElement(b.Fault, &se)
				if err != nil {
					return err
				}

				consumed = true
			} else {
				if err = d.DecodeElement(b.Content, &se); err != nil {
					return err
				}

				consumed = true
			}
		case xml.EndElement:
			break Loop
		}
	}

	return nil
}

func (f *SOAPFault) Error() string {
	return f.String
}

func NewSOAPClient(url string, insecureSkipVerify bool, auth *BasicAuth) *SOAPClient {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: insecureSkipVerify,
	}
	return NewSOAPClientWithTLSConfig(url, tlsCfg, auth)
}

func NewSOAPClientWithTLSConfig(url string, tlsCfg *tls.Config, auth *BasicAuth) *SOAPClient {
	return &SOAPClient{
		url:    url,
		tlsCfg: tlsCfg,
		auth:   auth,
	}
}

func (s *SOAPClient) AddHeader(header interface{}) {
	s.headers = append(s.headers, header)
}

func (s *SOAPClient) Call(soapAction string, request, response interface{}) error {
	envelope := SOAPEnvelope{}

	if s.headers != nil && len(s.headers) > 0 {
		soapHeader := &SOAPHeader{Items: make([]interface{}, len(s.headers))}
		copy(soapHeader.Items, s.headers)
		envelope.Header = soapHeader
	}

	envelope.Body.Content = request
	buffer := new(bytes.Buffer)

	encoder := xml.NewEncoder(buffer)
	//encoder.Indent("  ", "    ")

	if err := encoder.Encode(envelope); err != nil {
		return err
	}

	if err := encoder.Flush(); err != nil {
		return err
	}

	log.Println(buffer.String())

	req, err := http.NewRequest("POST", s.url, buffer)
	if err != nil {
		return err
	}
	if s.auth != nil {
		req.SetBasicAuth(s.auth.Login, s.auth.Password)
	}

	req.Header.Add("Content-Type", "text/xml; charset=\"utf-8\"")
	req.Header.Add("SOAPAction", soapAction)

	req.Header.Set("User-Agent", "gowsdl/0.1")
	req.Close = true

	tr := &http.Transport{
		TLSClientConfig: s.tlsCfg,
		Dial:            dialTimeout,
	}

	client := &http.Client{Transport: tr}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	rawbody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if len(rawbody) == 0 {
		log.Println("empty response")
		return nil
	}

	log.Println(string(rawbody))
	respEnvelope := new(SOAPEnvelope)
	respEnvelope.Body = SOAPBody{Content: response}
	err = xml.Unmarshal(rawbody, respEnvelope)
	if err != nil {
		return err
	}

	fault := respEnvelope.Body.Fault
	if fault != nil {
		return fault
	}

	return nil
}
