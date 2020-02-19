package cmd

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/patrickmn/go-cache"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wardle/concierge/empi"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts a REST server",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		sigs := make(chan os.Signal, 1) // channel to receive OS termination/kill/interrupt signal
		signal.Notify(sigs, os.Interrupt, os.Kill, syscall.SIGTERM)
		go func() {
			s := <-sigs
			log.Printf("RECEIVED SIGNAL: %s", s)
			os.Exit(1)
		}()
		app := new(empi.App)
		endpoint := empi.LookupEndpoint(viper.GetString("endpoint"))
		if endpoint == empi.UnknownEndpoint {
			log.Fatalf("unknown endpoint: %v", cmd.Flag("endpoint"))
		}
		app.Endpoint = endpoint
		app.Router = mux.NewRouter().StrictSlash(true)
		app.Fake = viper.GetBool("fake")
		app.TimeoutSeconds = viper.GetInt("timeoutSeconds")
		cacheMinutes := viper.GetInt("cacheMinutes")
		if cacheMinutes != 0 {
			app.Cache = cache.New(time.Duration(cacheMinutes)*time.Minute, time.Duration(cacheMinutes*2)*time.Minute)
		}
		port := viper.GetInt("port")
		app.Router.HandleFunc("/nhsnumber/{nnn}", app.GetByNhsNumber).Methods("GET")
		app.Router.HandleFunc("/authority/{authorityCode}/{identifier}", app.GetByIdentifier).Methods("GET")
		log.Printf("starting REST server: port:%d cache:%dm timeout:%ds endpoint:(%s)%s",
			port, cacheMinutes, app.TimeoutSeconds, endpoint.Name(), endpoint.URL())
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", viper.GetInt("port")), app.Router))
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Here you will define your flags and configuration settings.
	serveCmd.PersistentFlags().String("endpoint", "D", "(P)roduction, (T)est or (D)evelopment")
	viper.BindPFlag("endpoint", serveCmd.PersistentFlags().Lookup("endpoint"))
	serveCmd.PersistentFlags().Int("port", 8080, "Port to run HTTP server")
	viper.BindPFlag("port", serveCmd.PersistentFlags().Lookup("port"))
	serveCmd.PersistentFlags().Int("timeoutSeconds", 2, "Timeout for calls to backend server endpoint(s)")
	viper.BindPFlag("timeoutSeconds", serveCmd.PersistentFlags().Lookup("timeoutSeconds"))
	serveCmd.PersistentFlags().Int("cacheMinutes", 5, "cache expiration in minutes, 0=no cache")
	viper.BindPFlag("cacheMinutes", serveCmd.PersistentFlags().Lookup("cacheMinutes"))
	serveCmd.PersistentFlags().Bool("fake", false, "Run a fake server")
	viper.BindPFlag("fake", serveCmd.PersistentFlags().Lookup("fake"))
}
