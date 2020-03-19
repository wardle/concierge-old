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
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts a server (gRPC and REST)",
	Long:  `Starts a server (gRPC and REST)`,
	Run: func(cmd *cobra.Command, args []string) {

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

		ep := walesEmpiServer()
		//server.Register("wales-empi", ep) 		-- temporarily unnecessary as can use identifier lookup instead
		identifiers.RegisterResolver(identifiers.NHSNumber, ep.ResolveIdentifier)
		identifiers.RegisterResolver(identifiers.CardiffAndValeURI, ep.ResolveIdentifier)
		identifiers.RegisterResolver(identifiers.AneurinBevanURI, ep.ResolveIdentifier)
		identifiers.RegisterResolver(identifiers.CwmTafURI, ep.ResolveIdentifier)
		identifiers.RegisterResolver(identifiers.SwanseaBayURI, ep.ResolveIdentifier)

		np := nadexServer()
		sv.Register("nadex", np)
		identifiers.RegisterResolver(identifiers.CymruUserID, np.ResolvePractitioner)

		auth, err := server.NewAuthenticationServerWithTemporaryKey() // TODO: option to turn off
		if err != nil {
			log.Fatalf("cmd: failed to start authentication server: %s", err)
		}
		sv.Auth = auth
		sv.Register("auth", auth)

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

	// Here you will define your flags and configuration settings.
	serveCmd.PersistentFlags().String("empi-endpoint", "D", "EMPI endpoint - (P)roduction, (T)est or (D)evelopment")
	viper.BindPFlag("empi-endpoint", serveCmd.PersistentFlags().Lookup("empi-endpoint"))
	serveCmd.PersistentFlags().String("empi-endpoint-url", "", "URL for EMPI endpoint (if different to default for P/T/D")
	viper.BindPFlag("empi-endpoint-url", serveCmd.PersistentFlags().Lookup("empi-endpoint-url"))
	serveCmd.PersistentFlags().Int("port-http", 8080, "Port to run HTTP server")
	viper.BindPFlag("port-http", serveCmd.PersistentFlags().Lookup("port-http"))
	serveCmd.PersistentFlags().Int("port-grpc", 9090, "Port to run gRPC server")
	viper.BindPFlag("port-grpc", serveCmd.PersistentFlags().Lookup("port-grpc"))
	serveCmd.PersistentFlags().Int("empi-timeout-seconds", 2, "Timeout for calls to EMPI backend server endpoint(s)")
	viper.BindPFlag("empi-timeout-seconds", serveCmd.PersistentFlags().Lookup("empi-timeout-seconds"))
	serveCmd.PersistentFlags().Int("empi-cache-minutes", 5, "EMPI cache expiration in minutes, 0=no cache")
	viper.BindPFlag("empi-cache-minutes", serveCmd.PersistentFlags().Lookup("empi-cache-minutes"))
	serveCmd.PersistentFlags().Bool("fake", false, "Run a fake server")
	viper.BindPFlag("fake", serveCmd.PersistentFlags().Lookup("fake"))
	serveCmd.PersistentFlags().String("cert", "", "SSL certificate file (.cert)")
	viper.BindPFlag("cert", serveCmd.PersistentFlags().Lookup("cert"))
	serveCmd.PersistentFlags().String("key", "", "SSL certificate key file (.key)")
	viper.BindPFlag("key", serveCmd.PersistentFlags().Lookup("key"))
	serveCmd.PersistentFlags().String("nadex-username", "", "Username for directory lookups")
	viper.BindPFlag("nadex-username", serveCmd.PersistentFlags().Lookup("nadex-username"))
	serveCmd.PersistentFlags().String("nadex-password", "", "Password for directory lookups")
	viper.BindPFlag("nadex-password", serveCmd.PersistentFlags().Lookup("nadex-password"))
}
