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
	"google.golang.org/protobuf/encoding/protojson"
)

// resolveCmd represents the resolve command
var resolveCmd = &cobra.Command{
	Use:   "resolve <system> <value>",
	Args:  cobra.ExactArgs(2),
	Short: "Resolve the value of an arbitrary identifier defined by a tuple of system (uri) and value",
	Long: `Resolve the value of an arbitrary identifier. 

For example, to test the EMPI resolution service:

concierge resolve https://fhir.nhs.uk/Id/nhs-number 7705820730
concierge resolve https://fhir.nhs.uk/Id/nhs-number 6145933267
concierge resolve https://fhir.nhs.uk/Id/nhs-number 7253698428

For example, to test the user directory service
concierge resolve https://fhir.nhs.uk/Id/cymru-user-id ma090906

Other tests:
concierge resolve http://snomed.info/sct 24700007
`,
	Run: func(cmd *cobra.Command, args []string) {
		my := createServers()
		my.sv.RegisterAuthenticator(nil) // turn off authentication
		v, err := my.identifiers.GetIdentifier(context.Background(), &apiv1.Identifier{System: args[0], Value: args[1]})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(protojson.MarshalOptions{Multiline: true, Indent: "  ", UseProtoNames: true}.Format(v))
	},
}

func init() {
	rootCmd.AddCommand(resolveCmd)

}
