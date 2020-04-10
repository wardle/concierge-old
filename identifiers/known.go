package identifiers

// list of built-in supported systems (although extendable at runtime and by importing other packages)
const (

	// generic
	URI   = "urn:ietf:rfc:3986" // general URI (uniform resource identifier)
	UUID  = "urn:uuid"          // a UUID as per https://tools.ietf.org/html/rfc4122
	OID   = "urn:oid"
	DICOM = "urn:dicom:uid"

	// health and care
	SNOMEDCT    = "http://snomed.info/sct"
	LOINC       = "http://loinc.org"
	ReadV2      = "http://read.info/readv2"
	ReadV3      = "http://read.info/ctv3"
	GMCNumber   = "https://fhir.hl7.org.uk/Id/gmc-number"
	NMCPIN      = "https://fhir.hl7.org.uk/Id/nmc-pin" // TODO: has anyone decided URIs for other authorities in UK?
	SDSUserID   = "https://fhir.nhs.uk/Id/sds-user-id"
	NHSNumber   = "https://fhir.nhs.uk/Id/nhs-number"
	ODSCode     = "https://fhir.nhs.uk/Id/ods-organization-code"
	ODSSiteCode = "https://fhir.nhs.uk/Id/ods-site-code"

	// NHS UK / NHS Digital URIs for specific value sets  (arguably all better as SCT identifiers)
	NHSNumberVerificationStatus = "https://fhir.hl7.org.uk/CareConnect-NHSNumberVerificationStatus-1"
	SDSJobRoleNameURI           = "https://fhir.nhs.uk/STU3/CodeSystem/CareConnect-SDSJobRoleName-1"
	CareConnectEthnicCategory   = "https://fhir.hl7.org.uk/CareConnect-EthnicCategory-1"

	// NHS Wales identifiers - I have made these up in the absence of any other published standard
	CymruUserID       = "https://fhir.nhs.uk/Id/cymru-user-id"
	CymruEmpiURI      = "https://fhir.wales.nhs.uk/Id/empi-number"            // ephemeral EMPI identifier
	CardiffAndValeCRN = "https://fhir.cardiff.wales.nhs.uk/Id/pas-identifier" // CAV PMS identifier
	SwanseaBayCRN     = "https://fhir.swansea.wales.nhs.uk/Id/pas-identifier"
	CwmTafCRN         = "https://fhir.cwmtaf.wales.nhs.uk/Id/pas-identifier"
	AneurinBevanCRN   = "https://fhir.aneurinbevan.nhs.uk/Id/pas-identifier"
	HywelDdaCRN       = "https://fhir.hyweldda.wales.nhs.uk/Id/pas-identifier"
	BetsiCentralCRN   = "https://fhir.betsicentral.wales.nhs.uk/Id/pas-identifier"
	BetsiMaelorCRN    = "https://fhir.betsimaelor.wales.nhs.uk/Id/pas-identifier"
	BetsiWestCRN      = "https://fhir.betsiwest.wales.nhs.uk/Id/pas-identifier"

	// Document repository identifiers
	CardiffAndValeDocID      = "https://fhir.cardiff.wales.nhs.uk/Id/document-identifier" // internal document identifier from CAV PMS
	CardiffAndValeClinicCode = "https://fhir.cardiff.wales.nhs.uk/Id/clinic-code"

	// Specific FHIR value sets
	CompositionStatus = "http://hl7.org/fhir/composition-status" // see https://www.hl7.org/fhir/valueset-composition-status.html

	// Concierge service user
	ConciergeServiceUser    = "https://concierge.eldrix.com/Id/service-user"
	ConciergeDocumentStatus = "https://concierge.eldrix.com/Id/document-status"
	PatientCare             = "https://patientcare.eldrix.com/Id/patientcare-application"
)
