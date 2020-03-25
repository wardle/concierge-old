package cmd

import (
	"log"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wardle/concierge/empi"
	"github.com/wardle/concierge/identifiers"
	"github.com/wardle/concierge/nadex"
	"github.com/wardle/concierge/server"
	"github.com/wardle/concierge/terminology"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts a server (gRPC and REST)",
	Long:  `Starts a server (gRPC and REST)`,
	Run: func(cmd *cobra.Command, args []string) {
		log.Printf("========== starting concierge ==========")
		my := createServers()

		// start server
		log.Printf("cmd: starting server: rpc-port:%d http-port:%d", my.sv.Options.RPCPort, my.sv.Options.RESTPort)
		if err := my.sv.RunServer(); err != nil {
			log.Fatal(err)
		}
		my.sv.Close()
	},
}

type myServer struct {
	sv *server.Server // the main gRPC/HTTP server
	// services
	identifiers *identifiers.Server // an identifier service
	nadex       *nadex.App
	empi        *empi.App
	term        *terminology.Terminology
}

// createServers creates a gRPC/HTTP server and plugs-in modular providers based on runtime configuration
func createServers() *myServer {
	sv := server.New(server.Options{
		RESTPort: viper.GetInt("port-http"),
		RPCPort:  viper.GetInt("port-grpc"),
		CertFile: viper.GetString("cert"),
		KeyFile:  viper.GetString("key"),
	})
	my := &myServer{
		sv: sv,
	}
	// generic servers: these are high-level and distinct from underlying implementations
	my.identifiers = &identifiers.Server{}
	my.sv.Register("identifier", my.identifiers)

	// specific servers: these provide an abstraction over a specific back-end service.
	// in the future, these endpoints will be deprecated in favour of complete abstraction,
	// but we will still need to support identifier resolution and mapping using this mechanism
	my.nadex = nadexServer()
	my.sv.Register("nadex", my.nadex)
	identifiers.RegisterResolver(identifiers.CymruUserID, my.nadex.ResolvePractitioner)

	my.empi = walesEmpiServer()
	//my.empi.Register("wales-empi", ep) 		-- temporarily unnecessary as can use identifier lookup instead
	identifiers.RegisterResolver(identifiers.NHSNumber, my.empi.ResolveIdentifier)
	identifiers.RegisterResolver(identifiers.CardiffAndValeURI, my.empi.ResolveIdentifier)
	identifiers.RegisterResolver(identifiers.AneurinBevanURI, my.empi.ResolveIdentifier)
	identifiers.RegisterResolver(identifiers.CwmTafURI, my.empi.ResolveIdentifier)
	identifiers.RegisterResolver(identifiers.SwanseaBayURI, my.empi.ResolveIdentifier)

	// terminology server
	if addr := viper.GetString("terminology-addr"); addr != "" {
		var err error
		my.term, err = terminology.NewTerminology(addr)
		if err != nil {
			log.Fatal(err)
		}
		identifiers.RegisterResolver(identifiers.SNOMEDCT, my.term.Resolve)
	} else {
		log.Printf("warning: running without terminology server")
	}
	// authentication
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
		my.sv.RegisterAuthenticator(auth)
		if db := viper.GetString("auth-db"); db != "" {
			ap, err := server.NewDatabaseAuthProvider(db)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("cmd: using postgresql ('%s') for service user authentication", db)
			auth.RegisterAuthProvider(identifiers.ConciergeServiceUser, "postgresql", ap, true)
		} else if hash := viper.GetString("auth-secret"); hash != "" {
			log.Printf("cmd: using explicitly defined single secret for service user authentication")
			auth.RegisterAuthProvider(identifiers.ConciergeServiceUser, "single", server.NewSingleAuthProvider(hash), true)
		} else {
			log.Fatalf("cmd: you must specify a authentication provider (--auth-db or --auth-secret) or specify --no-auth explicitly")
		}
		auth.RegisterAuthProvider(identifiers.CymruUserID, "nadex", my.nadex, false)
		my.sv.Register("auth", auth)
	}
	return my
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
	serveCmd.PersistentFlags().String("auth-db", "", "Auth database connection string (e.g. 'dbname=concierge sslmode=disable'")
	viper.BindPFlag("auth-db", serveCmd.PersistentFlags().Lookup("auth-db"))

}
