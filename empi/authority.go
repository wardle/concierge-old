package empi

import (
	"context"
	"fmt"

	"github.com/wardle/concierge/apiv1"
	"github.com/wardle/concierge/identifiers"
)

const (
	empiNamespaceURI      = "https://fhir.empi.wales.nhs.uk/Id/authority-code"      // this is made up
	authorityNamespaceURI = "https://fhir.eldrix.co.uk/concierge/Id/authority-code" // internal code
)

func init() {
	// register identifiers of the tuple empi-authority-code/organisation-code (https://fhir.wales.nhs.uk/empi-authority-code|140)  (for Cardiff and Vale)
	identifiers.Register("Wales EMPI authority", empiNamespaceURI)
	// map between above and a standard ODS identifier (https://fhir.nhs.uk/Id/ods-site-code|RWMBV)
	identifiers.RegisterMapper(empiNamespaceURI, identifiers.ODSSiteCode, func(ctx context.Context, empiID *apiv1.Identifier) (*apiv1.Identifier, error) {
		if empiID.System != empiNamespaceURI {
			return nil, fmt.Errorf("expected namespace: %s. got: %s. error:%w", empiNamespaceURI, empiID.System, identifiers.ErrNoMapper)
		}
		auth := lookupFromEmpiOrgCode(empiID.Value)
		if auth == AuthorityUnknown {
			return nil, fmt.Errorf("unable to map %s|%s to namespace %s", empiID.System, empiID.Value, identifiers.ODSSiteCode)
		}
		return auth.ToODSIdentifier(), nil
	})
}

// Authority represents the different authorities that issue identifiers
// These ultimately represent identifiers within the "system" https://fhir.nhs.uk/Id/ods-organization-code
// These are currently hard-coded, but this could easily be switched to a more modular extension registration
// approach based on runtime configuration
type Authority int

// List of authority codes for different organisations in Wales
const (
	AuthorityUnknown = iota
	AuthorityNHS
	AuthorityEMPI
	AuthorityABH
	AuthorityABMU
	AuthorityBCUCentral
	AuthorityBCUMaelor
	AuthorityBCUWest
	AuthorityCT
	AuthorityCV
	AuthorityHD
	AuthorityPowys
	lastAuthority
)

// ValidateIdentifier applies the authorities' formatting rules to validate and sanitise
// the identifier provided.
// Returns whether the identifier is valid and a sanitised version of that identifier.
func (a Authority) ValidateIdentifier(id string) (bool, string) {
	switch a {
	case AuthorityNHS:
		return ValidateNHSNumber(id)
	}
	return true, id
}

func (a Authority) empiOrganisationCode() string {
	if a > lastAuthority {
		return ""
	}
	return empiOrgCodes[a]
}

func (a Authority) odsHospitalCode() string {
	if a > lastAuthority {
		return ""
	}
	return hospitalCodes[a]
}
func (a Authority) typeCode() string {
	if a > lastAuthority {
		return ""
	}
	return authorityTypes[a]
}

// ToODSIdentifier converts the authority into a proper Identifier based on ODS code
// TODO: once using the new ODS microservice, check that it shouldn't be ODS site code
// TODO: plan migration to new ODS coding system (ANANA)
func (a Authority) ToODSIdentifier() *apiv1.Identifier {
	return &apiv1.Identifier{
		System: identifiers.ODSCode,
		Value:  a.odsHospitalCode(),
	}
}

// ToURI returns the URI for this authority
func (a Authority) ToURI() string {
	if a > lastAuthority {
		return ""
	}
	return uris[a]
}

// empiOrgCodes are the internal (proprietary) codes given to authorities within the Welsh EMPI
var empiOrgCodes = [...]string{
	"",
	"NHS", // NHS number
	"100", // internal EMPI identifier - this authority provides on ephemeral identifiers
	"139", // Aneurin Bevan (AB)
	"108", // Abertawe Bro Morgannwg (ABM)
	"109", // Betsi Cadwalader Central (BCUCentral)
	"110", // BCUMaelor
	"111", // BCUWest
	"126", // Cwm Taf (CT)
	"140", // Cardiff and Vale (CAV)
	"149", // Hywel Dda (HD)
	"170", // Powys
}

var uris = [...]string{
	"",
	identifiers.NHSNumber,
	identifiers.CymruEmpiURI,
	identifiers.AneurinBevanURI,
	identifiers.SwanseaBayURI,
	identifiers.BetsiCentralURI,
	identifiers.BetsiMaelorURI,
	identifiers.BetsiWestURI,
	identifiers.CwmTafURI,
	identifiers.CardiffAndValeURI,
	identifiers.HywelDdaURI,
	"", // don't thnk powys has a PAS!
}

// hospitalCodes provide ODS organisation codes
var hospitalCodes = [...]string{
	"",
	"NHS",
	"",
	"RVFAR", // Royal Gwent
	"RYMC7", // Morriston
	"",
	"",
	"",
	"RYLB3", // Prince Charles Hospital
	"RWMBV", // UHW
	"",
	"",
}
var empiOrgLookup = make(map[string]Authority)
var hospitalLookup = make(map[string]Authority)
var uriLookup = make(map[string]Authority)

func init() {
	for i, code := range empiOrgCodes {
		empiOrgLookup[code] = Authority(i)
	}
	for i, code := range hospitalCodes {
		hospitalLookup[code] = Authority(i)
	}
	for i, uri := range uris {
		uriLookup[uri] = Authority(i)
	}
}

var authorityTypes = [...]string{
	"",
	"NH",
	"PE", // unknown - TODO: check this
	"PI",
	"PI",
	"PI",
	"PI",
	"PI",
	"PI",
	"PI",
	"PI",
	"PI",
}

func lookupFromEmpiOrgCode(identifier string) Authority {
	if a, ok := empiOrgLookup[identifier]; ok {
		return a
	}
	return AuthorityUnknown
}

func lookupFromOdsHospital(identifier string) Authority {
	if a, ok := hospitalLookup[identifier]; ok {
		return a
	}
	return AuthorityUnknown
}
