package nadex

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/wardle/concierge/apiv1"
	"github.com/wardle/concierge/identifiers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
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

// App reflects the NADEX server application, providing user services for NHS Wales
type App struct {
	Username string
	Password string
	Fake     bool
}

var _ apiv1.PractitionerDirectoryServer = (*App)(nil)

// RegisterServer registers this server
func (app *App) RegisterServer(s *grpc.Server) {
	if app.Username == "" || app.Password == "" {
		log.Printf("nadex: warning! no credentials provided for NADEX lookup. ")
	}
	if app.Fake {
		log.Printf("nadex: running in fake mode")
	}
	apiv1.RegisterPractitionerDirectoryServer(s, app)
}

// RegisterHTTPProxy registers this as a reverse HTTP proxy
func (app *App) RegisterHTTPProxy(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) error {
	return apiv1.RegisterPractitionerDirectoryHandlerFromEndpoint(ctx, mux, endpoint, opts)
}

// Close closes any linked resources
func (app *App) Close() error { return nil }

// SearchPractitioner permits a search for a practitioner
// this currently only supports search by username!
// TODO: implement search by name
func (app *App) SearchPractitioner(r *apiv1.PractitionerSearchRequest, s apiv1.PractitionerDirectory_SearchPractitionerServer) error {
	if r.GetSystem() != identifiers.CymruUserID {
		return status.Errorf(codes.InvalidArgument, "practitioner search for namespace '%s' not supported", r.GetSystem())
	}
	if r.GetFirstName() != "" || r.GetLastName() != "" {
		return status.Errorf(codes.Unimplemented, "practitioner search by name not implemented yet")
	}
	if r.GetUsername() != "" {
		p, err := app.GetPractitioner(s.Context(), &apiv1.Identifier{System: r.GetSystem(), Value: r.GetUsername()})
		if err != nil {
			return err
		}
		if err := s.Send(p); err != nil {
			return err
		}
		return nil
	}
	return status.Errorf(codes.InvalidArgument, "no search parameters specified")
}

// ResolvePractitioner provides identifier resolution for the CYMRU USER namespace (see identifiers.CymruUserID)
func (app *App) ResolvePractitioner(ctx context.Context, id *apiv1.Identifier) (proto.Message, error) {
	return app.GetPractitioner(ctx, id)
}

// GetPractitioner returns the specified practitioner
func (app *App) GetPractitioner(ctx context.Context, r *apiv1.Identifier) (*apiv1.Practitioner, error) {
	if r.System != identifiers.CymruUserID {
		return nil, fmt.Errorf("unsupported identifier system: %s. supported: %s", r.System, identifiers.CymruUserID)
	}
	log.Printf("nadex: request for %s|%s", r.System, r.Value)
	if app.Fake {
		return app.GetFakePractitioner(ctx, r)
	}
	config := &auth.Config{
		Server:   "cymru.nhs.uk",
		Port:     389,
		BaseDN:   "OU=Users,DC=cymru,DC=nhs,DC=uk",
		Security: auth.SecurityNone,
	}
	if app.Username == "" {
		return nil, fmt.Errorf("nadex: no credentials provided for directory lookup")
	}
	// for the moment, we use the fallback username/password configured - TODO: use user who is making request's own credentials
	auth, err := auth.Authenticate(config, app.Username, app.Password)
	if err != nil {
		return nil, err
	}
	if auth == false {
		log.Printf("nadex: failed to login for user %s", app.Username)
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
		return nil, status.Errorf(codes.Unauthenticated, "failed to login for user %s", app.Username)
	}
	// search for a user
	searchRequest := ldap.NewSearchRequest(
		"dc=cymru,dc=nhs,dc=uk", // The base dn to search
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(&(objectClass=User)(sAMAccountName=%s))", r.Value), // The filter to apply
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
		log.Printf("nadex: user %s|%s not found", r.System, r.Value)
		return nil, status.Errorf(codes.NotFound, "user not found: %s|%s", r.System, r.Value)
	}
	if len(sr.Entries) > 1 {
		return nil, status.Errorf(codes.InvalidArgument, "more than one match for username %s", r.Value)
	}
	entry := sr.Entries[0]
	phones := make([]*apiv1.Telephone, 0)
	if n := entry.GetAttributeValue("mobile"); n != "" {
		phones = append(phones, &apiv1.Telephone{Number: n, Description: "Mobile"})
	}
	if n := entry.GetAttributeValue("telephoneNumber"); n != "" {
		phones = append(phones, &apiv1.Telephone{Number: n, Description: "Office"})
	}
	ids := make([]*apiv1.Identifier, 0)
	ids = append(ids, &apiv1.Identifier{
		System: identifiers.CymruUserID,
		Value:  entry.GetAttributeValue("sAMAccountName"),
	})
	//  bizarrely, the active directory uses postOfficeBox to store professional registration information
	if profReg := entry.GetAttributeValue("postOfficeBox"); profReg != "" && len(profReg) > 4 {
		switch {
		case strings.HasPrefix(profReg, "GMC:"):
			ids = append(ids, &apiv1.Identifier{
				System: identifiers.GMCNumber,
				Value:  strings.TrimSpace(profReg[4:]),
			})
		}
	}
	user := &apiv1.Practitioner{
		Active: true,
		Names: []*apiv1.HumanName{{
			Given:  entry.GetAttributeValue("givenName"),
			Family: entry.GetAttributeValue("sn"),
			Use:    apiv1.HumanName_OFFICIAL,
		},
		},
		Emails: []string{
			entry.GetAttributeValue("mail"),
		},
		Telephones:  phones,
		Identifiers: ids,
	}
	if title := entry.GetAttributeValue("title"); title != "" {
		user.Roles = []*apiv1.PractitionerRole{
			{Role: &apiv1.Role{JobTitle: title}},
		}
	}
	log.Printf("nadex: returning user: %+v", user)
	return user, nil
}

// GetFakePractitioner returns a fake practitioner, useful in testing without a live backend service
func (app *App) GetFakePractitioner(ctx context.Context, r *apiv1.Identifier) (*apiv1.Practitioner, error) {
	p := &apiv1.Practitioner{
		Active: true,
		Emails: []string{"wibble@wobble.org"},
		Names: []*apiv1.HumanName{
			{Given: "Fred", Family: "Flintstone", Prefixes: []string{"Mr"}},
		},
		Roles: []*apiv1.PractitionerRole{
			{Role: &apiv1.Role{JobTitle: "Consultant Neurologist"}},
		},
		Identifiers: []*apiv1.Identifier{
			{System: identifiers.CymruUserID, Value: r.GetValue()},
			{System: identifiers.GMCNumber, Value: "4624000"},
		},
	}
	log.Printf("nadex: returning fake practitioner: %+v", p)
	return p, nil
}

// Authenticate authenticates a user against the NHS Wales' directory service
func (app *App) Authenticate(id *apiv1.Identifier, credential string) (bool, error) {
	if id.GetSystem() != identifiers.CymruUserID {
		return false, fmt.Errorf("nadex: unsupported uri: %s", id.GetSystem())
	}
	if app.Fake {
		return credential == "password", nil
	}
	cfg, err := config.NewConfigFromString(krbConfig)
	if err != nil {
		return false, err
	}
	cl := client.NewClientWithPassword(id.GetValue(), "CYMRU.NHS.UK", credential, cfg, client.DisablePAFXFAST(true))
	err = cl.Login()
	if err != nil {
		return false, err
	}
	return true, nil
}
