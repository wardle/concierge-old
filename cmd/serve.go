package cmd

import (
	"log"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wardle/concierge/empi"
	"github.com/wardle/concierge/server"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts a server (gRPC and REST)",
	Long:  `Starts a server (gRPC and REST)`,
	Run: func(cmd *cobra.Command, args []string) {
		app := new(empi.App)
		endpoint := empi.LookupEndpoint(viper.GetString("empi-endpoint"))
		if endpoint == empi.UnknownEndpoint {
			log.Fatalf("unknown endpoint: %v", cmd.Flag("empi-endpoint"))
		}
		app.Endpoint = endpoint
		if endpointURL := viper.GetString("empi-endpoint-url"); endpointURL != "" {
			app.EndpointURL = endpointURL
		} else {
			app.EndpointURL = endpoint.URL()
		}
		app.Fake = viper.GetBool("fake")
		app.TimeoutSeconds = viper.GetInt("empi-timeout-seconds")
		cacheMinutes := viper.GetInt("empi-cache-minutes")
		if cacheMinutes != 0 {
			app.Cache = cache.New(time.Duration(cacheMinutes)*time.Minute, time.Duration(cacheMinutes*2)*time.Minute)
		}
		server := server.Server{
			Options: server.Options{
				RESTPort: viper.GetInt("port-http"),
				RPCPort:  viper.GetInt("port-grpc"),
				CertFile: viper.GetString("cert"),
				KeyFile:  viper.GetString("key"),
			},
			WalesEMPIServer: app,
		}
		log.Printf("starting server: http-port:%d rpc-port:%d cache:%dm timeout:%ds endpoint:(%s)%s",
			server.Options.RPCPort, server.Options.RESTPort, cacheMinutes, app.TimeoutSeconds, endpoint.Name(), app.EndpointURL)
		log.Fatal(server.RunServer())
	},
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
}
