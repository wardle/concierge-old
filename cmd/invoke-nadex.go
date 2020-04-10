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
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/wardle/concierge/apiv1"
	"github.com/wardle/concierge/identifiers"
	"github.com/wardle/concierge/wales/nadex"
	"google.golang.org/protobuf/encoding/protojson"
)

var invokeNadexCmd = &cobra.Command{
	Use:   "nadex <username> <password> <username>",
	Short: "Tests connectivity to the NHS Wales' national directory service (NADEX)",
	Long:  ``,
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("testNadex called")
		n := nadex.App{
			Username: args[0],
			Password: args[1],
			Fake:     false,
		}
		// Attempt a simple authentication
		success, err := n.Authenticate(&apiv1.Identifier{
			System: identifiers.CymruUserID,
			Value:  args[0],
		}, args[1])
		if err != nil {
			log.Fatal(err)
		}
		if !success {
			log.Printf("authentication failed: invalid credentials")
		}
		// Attempt a user lookup by username
		p, err := n.GetPractitioner(context.Background(), &apiv1.Identifier{
			System: identifiers.CymruUserID,
			Value:  args[2],
		})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(protojson.Format(p))
	},
}

func init() {
	invokeCmd.AddCommand(invokeNadexCmd)
}
