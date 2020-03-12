package nadex

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/wardle/concierge/apiv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	Fake     bool
}

func (app App) GetPractitioner(ctx context.Context, r *apiv1.PractitionerRequest) (*apiv1.Practitioner, error) {
	if app.Fake {
		return app.GetFakePractitioner(ctx, r)
	}
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
	identifiers := make([]*apiv1.Identifier, 0)
	identifiers = append(identifiers, &apiv1.Identifier{
		System: "cymru.nhs.uk", // TODO: need to check unique system identifier for user directory
		Value:  entry.GetAttributeValue("sAMAccountName"),
	})
	//  bizarrely, the active directory uses postOfficeBox to store professional registration information
	if profReg := entry.GetAttributeValue("postOfficeBox"); profReg != "" && len(profReg) > 4 {
		switch {
		case strings.HasPrefix(profReg, "GMC:"):
			identifiers = append(identifiers, &apiv1.Identifier{
				System: "https://fhir.hl7.org.uk/Id/gmc-number", // see https://github.com/HL7-UK/System-Identifiers
				Value:  strings.TrimSpace(profReg[4:]),
			})
		}
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
		Telephones:  phones,
		Identifiers: identifiers,
	}
	if title := entry.GetAttributeValue("title"); title != "" {
		user.Roles = []*apiv1.PractitionerRole{
			&apiv1.PractitionerRole{JobTitle: title},
		}
	}
	return user, nil
}

func (app App) GetFakePractitioner(ctx context.Context, r *apiv1.PractitionerRequest) (*apiv1.Practitioner, error) {
	p := &apiv1.Practitioner{
		Active: true,
		Emails: []string{"wibble@wobble.org"},
		Names: []*apiv1.HumanName{
			&apiv1.HumanName{Given: "Fred", Family: "Flintstone", Prefixes: []string{"Mr"}},
		},
		Roles: []*apiv1.PractitionerRole{
			&apiv1.PractitionerRole{JobTitle: "Consultant Neurologist"},
		},
		Identifiers: []*apiv1.Identifier{&apiv1.Identifier{Value: r.GetUsername()}},
	}
	return p, nil
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
