package terminology

import (
	"context"
	"fmt"
	"io"
	"log"
	"strconv"
	"time"

	"github.com/wardle/concierge/apiv1"
	"github.com/wardle/concierge/identifiers"
	"github.com/wardle/go-terminology/snomed"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// Terminology provides a SNOMED identifier resolution service
type Terminology struct {
	conn   *grpc.ClientConn
	client snomed.SnomedCTClient
}

// NewTerminology creates a new SNOMED identifier resolution service
func NewTerminology(addr string) (*Terminology, error) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	client := snomed.NewSnomedCTClient(conn)
	return &Terminology{conn: conn, client: client}, nil
}

// Close the connection to the terminology server
func (term *Terminology) Close() error {
	if term == nil {
		return nil
	}
	if term.conn == nil {
		return nil
	}
	return term.conn.Close()
}

// Resolve provides a resolution service for SNOMED CT identifiers (currently only concept identifiers, not expressions)
// TODO: support parsing expression using expression.Parse() once SNOMED toolchain
// supports deriving equivalent of an "ExtendedConcept" for any arbitrary expression
func (term *Terminology) Resolve(ctx context.Context, id *apiv1.Identifier) (proto.Message, error) {
	sctID, err := snomed.ParseAndValidate(id.GetValue())
	if err != nil {
		return nil, fmt.Errorf("could not resolve SNOMED CT: %w", err)
	}
	header := metadata.New(map[string]string{"accept-language": "en-GB"})
	ctx = metadata.NewOutgoingContext(ctx, header)
	if sctID.IsConcept() {
		ec, err := term.client.GetExtendedConcept(ctx, &snomed.SctID{Identifier: sctID.Integer()})
		if err != nil {
			return nil, fmt.Errorf("could not resolve SNOMED CT concept '%d': %w", sctID, err)
		}
		return ec, nil
	}
	if sctID.IsDescription() {
		d, err := term.client.GetDescription(ctx, &snomed.SctID{Identifier: sctID.Integer()})
		if err != nil {
			return nil, fmt.Errorf("could not resolve SNOMED CT description '%d': %w", sctID, err)
		}
		return d, nil
	}
	return nil, fmt.Errorf("could not resolve SNOMED CT entity '%d': only concepts and descriptions supported", sctID)
}

// SNOMEDCTtoReadV2 performs a crossmap from SNOMED to Read V2
func (term *Terminology) SNOMEDCTtoReadV2(ctx context.Context, id *apiv1.Identifier, f func(*apiv1.Identifier) error) error {
	sctID, err := snomed.ParseAndValidate(id.GetValue())
	if err != nil {
		return fmt.Errorf("could not parse SNOMED identifier: %w", err)
	}
	if sctID.IsConcept() == false {
		return fmt.Errorf("can map only concepts: '%d' not a concept", sctID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := term.client.CrossMap(ctx, &snomed.CrossMapRequest{
		ConceptId: sctID.Integer(),
		RefsetId:  900000000000497000,
	})
	if err != nil {
		return fmt.Errorf("crossmap error: %w", err)
	}
	for {
		item, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("crossmap error: %w", err)
		}
		err = f(&apiv1.Identifier{
			System: identifiers.ReadV2,
			Value:  item.GetSimpleMap().GetMapTarget(),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// ReadV2toSNOMEDCT performs a crossmap from  Read V2 to SNOMED CT
func (term *Terminology) ReadV2toSNOMEDCT(ctx context.Context, id *apiv1.Identifier, f func(*apiv1.Identifier) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := term.client.FromCrossMap(ctx, &snomed.TranslateFromRequest{S: id.GetValue(), RefsetId: 900000000000497000})
	if err != nil {
		return err
	}
	if len(response.GetTranslations()) == 0 {
		log.Printf("no translations found for map from '%s:%s' to '%s'", id.GetSystem(), id.GetValue(), identifiers.SNOMEDCT)
	}
	for _, t := range response.GetTranslations() {
		ref := t.GetReferenceSetItem().GetReferencedComponentId()
		if err := f(&apiv1.Identifier{System: identifiers.SNOMEDCT, Value: strconv.FormatInt(ref, 10)}); err != nil {
			return err
		}
	}
	return nil
}
