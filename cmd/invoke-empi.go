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
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/wardle/concierge/apiv1"
	"github.com/wardle/concierge/identifiers"
	"github.com/wardle/concierge/wales/empi"
	"google.golang.org/protobuf/encoding/protojson"
)

// empiCmd is the "concierge test empi" command for simple testing of the EMPI at the command-line
var empiCmd = &cobra.Command{
	Use: "empi [uri] <identifier>",
	Example: `concierge test empi https://fhir.nhs.uk/Id/nhs-number 7253698428
concierge test empi https://fhir.nhs.uk/Id/nhs-number 7705820730
concierge test empi https://fhir.nhs.uk/Id/nhs-number 6145933267
concierge test empi 7253698428`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 || len(args) > 3 {
			return errors.New("requires an an optional authority uri and a mandatory identifier argument")
		}
		return nil
	},
	Short: "Test a query against the NHS Wales' EMPI",
	Long:  `Test a query against the NHS Wales' EMPI`,
	Run: func(cmd *cobra.Command, args []string) {
		system := identifiers.NHSNumber
		var value string
		switch len(args) {
		case 1:
			value = args[0]
		case 2:
			system = args[0]
			value = args[1]
		default:
			log.Fatalf("incorrect number of arguments: %v. expected [system] identifier", args)
		}
		endpointURL := cmd.Flag("endpointURL").Value.String()
		processingID := cmd.Flag("processingID").Value.String()
		log.Printf("executing against endpoint: %s processing ID: %s", endpointURL, processingID)
		empiSvc := empi.App{EndpointURL: endpointURL, ProcessingID: processingID}
		pt, err := empiSvc.GetEMPIRequest(context.Background(), &apiv1.Identifier{System: system, Value: value})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(protojson.Format(pt))
	},
}

func init() {
	invokeCmd.AddCommand(empiCmd)
	empiCmd.PersistentFlags().String("endpointURL", "", "URL for endpoint (if different to default for P/T/D")
	empiCmd.MarkFlagRequired("endpointURL")
	empiCmd.PersistentFlags().String("processingID", "", "processing ID. P:production U:user acceptance testing T:development")
	empiCmd.MarkFlagRequired("processingID")
}
