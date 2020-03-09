package empi

// Authority represents the different authorities that issue identifiers
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

// ValidateIdentifier applies the authorities' formatting rules to validate and sanitise
// the identifier provided
func (a Authority) ValidateIdentifier(id string) (bool, string) {
	switch a {
	case AuthorityNHS:
		return ValidateNHSNumber(id)
	}
	return true, id
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
	"RVMBV", // UHW
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
func LookupAuthority(authority string) Authority {
	for i, a := range authorityCodes {
		if a == authority {
			return Authority(i)
		}
	}
	return AuthorityUnknown
}
