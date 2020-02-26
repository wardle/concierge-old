/*

Package cmd provides the command-line commands and actions.

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
	"errors"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/wardle/concierge/empi"
)

// empiCmd is the "concierge test empi" command for simple testing of the EMPI at the command-line
var empiCmd = &cobra.Command{
	Use: "empi [authority] <identifier>", //(e.g. NHS 7253698428, NHS 7705820730, NHS 6145933267)
	Example: `concierge test empi NHS 7253698428
concierge test empi NHS 7705820730
concierge test empi NHS 6145933267
concierge test empi 7253698428`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 || len(args) > 3 {
			return errors.New("requires an an optional authority code and a mandatory identifier argument")
		}
		if len(args) == 2 && empi.LookupAuthority(args[0]) == empi.AuthorityUnknown {
			return fmt.Errorf("unsupported authority code: %s", args[0])
		}
		return nil
	},
	Short: "Test a query against the NHS Wales' EMPI",
	Long:  `Test a query against the NHS Wales' EMPI`,
	Run: func(cmd *cobra.Command, args []string) {
		authority := "NHS"
		var identifier string
		switch len(args) {
		case 1:
			identifier = args[0]
		case 2:
			authority = args[0]
			identifier = args[1]
		default:
			log.Fatalf("incorrect number of arguments: %v. expected [authority] identifier", args)
		}
		endpoint := empi.LookupEndpoint(cmd.Flag("endpoint").Value.String())
		endpointURL := endpoint.URL()
		if cmd.Flag("endpointURL").Value.String() != "" {
			endpointURL = cmd.Flag("endpointURL").Value.String()
		}
		log.Printf("executing against endpoint: %s, URL: %s", endpoint.Name(), endpointURL)
		if endpointURL == "" {
			log.Fatalf("invalid endpoint URL")
		}
		empi.Invoke(endpointURL, endpoint.ProcessingID(), authority, identifier)
	},
}

func init() {
	testCmd.AddCommand(empiCmd)
	empiCmd.PersistentFlags().String("endpoint", "D", "(P)roduction, (T)esting or (D)evelopment")
	empiCmd.MarkFlagRequired("endpoint")
	empiCmd.PersistentFlags().String("endpointURL", "", "URL for endpoint (if different to default for P/T/D")
}
