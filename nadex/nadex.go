package nadex

import (
	"fmt"
	"log"

	"github.com/wardle/concierge/apiv1"
	"google.golang.org/protobuf/encoding/protojson"
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
default_tkt_enctypes = aes256-cts rc4-hmac
default_tgs_enctypes = aes256-cts rc4-hmac
 
[realms]
CYMRU.NHS.UK = {
    kdc = 7a4bvsrvdom0001.cymru.nhs.uk
}
 
[domain_realm]
.nhs.uk = CYMRU.NHS.UK
nhs.uk = CYMRU.NHS.UK
`
)

// Authenticate is a simple password check against the active directory
func Authenticate(username string, password string) (bool, error) {
	cfg, err := config.NewConfigFromString(krbConfig)
	if err != nil {
		return false, err
	}
	cl := client.NewClientWithPassword(username, "CYMRU.NHS.UK", password, cfg, client.DisablePAFXFAST(true))
	err = cl.Login()
	if err != nil {
		return false, err
	}
	return true, nil
}

// Experiments perform tests/experiments against the NHS Wales active directory, using credentials supplied
func Experiments(username string, password string) {

	// first, let's try kerberos
	cfg, err := config.NewConfigFromString(krbConfig)
	if err != nil {
		panic(err)
	}
	cl := client.NewClientWithPassword(username, "CYMRU.NHS.UK", password, cfg, client.DisablePAFXFAST(true))

	err = cl.Login()
	if err != nil {
		log.Fatalf("failed login for user %s: kerberos error: %s\n", username, err)
	} else {
		log.Printf("successful login for user %s", username)
	}

	// now let's use LDAP authentication and lookup instead
	config := &auth.Config{
		Server:   "cymru.nhs.uk",
		Port:     389,
		BaseDN:   "OU=Users,DC=cymru,DC=nhs,DC=uk",
		Security: auth.SecurityNone,
	}

	status, err := auth.Authenticate(config, username, password)
	if err != nil {
		log.Fatalf("authentication error: %s", err)
	}
	if status {
		log.Printf("LDAP login success!")
	} else {
		log.Fatalf("failed login")
	}

	conn, err := config.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Conn.Close()
	upn, err := config.UPN(username)
	if err != nil {
		log.Fatal(err)
	}
	success, err := conn.Bind(upn, password)
	if err != nil {
		log.Fatal(err)
	}
	if !success {
		log.Fatal("invalid credentials")
	}

	// search for a user
	searchUser := "ma090906" // for testing
	searchRequest := ldap.NewSearchRequest(
		"dc=cymru,dc=nhs,dc=uk", // The base dn to search
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(&(objectClass=User)(sAMAccountName=%s))", searchUser), // The filter to apply
		[]string{"sn", "givenName", "mail", "title"},                        // A list attributes to retrieve
		nil,
	)

	sr, err := conn.Conn.Search(searchRequest)
	if err != nil {
		log.Fatal(err)
	}

	for _, entry := range sr.Entries {
		user := &apiv1.Practitioner{
			Active: true,
			Names: []*apiv1.HumanName{
				&apiv1.HumanName{
					Given:    entry.GetAttributeValue("givenName"),
					Family:   entry.GetAttributeValue("sn"),
					Prefixes: []string{entry.GetAttributeValue("title")},
					Use:      apiv1.HumanName_OFFICIAL,
				},
			},
			Emails: []string{
				entry.GetAttributeValue("mail"),
			},
		}
		log.Printf(protojson.Format(user))
	}
}
