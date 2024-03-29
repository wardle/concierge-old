/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

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

	"github.com/spf13/cobra"
	"github.com/wardle/concierge/server"
)

// authCreateCmd allows generation of service user credentials
var authCreateCmd = &cobra.Command{
	Use:   "password",
	Short: "Generate random credentials",
	Args:  cobra.ExactArgs(0),

	Run: func(cmd *cobra.Command, args []string) {
		password, hash, err := server.GenerateCredentials()
		if err != nil {
			log.Fatalf("could not generate credentials: %s", err)
		}
		fmt.Printf("password : %s\n", password)
		fmt.Printf("hash     : %s\n", hash)
	},
}

func init() {
	authCmd.AddCommand(authCreateCmd)
}
