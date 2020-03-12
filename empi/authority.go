package empi

import "github.com/wardle/concierge/apiv1"

const (
	odsSystem = "https://fhir.nhs.uk/Id/ods-organization-code"
)

// Authority represents the different authorities that issue identifiers
// These represent identifiers within the "system" https://fhir.nhs.uk/Id/ods-organization-code
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
	LastAuthority
)

var protobufCodes = []apiv1.CymruEmpiRequest_Authority{
	apiv1.CymruEmpiRequest_UNKNOWN,
	apiv1.CymruEmpiRequest_NHS_NUMBER,
	apiv1.CymruEmpiRequest_UNKNOWN,
	apiv1.CymruEmpiRequest_ANEURIN_BEVAN,
	apiv1.CymruEmpiRequest_SWANSEA,
	apiv1.CymruEmpiRequest_BCU_CENTRAL,
	apiv1.CymruEmpiRequest_BCU_MAELOR,
	apiv1.CymruEmpiRequest_BCU_WEST,
	apiv1.CymruEmpiRequest_CWM_TAF,
	apiv1.CymruEmpiRequest_CARDIFF_AND_VALE,
	apiv1.CymruEmpiRequest_HYWEL_DDA,
	apiv1.CymruEmpiRequest_POWYS,
}

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

func (a Authority) authorityCode() string {
	if a > LastAuthority {
		return ""
	}
	return authorityCodes[a]
}

func (a Authority) hospitalCode() string {
	if a > LastAuthority {
		return ""
	}
	return hospitalCodes[a]
}
func (a Authority) typeCode() string {
	if a > LastAuthority {
		return ""
	}
	return authorityTypes[a]
}

var authorityCodes = [...]string{
	"",
	"NHS", // NHS number
	"100", // internal EMPI identifier - ephemeral identifier
	"139", // AB
	"108", // ABM
	"109", //BCUCentral
	"110", //BCUMaelor
	"111", //BCUWest
	"126", //CT
	"140", //CAV
	"149", //HD
	"170", //Powys
}

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

// LookupAuthority looks up an authority via a code
func LookupAuthority(system string, identifier string) Authority {
	if system == "" || system == odsSystem {
		for i, a := range authorityCodes {
			if a == identifier {
				return Authority(i)
			}
		}
	}
	return AuthorityUnknown
}
