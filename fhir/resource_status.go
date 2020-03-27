package fhir

// ResourceStatus represents a FHIR composition status
type ResourceStatus int

const (
	ResourceStatusUnknown ResourceStatus = iota
	ResourceStatusError
	ResourceStatusProposed
	ResourceStatusPlanned
	ResourceStatusDraft
	ResourceStatusRequested
	ResourceStatusReceived
	ResourceStatusDeclined
	ResourceStatusAccepted
	ResourceStatusArrived
	ResourceStatusActive
	ResourceStatusSuspended
	ResourceStatusFailed
	ResourceStatusReplaced
	ResourceStatusComplete
	ResourceStatusInactive
	ResourceStatusAbandoned
	ResourceStatusUnconfirmed
	ResourceStatusConfirmed
	ResourceStatusResolved
	ResourceStatusRefuted
	ResourceStatusDifferential
	ResourceStatusPartial
	ResourceStatusBusyUnavailable
	ResourceStatusFree
	ResourceStatusOnTarget
	ResourceStatusAheadOfTarget
	ResourceStatusBehindTarget
	ResourceStatusNotReady
	ResourceStatusTransducDiscon
	ResourceStatusHwDiscon
	ResourceStatusLast
)

// Code returns the code for this resource status
func (rs ResourceStatus) Code() string {
	if rs < ResourceStatusUnknown || rs >= ResourceStatusLast {
		return resourceStatusCodes[ResourceStatusUnknown]
	}
	return resourceStatusCodes[rs]
}

var resourceStatusLookup map[string]ResourceStatus

func init() {
	resourceStatusLookup = make(map[string]ResourceStatus)

	for i := ResourceStatusUnknown; i < ResourceStatusLast; i++ {
		resourceStatusLookup[resourceStatusCodes[i]] = i
	}
}

// LookResourceStatus returns the ResourceStatus for the specified code
func LookupResourceStatus(code string) ResourceStatus {
	return resourceStatusLookup[code]
}

var resourceStatusCodes = [...]string{
	"unknown",
	"error",
	"proposed",
	"planned",
	"draft",
	"requested",
	"received",
	"declined",
	"accepted",
	"arrived",
	"active",
	"suspended",
	"failed",
	"replaced",
	"complete",
	"inactive",
	"abandoned",
	"unconfirmed",
	"confirmed",
	"resolved",
	"refuted",
	"differential",
	"partial",
	"busy-unavailable",
	"free",
	"on-target",
	"ahead-of-target",
	"behind-target",
	"not-ready",
	"transduc-discon",
	"hw-discon",
}
