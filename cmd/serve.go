package cmd

import (
	"fmt"
	"log"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wardle/concierge/apiv1"
	"github.com/wardle/concierge/empi"
	"github.com/wardle/concierge/identifiers"
	"github.com/wardle/concierge/nadex"
	"github.com/wardle/concierge/server"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts a server (gRPC and REST)",
	Long:  `Starts a server (gRPC and REST)`,
	Run: func(cmd *cobra.Command, args []string) {
		log.Printf("========== starting concierge ==========")
		sv := server.Server{
			Options: server.Options{
				RESTPort: viper.GetInt("port-http"),
				RPCPort:  viper.GetInt("port-grpc"),
				CertFile: viper.GetString("cert"),
				KeyFile:  viper.GetString("key"),
			},
		}

		// generic servers: these are high-level and distinct from underlying implementations
		sv.Register("identifier", &identifiers.Server{})

		// specific servers: these provide an abstraction over a specific back-end service.
		// in the future, these endpoints will be deprecated in favour of complete abstraction,
		// but we will still need to support identifier resolution and mapping using this mechanism
		np := nadexServer()
		sv.Register("nadex", np)
		identifiers.RegisterResolver(identifiers.CymruUserID, np.ResolvePractitioner)
		ep := walesEmpiServer()
		//server.Register("wales-empi", ep) 		-- temporarily unnecessary as can use identifier lookup instead
		identifiers.RegisterResolver(identifiers.NHSNumber, ep.ResolveIdentifier)
		identifiers.RegisterResolver(identifiers.CardiffAndValeURI, ep.ResolveIdentifier)
		identifiers.RegisterResolver(identifiers.AneurinBevanURI, ep.ResolveIdentifier)
		identifiers.RegisterResolver(identifiers.CwmTafURI, ep.ResolveIdentifier)
		identifiers.RegisterResolver(identifiers.SwanseaBayURI, ep.ResolveIdentifier)

		var auth *server.Auth
		if viper.GetBool("no-auth") {
			log.Printf("cmd: warning: running without API authentication")
		} else {
			var err error
			jwtKey := viper.GetString("jwt-key")
			if jwtKey != "" {
				auth, err = server.NewAuthenticationServer(jwtKey)
			} else {
				log.Printf("warning: missing jwt-key: generating jwt tokens using temporary key")
				auth, err = server.NewAuthenticationServerWithTemporaryKey()
			}
			if err != nil {
				log.Fatalf("cmd: failed to start authentication server: %s", err)
			}
			sv.Auth = auth
			if db := viper.GetString("auth-db"); db != "" {
				ap, err := server.NewDatabaseAuthProvider(db)
				if err != nil {
					log.Fatal(err)
				}
				auth.RegisterAuthProvider(identifiers.ConciergeServiceUser, "postgresql", ap, true)
			} else {
				auth.RegisterAuthProvider(identifiers.ConciergeServiceUser, "stupid-simple", &stupidAuthProvider{}, true)
			}
			auth.RegisterAuthProvider(identifiers.CymruUserID, "nadex", np, false)
			sv.Register("auth", auth)
		}

		// start server
		log.Printf("cmd: starting server: rpc-port:%d http-port:%d", sv.Options.RPCPort, sv.Options.RESTPort)
		if err := sv.RunServer(); err != nil {
			log.Fatal(err)
		}
	},
}

func nadexServer() *nadex.App {
	nadexApp := new(nadex.App)
	nadexApp.Username = viper.GetString("nadex-username") // this will be fallback username/password to use
	nadexApp.Password = viper.GetString("nadex-password")
	nadexApp.Fake = viper.GetBool("fake")
	return nadexApp
}

