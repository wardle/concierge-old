/*
Package cmd supports the command-line interface for the concierge utility.

Copyright © 2020 Eldrix Ltd and Mark Wardle (mark@wardle.org)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

var cfgFile string
var Version string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "concierge",
	Short: "Concierge is a suite of health and care integration utilities",
	Long: `
Concierge is a suite of health and care integration modules, abstracting and
simplifing integrations with underlying health and care systems. 
	
A concierge assists guests. This concierge assists clients to integrate into
the local health and care ecosystem.
	
See https://github.com/wardle/concierge`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		warnIfHTTPProxy()
		if logfile := viper.GetString("log"); logfile != "" {
			f, err := os.OpenFile(logfile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
			if err != nil {
				log.Fatalf("fatal error: couldn't open log file ('%s'): %s", logfile, err)
			}
			log.SetOutput(f)
			log.SetFlags(log.LstdFlags | log.Lshortfile)
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	rootCmd.Version = Version
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.concierge.yaml)")

	rootCmd.PersistentFlags().String("log", "", "Log file to use")
	viper.BindPFlag("log", rootCmd.PersistentFlags().Lookup("log"))

	rootCmd.PersistentFlags().Bool("fake", false, "Run with fake results")
	viper.BindPFlag("fake", rootCmd.PersistentFlags().Lookup("fake"))

	// empi configuration
	rootCmd.PersistentFlags().String("empi-endpoint", "D", "EMPI endpoint - (P)roduction, (T)est or (D)evelopment")
	viper.BindPFlag("empi-endpoint", rootCmd.PersistentFlags().Lookup("empi-endpoint"))
	rootCmd.PersistentFlags().String("empi-endpoint-url", "", "URL for EMPI endpoint (if different to default for P/T/D")
	viper.BindPFlag("empi-endpoint-url", rootCmd.PersistentFlags().Lookup("empi-endpoint-url"))
	rootCmd.PersistentFlags().Int("empi-timeout-seconds", 2, "Timeout for calls to EMPI backend server endpoint(s)")
	viper.BindPFlag("empi-timeout-seconds", rootCmd.PersistentFlags().Lookup("empi-timeout-seconds"))
	rootCmd.PersistentFlags().Int("empi-cache-minutes", 5, "EMPI cache expiration in minutes, 0=no cache")
	viper.BindPFlag("empi-cache-minutes", rootCmd.PersistentFlags().Lookup("empi-cache-minutes"))

	// cav configuration
	rootCmd.PersistentFlags().String("cav-pms-username", "", "Username for CAV PMS")
	viper.BindPFlag("cav-pms-username", rootCmd.PersistentFlags().Lookup("cav-pms-username"))
	rootCmd.PersistentFlags().String("cav-pms-password", "", "Password for CAV PMS")
	viper.BindPFlag("cav-pms-password", rootCmd.PersistentFlags().Lookup("cav-pms-password"))

	// nadex configuration
	rootCmd.PersistentFlags().String("nadex-username", "", "Username for directory lookups")
	viper.BindPFlag("nadex-username", rootCmd.PersistentFlags().Lookup("nadex-username"))
	rootCmd.PersistentFlags().String("nadex-password", "", "Password for directory lookups")
	viper.BindPFlag("nadex-password", rootCmd.PersistentFlags().Lookup("nadex-password"))

	// SNOMED terminology server integration
	rootCmd.PersistentFlags().String("terminology-addr", "", "gRPC address of terminology server (e.g. localhost:8081")
	viper.BindPFlag("terminology-addr", rootCmd.PersistentFlags().Lookup("terminology-addr"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".go-empi" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".concierge")
	}

	viper.SetEnvPrefix("CONCIERGE")
	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}

// Log some important configuration variables which can cause live service failings.
// Directly use an environmental variable lookup, rather than viper, as that looks for upper case versions of the requested variable
func warnIfHTTPProxy() {
	httpProxy, exists := os.LookupEnv("http_proxy") // give warning if proxy set, to help debug connection errors in live
	if exists {
		log.Printf("warning: http proxy set to %s\n", httpProxy)
	}
	httpsProxy, exists := os.LookupEnv("https_proxy")
	if exists {
		log.Printf("warning: https proxy set to %s\n", httpsProxy)
	}
}
