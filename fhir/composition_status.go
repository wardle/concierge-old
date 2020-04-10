// Package fhir provides concierge support for FHIR value lists
package fhir

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/wardle/concierge/apiv1"
	"github.com/wardle/concierge/identifiers"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// CompositionStatus represents a FHIR composition status
type CompositionStatus int

// List of composition statuses
const (
	CompositionStatusUnknown        CompositionStatus = iota // Unknown
	CompositionStatusPreliminary                             // Draft
	CompositionStatusFinal                                   // Final
	CompositionStatusAmended                                 // Amended
	CompositionStatusEnteredInError                          // Entered in error
	CompositionStatusLast
)

// Code returns the FHIR code for this composition status
func (cs CompositionStatus) Code() string {
	if cs >= CompositionStatusLast {
		return compositionStatusCodes[CompositionStatusUnknown]
	}
	return compositionStatusCodes[cs]
}

var compositionStatusLookup map[string]CompositionStatus

func init() {
	compositionStatusLookup = make(map[string]CompositionStatus)
	for i := CompositionStatusUnknown; i < CompositionStatusLast; i++ {
		compositionStatusLookup[compositionStatusCodes[i]] = i
	}
}

// LookupCompositionStatus maps a FHIR composition status code to CompositionStatus
func LookupCompositionStatus(code string) CompositionStatus {
	return compositionStatusLookup[code]
}

var compositionStatusCodes = [...]string{
	"unknown",
	"preliminary",
	"final",
	"amended",
	"entered-in-error",
}

// ToProtobuf maps this composition status to the concierge equivalent
func (cs CompositionStatus) ToConcierge() apiv1.Document_Status {
	if cs < CompositionStatusUnknown || cs >= CompositionStatusLast {
		return compositionStatusToConcierge[CompositionStatusUnknown]
	}
	return compositionStatusToConcierge[cs]
}

var compositionStatusToConcierge = [...]apiv1.Document_Status{
	apiv1.Document_UNKNOWN,
	apiv1.Document_DRAFT,
	apiv1.Document_FINAL,
	apiv1.Document_AMENDED,
	apiv1.Document_IN_ERROR,
}

// ToSctID returns the SNOMED identifier representing this composition status
func (cs CompositionStatus) ToSctID() int64 {
	if cs >= CompositionStatusLast {
		return compositionalStatusSNOMED[CompositionStatusUnknown]
	}
	return compositionalStatusSNOMED[cs]
}

func LookupCompositionStatusFromSctID(sctID int64) CompositionStatus {
	for cs := CompositionStatusUnknown; cs < CompositionStatusLast; cs++ {
		if compositionalStatusSNOMED[cs] == sctID {
			return cs
		}
	}
	return CompositionStatusUnknown
}

var compositionalStatusSNOMED = [...]int64{
	0,         // unknown
	445667001, // interim
	445665009, // final
	445584004, // amended
	0,         // no code for "in error" - TODO: contact SNOMED and revise report final status
}

// Title returns the human-readable title for this composition status
func (cs CompositionStatus) Title() string {
	if cs >= CompositionStatusLast {
		return compositionalStatusTitles[CompositionStatusUnknown]
	}
	return compositionalStatusTitles[cs]
}

var compositionalStatusTitles = [...]string{
	"Unknown",
	"Preliminary",
	"Final",
	"Amended",
	"Entered in error",
}

// ToResourceStatus maps a composition status to a generic FHIR resource status
func (cs CompositionStatus) ToResourceStatus() ResourceStatus {
	if cs >= CompositionStatusLast {
		return compositionalStatusResourceStatus[CompositionStatusUnknown]
	}
	return compositionalStatusResourceStatus[cs]
}

// map to and from FHIR resource status (http://hl7.org/fhir/ValueSet/resource-status)
var compositionalStatusResourceStatus = [...]ResourceStatus{
	ResourceStatusUnknown,
	ResourceStatusDraft,
	ResourceStatusComplete,
	ResourceStatusReplaced,
	ResourceStatusError,
}

func init() {
	identifiers.Register("FHIR composition status", identifiers.CompositionStatus)
	identifiers.RegisterResolver(identifiers.CompositionStatus, compositionStatusResolver)
	identifiers.RegisterMapper(identifiers.CompositionStatus, identifiers.SNOMEDCT, mapCompositionStatusToSNOMED)
	identifiers.RegisterMapper(identifiers.SNOMEDCT, identifiers.CompositionStatus, mapSNOMEDtoCompositionStatus)
}

func compositionStatusResolver(ctx context.Context, id *apiv1.Identifier) (proto.Message, error) {
	cs := LookupCompositionStatus(id.GetValue())
	if cs != CompositionStatusUnknown {
		log.Printf("fhir: resolving %s|%s to %s", id.System, id.Value, cs.ToConcierge())
		return &apiv1.Identifier{
			System: identifiers.ConciergeDocumentStatus,
			Value:  cs.ToConcierge().Enum().String(),
		}, nil
	}
	return nil, status.Errorf(codes.NotFound, "no composition status found matching code: '%s'", id.GetValue())
}

func mapCompositionStatusToSNOMED(ctx context.Context, id *apiv1.Identifier, f func(*apiv1.Identifier) error) error {
	sctID := LookupCompositionStatus(id.GetValue()).ToSctID()
	if sctID != 0 {
		f(&apiv1.Identifier{
			System: identifiers.SNOMEDCT,
			Value:  strconv.FormatInt(sctID, 10),
		})
	}
	return nil
}

func mapSNOMEDtoCompositionStatus(ctx context.Context, id *apiv1.Identifier, f func(*apiv1.Identifier) error) error {
	sctID, err := strconv.ParseInt(id.GetValue(), 10, 64)
	if err != nil {
		return fmt.Errorf("failed to map SCTID '%s':%w", id.GetValue(), err)
	}
	cs := LookupCompositionStatusFromSctID(sctID)
	if cs != CompositionStatusUnknown {
		f(&apiv1.Identifier{
			System: identifiers.CompositionStatus,
			Value:  cs.Code(),
		})
	}
	return nil
}
