package nadex

import (
	"context"
	"fmt"
	"log"

	"github.com/wardle/concierge/apiv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// App reflects the NADEX server application
type App struct {
	Username string
	Password string
}

func (app App) GetPractitioner(ctx context.Context, r *apiv1.PractitionerRequest) (*apiv1.Practitioner, error) {
	config := &auth.Config{
		Server:   "cymru.nhs.uk",
		Port:     389,
		BaseDN:   "OU=Users,DC=cymru,DC=nhs,DC=uk",
		Security: auth.SecurityNone,
	}
	// for the moment, we use the fallback username/password configured - TODO: use user who is making request's own credentials
	auth, err := auth.Authenticate(config, app.Username, app.Password)
	if err != nil {
		return nil, err
	}
	if auth == false {
		log.Printf("failed to login for user %s", app.Username)
		return nil, status.Errorf(codes.Unavailable, "failed to login for user %s", app.Username)
	}
	conn, err := config.Connect()
	if err != nil {
		return nil, err
	}
	defer conn.Conn.Close()
	// perform bind
	upn, err := config.UPN(app.Username)
	if err != nil {
		return nil, err
	}
	success, err := conn.Bind(upn, app.Password)
	if err != nil {
		return nil, err
	}
	if !success {
		return nil, status.Errorf(codes.Unavailable, "failed to login for user %s", app.Username)
	}
	// search for a user
	searchRequest := ldap.NewSearchRequest(
		"dc=cymru,dc=nhs,dc=uk", // The base dn to search
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(&(objectClass=User)(sAMAccountName=%s))", r.Username), // The filter to apply
		// A list attributes to retrieve
		[]string{
			"sAMAccountName",       // username
			"displayNamePrintable", // full name including title
			"sn",                   // surname
			"givenName",            // given names
			"mail",                 // email
			"title",                // job title, not name prefix
			"photo",
			"physicalDeliveryOfficeName",
			"postalAddress", "streetAddress",
			"l",  // l=city
			"st", // state/province
			"postalCode", "telephoneNumber",
			"mobile",
			"company",
			"department",
			"wWWHomePage",
			"postOfficeBox", // appears to be used for professional registration e.g. GMC: 4624000
		},
		nil,
	)
	sr, err := conn.Conn.Search(searchRequest)
	if err != nil {
		return nil, err
	}
	if len(sr.Entries) == 0 {
		return nil, status.Errorf(codes.NotFound, "user not found: %s", r.Username)
	}
	if len(sr.Entries) > 1 {
		return nil, status.Errorf(codes.InvalidArgument, "more than one match for username %s", r.Username)
	}
	entry := sr.Entries[0]
	entry.PrettyPrint(0)
	phones := make([]*apiv1.Telephone, 0)
	if n := entry.GetAttributeValue("mobile"); n != "" {
		phones = append(phones, &apiv1.Telephone{Number: n, Description: "Mobile"})
	}
	if n := entry.GetAttributeValue("telephoneNumber"); n != "" {
		phones = append(phones, &apiv1.Telephone{Number: n, Description: "Office"})
	}
	user := &apiv1.Practitioner{
		Active: true,
		Names: []*apiv1.HumanName{
			&apiv1.HumanName{
				Given:  entry.GetAttributeValue("givenName"),
				Family: entry.GetAttributeValue("sn"),
				Use:    apiv1.HumanName_OFFICIAL,
			},
		},
		Emails: []string{
			entry.GetAttributeValue("mail"),
		},
		Telephones: phones,
		Identifier: &apiv1.Identifier{
			System: "cymru.nhs.uk", // TODO: need to check unique system identifier for user directory
			Value:  entry.GetAttributeValue("sAMAccountName"),
		},
	}
	if title := entry.GetAttributeValue("title"); title != "" {
		user.Roles = []*apiv1.PractitionerRole{
			&apiv1.PractitionerRole{JobTitle: title},
		}
	}
	return user, nil
}

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
func Experiments(username string, password string, lookupUsername string) {

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
	searchRequest := ldap.NewSearchRequest(
		"dc=cymru,dc=nhs,dc=uk", // The base dn to search
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(&(objectClass=User)(sAMAccountName=%s))", lookupUsername), // The filter to apply
		// A list attributes to retrieve
		[]string{
			"sAMAccountName",       // username
			"displayNamePrintable", // full name including title
			"sn",                   // surname
			"givenName",            // given names
			"mail",                 // email
			"title",                // job title, not name prefix
			"photo",
			"physicalDeliveryOfficeName",
			"postalAddress", "streetAddress",
			"l",  // l=city
			"st", // state/province
			"postalCode", "telephoneNumber",
			"mobile",
			"company",
			"department",
			"wWWHomePage",
			"postOfficeBox", // appears to be used for professional registration e.g. GMC: 4624000
		},
		nil,
	)

	sr, err := conn.Conn.Search(searchRequest)
	if err != nil {
		log.Fatal(err)
	}

	for _, entry := range sr.Entries {
		entry.PrettyPrint(2)
		user := &apiv1.Practitioner{
			Active: true,
			Names: []*apiv1.HumanName{
				&apiv1.HumanName{
					Given:  entry.GetAttributeValue("givenName"),
					Family: entry.GetAttributeValue("sn"),
					Use:    apiv1.HumanName_OFFICIAL,
				},
			},
			Emails: []string{
				entry.GetAttributeValue("mail"),
			},
			Identifier: &apiv1.Identifier{
				System: "cymru.nhs.uk", // TODO: need to check unique system identifier for user directory
				Value:  entry.GetAttributeValue("sAMAccountName"),
			},
		}
		log.Printf(protojson.Format(user))
	}
}
