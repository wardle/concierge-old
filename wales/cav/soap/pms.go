// This was generated using gowsdl and then modified to fix unmarshalling errors
package soap

import (
	"bytes"
	"crypto/tls"
	"encoding/xml"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"time"
)

// against "unused imports"
var _ time.Time
var _ xml.Name

type GetData struct {
	XMLName xml.Name `xml:"http://localhost/PMSInterfaceWebService GetData"`

	XmlDataBlockIn string `xml:"XmlDataBlockIn,omitempty"`
}

type GetDataResponse struct {
	XMLName xml.Name `xml:"http://localhost/PMSInterfaceWebService GetDataResponse"`

	GetDataResult struct {
	} `xml:"GetDataResult,omitempty"`
}

type GetData2 struct {
	XMLName xml.Name `xml:"http://localhost/PMSInterfaceWebService GetData2"`

	XmlDataBlockIn string `xml:"XmlDataBlockIn,omitempty"`
}

type GetData2Response struct {
	XMLName xml.Name `xml:"http://localhost/PMSInterfaceWebService GetData2Response"`

	GetData2Result struct {
	} `xml:"GetData2Result,omitempty"`
}

type ReceiveFile struct {
	XMLName xml.Name `xml:"http://localhost/PMSInterfaceWebService ReceiveFile"`

	PatientId string `xml:"patientId,omitempty"`

	BfsId string `xml:"bfsId,omitempty"`

	Key string `xml:"key,omitempty"`

	Source string `xml:"source,omitempty"`

	FileContent []byte `xml:"fileContent,omitempty"`

	FileType string `xml:"fileType,omitempty"`
}

type ReceiveFileResponse struct {
	XMLName xml.Name `xml:"http://localhost/PMSInterfaceWebService ReceiveFileResponse"`
	DocId   string   `xml:"ReceiveFileByCrnResult>DocId,omitempty"`

	ErrorMessage string `xml:"ReceiveFileByCrnResult>ErrorMessage,omitempty"`
}

type ReceiveFileByCrn struct {
	XMLName xml.Name `xml:"http://localhost/PMSInterfaceWebService ReceiveFileByCrn"`

	Crn string `xml:"crn,omitempty"`

	BfsId string `xml:"bfsId,omitempty"`

	Key string `xml:"key,omitempty"`

	Source string `xml:"source,omitempty"`

	FileContent []byte `xml:"fileContent,omitempty"`

	FileType string `xml:"fileType,omitempty"`
}

type ReceiveFileByCrnResponse struct {
	XMLName xml.Name `xml:"http://localhost/PMSInterfaceWebService ReceiveFileByCrnResponse"`

	DocId        string `xml:"ReceiveFileByCrnResult>DocId,omitempty"`
	ErrorMessage string `xml:"ReceiveFileByCrnResult>ErrorMessage,omitempty"`
}

type RetrieveFile struct {
	XMLName xml.Name `xml:"http://localhost/PMSInterfaceWebService RetrieveFile"`

	BfsId string `xml:"bfsId,omitempty"`

	AuthenticationToken string `xml:"authenticationToken,omitempty"`
}

type RetrieveFileResponse struct {
	XMLName xml.Name `xml:"http://localhost/PMSInterfaceWebService RetrieveFileResponse"`

	RetrieveFileResult *ResultFile `xml:"RetrieveFileResult,omitempty"`
}

type ResultFile struct {
	XMLName xml.Name `xml:"http://localhost/PMSInterfaceWebService ResultFile"`

	FileContent []byte `xml:"FileContent,omitempty"`

	FileType string `xml:"FileType,omitempty"`

	FileName string `xml:"FileName,omitempty"`
}

type PMSInterfaceWebServiceSoap struct {
	client *SOAPClient
}

func NewPMSInterfaceWebServiceSoap(url string, tls bool, auth *BasicAuth) *PMSInterfaceWebServiceSoap {
	if url == "" {
		url = "https://cavpmswsi.cymru.nhs.uk/PMSInterfaceWebService.asmx"
	}
	client := NewSOAPClient(url, tls, auth)

	return &PMSInterfaceWebServiceSoap{
		client: client,
	}
}

func NewPMSInterfaceWebServiceSoapWithTLSConfig(url string, tlsCfg *tls.Config, auth *BasicAuth) *PMSInterfaceWebServiceSoap {
	if url == "" {
		url = "https://cavpmswsi.cymru.nhs.uk/PMSInterfaceWebService.asmx"
	}
	client := NewSOAPClientWithTLSConfig(url, tlsCfg, auth)

	return &PMSInterfaceWebServiceSoap{
		client: client,
	}
}

func (service *PMSInterfaceWebServiceSoap) AddHeader(header interface{}) {
	service.client.AddHeader(header)
}

// Backwards-compatible function: use AddHeader instead
func (service *PMSInterfaceWebServiceSoap) SetHeader(header interface{}) {
	service.client.AddHeader(header)
}

func (service *PMSInterfaceWebServiceSoap) GetData(request *GetData) (*GetDataResponse, error) {
	response := new(GetDataResponse)
	err := service.client.Call("http://localhost/PMSInterfaceWebService/GetData", request, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (service *PMSInterfaceWebServiceSoap) GetData2(request *GetData2) (*GetData2Response, error) {
	response := new(GetData2Response)
	err := service.client.Call("http://localhost/PMSInterfaceWebService/GetData2", request, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (service *PMSInterfaceWebServiceSoap) ReceiveFile(request *ReceiveFile) (*ReceiveFileResponse, error) {
	response := new(ReceiveFileResponse)
	err := service.client.Call("http://localhost/PMSInterfaceWebService/ReceiveFile", request, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (service *PMSInterfaceWebServiceSoap) ReceiveFileByCrn(request *ReceiveFileByCrn) (*ReceiveFileByCrnResponse, error) {
	response := new(ReceiveFileByCrnResponse)
	err := service.client.Call("http://localhost/PMSInterfaceWebService/ReceiveFileByCrn", request, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (service *PMSInterfaceWebServiceSoap) RetrieveFile(request *RetrieveFile) (*RetrieveFileResponse, error) {
	response := new(RetrieveFileResponse)
	err := service.client.Call("http://localhost/PMSInterfaceWebService/RetrieveFile", request, response)
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
	req, err := http.NewRequest("POST", s.url, buffer)
	if err != nil {
		return err
	}
	if s.auth != nil {
		req.SetBasicAuth(s.auth.Login, s.auth.Password)
	}

	req.Header.Add("Content-Type", "text/xml; charset=\"utf-8\"")
	req.Header.Add("SOAPAction", soapAction)
	req.Header.Set("User-Agent", "concierge")
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
		return nil
	}
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