func walesEmpiServer() *empi.App {
	empiApp := new(empi.App)
	empiEndpoint := viper.GetString("empi-endpoint")
	endpoint := empi.LookupEndpoint(empiEndpoint)
	if endpoint == empi.UnknownEndpoint {
		log.Fatalf("unknown endpoint: %v", empiEndpoint)
	}
	empiApp.Endpoint = endpoint
	if endpointURL := viper.GetString("empi-endpoint-url"); endpointURL != "" {
		empiApp.EndpointURL = endpointURL
	} else {
		empiApp.EndpointURL = endpoint.URL()
	}
	empiApp.Fake = viper.GetBool("fake")
	empiApp.TimeoutSeconds = viper.GetInt("empi-timeout-seconds")
	cacheMinutes := viper.GetInt("empi-cache-minutes")
	if cacheMinutes != 0 {
		empiApp.Cache = cache.New(time.Duration(cacheMinutes)*time.Minute, time.Duration(cacheMinutes*2)*time.Minute)
	}
	log.Printf("empi configuration: cache:%dm timeout:%ds endpoint:(%s)%s", cacheMinutes, empiApp.TimeoutSeconds, endpoint.Name(), empiApp.EndpointURL)
	return empiApp
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// core flags and configuration settings.
	serveCmd.PersistentFlags().Int("port-http", 8080, "Port to run HTTP server")
	viper.BindPFlag("port-http", serveCmd.PersistentFlags().Lookup("port-http"))
	serveCmd.PersistentFlags().Int("port-grpc", 9090, "Port to run gRPC server")
	viper.BindPFlag("port-grpc", serveCmd.PersistentFlags().Lookup("port-grpc"))
	serveCmd.PersistentFlags().Bool("fake", false, "Run a fake server")
	viper.BindPFlag("fake", serveCmd.PersistentFlags().Lookup("fake"))

	// SSL certificate configuration
	serveCmd.PersistentFlags().String("cert", "", "SSL certificate file (.cert)")
	viper.BindPFlag("cert", serveCmd.PersistentFlags().Lookup("cert"))
	serveCmd.PersistentFlags().String("key", "", "SSL certificate key file (.key)")
	viper.BindPFlag("key", serveCmd.PersistentFlags().Lookup("key"))

	// authentication configuration.
	serveCmd.PersistentFlags().Bool("no-auth", false, "Turn off API authentication: all API endpoints will be unprotected")
	viper.BindPFlag("no-auth", serveCmd.PersistentFlags().Lookup("no-auth"))
	serveCmd.PersistentFlags().String("jwt-key", "", "RSA key to use for signing and validating JWTs")
	viper.BindPFlag("jwt-key", serveCmd.PersistentFlags().Lookup("jwt-key"))

	// database authentication server options
	serveCmd.PersistentFlags().String("auth-db", "", "Auth database connection string")
	viper.BindPFlag("auth-db", serveCmd.PersistentFlags().Lookup("auth-db"))

	// empi configuration
	serveCmd.PersistentFlags().String("empi-endpoint", "D", "EMPI endpoint - (P)roduction, (T)est or (D)evelopment")
	viper.BindPFlag("empi-endpoint", serveCmd.PersistentFlags().Lookup("empi-endpoint"))
	serveCmd.PersistentFlags().String("empi-endpoint-url", "", "URL for EMPI endpoint (if different to default for P/T/D")
	viper.BindPFlag("empi-endpoint-url", serveCmd.PersistentFlags().Lookup("empi-endpoint-url"))
	serveCmd.PersistentFlags().Int("empi-timeout-seconds", 2, "Timeout for calls to EMPI backend server endpoint(s)")
	viper.BindPFlag("empi-timeout-seconds", serveCmd.PersistentFlags().Lookup("empi-timeout-seconds"))
	serveCmd.PersistentFlags().Int("empi-cache-minutes", 5, "EMPI cache expiration in minutes, 0=no cache")
	viper.BindPFlag("empi-cache-minutes", serveCmd.PersistentFlags().Lookup("empi-cache-minutes"))

	// nadex configuration
	serveCmd.PersistentFlags().String("nadex-username", "", "Username for directory lookups")
	viper.BindPFlag("nadex-username", serveCmd.PersistentFlags().Lookup("nadex-username"))
	serveCmd.PersistentFlags().String("nadex-password", "", "Password for directory lookups")
	viper.BindPFlag("nadex-password", serveCmd.PersistentFlags().Lookup("nadex-password"))
}

type stupidAuthProvider struct{}

// stupid authenticator for concierge service users - currently validates credentials stupidly - TODO: switch to client cert
func (sap *stupidAuthProvider) Authenticate(id *apiv1.Identifier, credential string) (bool, error) {
	log.Printf("danger: stupid authenticator implementation called for '%s|%s'", id.GetSystem(), id.GetValue())
	if id.GetSystem() != identifiers.ConciergeServiceUser {
		return false, fmt.Errorf("cannot authenticate for users in namespace uri '%s'", id.GetSystem())
	}
	if id.GetValue() == credential {
		log.Printf("auth: successful (but stupid) login for service user '%s'", id.GetValue())
		return true, nil
	}
	log.Printf("auth: failed login for service user '%s' : invalid credentials", id.GetValue())
	return false, status.Errorf(codes.PermissionDenied, "invalid user key and secret key")

}
