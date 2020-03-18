// Package identifiers provides a mechamism to support the arbitrary mapping and resolution
// of system/value tuples that act as identifiers (uniform resource identifiers).

package identifiers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	protov1 "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/wardle/concierge/apiv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var (
	systemsMu   sync.RWMutex
	systems     = make(map[string]apiv1.System)
	resolversMu sync.RWMutex
	resolvers   = make(map[string]func(ctx context.Context, id *apiv1.Identifier) (proto.Message, error))
	mappersMu   sync.RWMutex
	mappers     = make(map[string]func(ctx context.Context, id *apiv1.Identifier) (*apiv1.Identifier, error))
)

// ErrNoResolver is an error for when a valid resolver is not registered for the specified URI
var ErrNoResolver = errors.New("no resolver for uri")

// ErrNoMapper is an error when when a mapper is not registered to convert from the specified URI to another
var ErrNoMapper = errors.New("no mapper for uri")

// ErrNotFound is an error when an identifier is not found
var ErrNotFound = errors.New("identifier not found")

// Register registers an identifier system with the registry
func Register(name string, uri string) {
	systemsMu.Lock()
	defer systemsMu.Unlock()
	systems[uri] = apiv1.System{Name: name, Uri: uri}
}

// RegisterResolver registers a handler to resolve the value for the system/identifier tuple
func RegisterResolver(uri string, f func(ctx context.Context, id *apiv1.Identifier) (proto.Message, error)) {
	resolversMu.Lock()
	defer resolversMu.Unlock()
	if _, dup := resolvers[uri]; dup {
		panic("identifiers: register resolver called twice for URI " + uri)
	}
	resolvers[uri] = f
}

// Resolve attempts to resolve the specified system/value tuple
func Resolve(ctx context.Context, id *apiv1.Identifier) (proto.Message, error) {
	resolversMu.RLock()
	resolver, ok := resolvers[id.GetSystem()]
	resolversMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unable to resolve '%s|%s': %w", id.GetSystem(), id.GetValue(), ErrNoResolver)
	}
	return resolver(ctx, id)
}

func mapperKey(fromURI string, toURI string) string {
	return fromURI + "|" + toURI
}

// RegisterMapper registers a handler to map a value from one system to another
func RegisterMapper(fromURI string, toURI string, f func(context.Context, *apiv1.Identifier) (*apiv1.Identifier, error)) {
	mappersMu.Lock()
	defer mappersMu.Unlock()
	key := mapperKey(fromURI, toURI)
	if _, dup := mappers[key]; dup {
		panic("identifiers: register mapper called twice for URI " + fromURI)
	}
	mappers[key] = f
}

// Server is the identifier service that offers resolution and mapping of identifiers based on system/value tuples
type Server struct{}

var _ apiv1.IdentifiersServer = (*Server)(nil)

// RegisterServer registers this server
func (svc *Server) RegisterServer(s *grpc.Server) {
	for _, uri := range Resolvers() {
		log.Printf("identifiers: registered resolver for '%s'", uri)
	}
	for from, to := range Mappers() {
		log.Printf("identifiers: registered mapper for '%s'->'%s'", from, to)
	}
	apiv1.RegisterIdentifiersServer(s, svc)
}

// RegisterHTTPProxy registers this as a reverse HTTP proxy
func (svc *Server) RegisterHTTPProxy(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) error {
	return apiv1.RegisterIdentifiersHandlerFromEndpoint(ctx, mux, endpoint, opts)
}

// GetIdentifier resolves an identifier
func (svc *Server) GetIdentifier(ctx context.Context, id *apiv1.Identifier) (*any.Any, error) {
	if id.GetSystem() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "identifier: missing parameter: system")
	}
	o, err := Resolve(ctx, id)
	if err != nil {
		return nil, err
	}
	r, err := ptypes.MarshalAny(protov1.MessageV1(o))
	if err != nil {
		return nil, err
	}
	r.TypeUrl = strings.Replace(r.TypeUrl, "type.googleapis.com", "concierge.eldrix.com", 1)
	return r, nil
}

