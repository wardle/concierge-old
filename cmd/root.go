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
		logHTTPProxy()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.concierge.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
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
func logHTTPProxy() {
	httpProxy, exists := os.LookupEnv("http_proxy") // give warning if proxy set, to help debug connection errors in live
	if exists {
		log.Printf("warning: http proxy set to %s\n", httpProxy)
	}
	httpsProxy, exists := os.LookupEnv("https_proxy")
	if exists {
		log.Printf("warning: https proxy set to %s\n", httpsProxy)
	}
}