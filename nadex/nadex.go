package nadex

import (
	"fmt"
	"log"

	"gopkg.in/jcmturner/gokrb5.v7/client"
	"gopkg.in/jcmturner/gokrb5.v7/config"
	auth "gopkg.in/korylprince/go-ad-auth.v2"
	ldap "gopkg.in/ldap.v3"
)

const (
	krbConfig = `[libdefaults]
default_real = CYMRU.NHS.UK
dns_lookup_realm = false
dns_lookup_kdc = false
ticket_lifetime = 24h
forwardable = yes
default_tkt_enctypes = aes256-cts-hmac-sha1-96
default_tgs_enctypes = aes256-cts-hmac-sha1-96
 
[realms]
CYMRU.NHS.UK = {
    kdc = 7a4bvsrvdom0001.cymru.nhs.uk
}
 
[domain_realm]
.nhs.uk = CYMRU.NHS.UK
nhs.uk = CYMRU.NHS.UK
`
)

func Experiments(username string, password string) {

	// first, let's try kerberos
	cfg, err := config.NewConfigFromString(krbConfig)
	if err != nil {
		panic(err)
	}
	cl := client.NewClientWithPassword(username, "CYMRU.NHS.UK", password, cfg, client.DisablePAFXFAST(true))

	err = cl.Login()
	if err != nil {
		log.Fatalf("kerberos error: %s\n", err)
	} else {
		log.Printf("Kerberos success!\n")
	}
	// now let's use LDAP instead
	config := &auth.Config{
		Server:   "cymru.nhs.uk",
		Port:     389,
		BaseDN:   "OU=Users,DC=cymru,DC=nhs,DC=uk",
		Security: auth.SecurityStartTLS,
	}

	status, err := auth.Authenticate(config, username, password)
	if err != nil {
		panic(err)
	}
	if status {
		fmt.Printf("LDAP login success!")
	} else {
		fmt.Printf("failed login")
	}

	conn, err := config.Connect()
	if err != nil {
		panic(err)
	}
	defer conn.Conn.Close()
	upn, err := config.UPN(username)
	if err != nil {
		panic(err)
	}
	success, err := conn.Bind(upn, password)
	if err != nil {
		panic(err)
	}
	if !success {
		panic("invalid credentials")
	}
	searchRequest := ldap.NewSearchRequest(
		"dc=cymru,dc=nhs,dc=uk", // The base dn to search
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		"(&(objectClass=User)(sAMAccountName=ma090906))", // The filter to apply
		[]string{"sn", "givenName", "mail", "title"},     // A list attributes to retrieve
		nil,
	)

	sr, err := conn.Conn.Search(searchRequest)
	if err != nil {
		log.Fatal(err)
	}

	for _, entry := range sr.Entries {
		givenNames := entry.GetAttributeValue("givenName")
		surname := entry.GetAttributeValue("sn")
		email := entry.GetAttributeValue("mail")
		jobTitle := entry.GetAttributeValue("title")
		fmt.Printf("%s:\n firstnames:%v lastname:%v email:%v job title:%v\n", entry.DN, givenNames, surname, email, jobTitle)
	}
}
