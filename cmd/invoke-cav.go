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
	"io/ioutil"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/wardle/concierge/apiv1"
	"github.com/wardle/concierge/cav"
	"github.com/wardle/concierge/identifiers"
	"google.golang.org/protobuf/encoding/protojson"
)

var invokeCavCmd = &cobra.Command{
	Use:   "cav",
	Short: "Invoke tests on CAV service",
}

var invokeCavdocCmd = &cobra.Command{
	Use:   "doc <username> <password> <crn (e.g. A888888)> <pdf-filename>",
	Short: "A runtime test of the CAV document service",
	Args:  cobra.ExactArgs(4),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		pms := cav.NewPMSService(args[0], args[1], 5*time.Second)
		pt, err := pms.FetchPatient(ctx, args[2])
		if err != nil {
			log.Fatal(err)
		}
		log.Print(protojson.Format(pt))

		pdf, err := ioutil.ReadFile(args[3])
		if err != nil {
			log.Fatal(err)
		}
		receipt, err := pms.PublishDocument(ctx, &apiv1.PublishDocumentRequest{
			Id:      &apiv1.Identifier{System: identifiers.UUID, Value: uuid.New().String()},
			Patient: pt,
			Title:   "Test letter from concierge",
			Data:    &apiv1.Attachment{ContentType: "application/pdf", Data: pdf},
		})
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("successfully published document: receipt: '%s|%s'", receipt.GetId().GetSystem(), receipt.GetId().GetValue())
	},
}

var invokeCavclinicCmd = &cobra.Command{
	Use:   "clinic <username> <password> <date (YYYY/MM/DD)> <clinic codes>...",
	Short: "A runtime invocation of the CAV document service",
	Args:  cobra.MinimumNArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		pms := cav.NewPMSService(args[0], args[1], 5*time.Second)
		date, err := time.Parse("2006/01/02", args[2])
		if err != nil {
			log.Fatal(err)
		}
		codes := make([]*apiv1.Identifier, 0)
		for _, code := range args[3:] {
			codes = append(codes, &apiv1.Identifier{
				System: identifiers.CardiffAndValeClinicCode,
				Value:  code,
			})
		}
		pts, err := pms.PatientsForClinics(ctx, date, codes)
		if err != nil {
			log.Fatal(err)
		}
		if len(pts) == 0 {
			log.Print("no patients for those clinics on that date")
		}
		for _, pt := range pts {
			log.Print(protojson.Format(pt))
		}
	},
}

func init() {
	invokeCmd.AddCommand(invokeCavCmd)
	invokeCavCmd.AddCommand(invokeCavdocCmd)
	invokeCavCmd.AddCommand(invokeCavclinicCmd)
}
