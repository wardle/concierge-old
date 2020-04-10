package main

import (
	"context"
	"errors"
	"log"

	"github.com/wardle/concierge/apiv1"
	"github.com/wardle/concierge/identifiers"
	"github.com/wardle/concierge/wales/cav"
	"github.com/wardle/concierge/wales/empi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// DocumentService is a document publication service; it currently publishes to Cardiff and Vale but
// is easily extendable to publish documents to other providers as well.
type DocumentService struct {
	cavpms *cav.PMSService
	empi   *empi.App
}

// matchingIdentifiers gives a list of identifiers that will be matched before a document is accepted.
var matchingIdentifiers = []string{
	identifiers.NHSNumber,
	identifiers.CardiffAndValeCRN,
	identifiers.CwmTafCRN,
	identifiers.AneurinBevanCRN,
}

// PublishDocument is the single abstract end-point for publishing documents via concierge.
// This endpoint will try to *do the right thing* based on the context.
// In the future, the choices might be delegated to a rule engine
// TODO: also send appropriate documents to GP/via the NHS Wales' ESB and the NHS England MESH framework
func (ds *DocumentService) PublishDocument(ctx context.Context, r *apiv1.PublishDocumentRequest) (*apiv1.PublishDocumentResponse, error) {
	doc := r.GetDocument()
	if doc == nil {
		return nil, status.Error(codes.InvalidArgument, "no document specified")
	}

	// if the patient has a Cardiff and Vale identifier, we can safely publish to that repository and
	// it is automatically propagated to the national NHS Wales repository.
	if _, found := doc.GetPatient().GetIdentifiersForSystem(identifiers.CardiffAndValeCRN); found {
		return ds.cavpms.PublishDocument(ctx, r)
	}

	// ok, our client failed to provide a Cardiff identifier, so we can double-check for a CAV registration
	// using the national EMPI... if we have an NHS Number
	if nhsIDs, found := doc.GetPatient().GetIdentifiersForSystem(identifiers.NHSNumber); found {
		if npt, err := ds.empi.GetEMPIRequest(ctx, nhsIDs[0]); err == nil {
			if doc.GetPatient().Match(npt, matchingIdentifiers) == false {
				log.Print("doc: fatal error when publishing document for patient: mismatched patient identifiers compared to EMPI")
				log.Printf("doc: from doc : %s", protojson.MarshalOptions{}.Format(doc.GetPatient()))
				log.Printf("doc: from empi: %s", protojson.MarshalOptions{}.Format(npt))
				return nil, errors.New("could not publish document: mismatched demographics between Cardiff and Vale and EMPI")
			}
			if cavIDs, found := npt.GetIdentifiersForSystem(identifiers.CardiffAndValeCRN); found {
				pt := proto.Clone(doc.GetPatient()).(*apiv1.Patient) // make a copy
				pt.Identifiers = append(pt.Identifiers, &apiv1.Identifier{
					System: identifiers.CardiffAndValeCRN,
					Value:  cavIDs[0].GetValue(),
				})
				r2 := proto.Clone(r).(*apiv1.PublishDocumentRequest)
				r2.GetDocument().Patient = pt
				return ds.cavpms.PublishDocument(ctx, r2)
			}
		}
	}

	// TODO: add WCRS (Welsh Care Records Service) integration / send to GP  / send to MESH / send to registered organisations / send to patient
	return nil, status.Error(codes.InvalidArgument, "Unable to publish document: no repository found to support patient with these identifiers")
}