// MapIdentifier resolves an identifier
func (svc *Server) MapIdentifier(ctx context.Context, r *apiv1.IdentifierMapRequest) (*apiv1.Identifier, error) {
	id := &apiv1.Identifier{
		System: r.GetSystem(),
		Value:  r.GetValue(),
	}
	return Map(ctx, id, r.GetTargetUri())
}

// Map attempts to map an identifier from one code system to another
func Map(ctx context.Context, id *apiv1.Identifier, uri string) (*apiv1.Identifier, error) {
	if id.System == uri {
		return id, nil
	}
	key := mapperKey(id.System, uri)
	mappersMu.RLock()
	mapper, ok := mappers[key]
	mappersMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unable to map from '%s' to '%s':%w", id.System, uri, ErrNoMapper)
	}
	return mapper(ctx, id)
}

// Systems returns a list of the supported identifier systems
func Systems() []string {
	systemsMu.RLock()
	defer systemsMu.RUnlock()
	list := make([]string, 0, len(systems))
	for uri := range systems {
		list = append(list, uri)
	}
	sort.Strings(list)
	return list
}

// Resolvers returns the list of registered identifier resolvers
func Resolvers() []string {
	resolversMu.RLock()
	defer resolversMu.RUnlock()
	list := make([]string, 0, len(resolvers))
	for uri := range resolvers {
		list = append(list, uri)
	}
	sort.Strings(list)
	return list
}

// Mappers returns the list of registered identifier mappers
func Mappers() map[string]string {
	mappersMu.RLock()
	defer mappersMu.RUnlock()
	list := make(map[string]string)
	for m := range mappers {
		uris := strings.Split(m, "|")
		list[uris[0]] = uris[1]
	}
	return list
}

// Lookup returns the system for the specified uri
func Lookup(uri string) (*apiv1.System, bool) {
	systemsMu.RLock()
	defer systemsMu.RUnlock()
	val, ok := systems[uri]
	return &val, ok
}

func init() {
	// SNOMED CT concept identifiers and expressions (compositional grammar)
	Register("SNOMED CT", SNOMEDCT)
	// Read codes V2
	Register("Read V2", ReadV2)
	// Read codes CTV3
	Register("Read CTV3", ReadV3)
	// professional registration: General medical council (GMC)
	Register("GMC - General medical council", GMCNumber)
	// professional registration: Nursing and midwifery council (NMC)
	Register("NMC - Nursing and midwifery council", NMCPIN)
	// NHS England user directory
	Register("SDS", SDSUserID)
	// NHS Wales user directory
	Register("CYMRU", CymruUserID)
	// NHS England and Wales patient identifier
	Register("NHS number", NHSNumber)
	// Organisational data services code for an organisation
	Register("ODS code", ODSCode)
	// Organisational data services code for an organisational site
	Register("ODS site code", ODSSiteCode)
	// NHS number verification status - should be SNOMED CT and not a (semi-)proprietary value set
	Register("NHS number verification status", NHSNumberVerificationStatus)
}

// list of built-in supported systems (although extendable at runtime and by importing other packages)
const (
	SNOMEDCT    = "http://snomed.info/sct"
	ReadV2      = "http://read.info/readv2"
	ReadV3      = "http://read.info/ctv3"
	GMCNumber   = "https://fhir.hl7.org.uk/Id/gmc-number"
	NMCPIN      = "https://fhir.hl7.org.uk/Id/nmc-pin" // TODO: has anyone decided URIs for other authorities in UK?
	SDSUserID   = "https://fhir.nhs.uk/Id/sds-user-id"
	CymruUserID = "https://fhir.nhs.uk/Id/cymru-user-id" // TODO: I've made this one up; do we have one for Wales?
	NHSNumber   = "https://fhir.nhs.uk/Id/nhs-number"
	ODSCode     = "https://fhir.nhs.uk/Id/ods-organization-code"
	ODSSiteCode = "https://fhir.nhs.uk/Id/ods-site-code"

	// NHS UK / NHS Digital URIs for specific value sets  (arguably all better as SCT identifiers)
	NHSNumberVerificationStatus = "https://fhir.hl7.org.uk/CareConnect-NHSNumberVerificationStatus-1"
)
