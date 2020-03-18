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
	"github.com/wardle/concierge/nadex"
	"google.golang.org/protobuf/encoding/protojson"
)

// testNadexCmd represents the testNadex command
var testNadexCmd = &cobra.Command{
	Use:   "nadex <username> <password> <username>",
	Short: "Tests connectivity to the NHS Wales' national directory service (NADEX)",
	Long:  ``,
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("testNadex called")
		nadex := nadex.App{
			Username: args[0],
			Password: args[1],
			Fake:     false,
		}
		p, err := nadex.GetPractitioner(context.Background(), &apiv1.Identifier{
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
	testCmd.AddCommand(testNadexCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// testNadexCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// testNadexCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
