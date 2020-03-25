package terminology

import (
	"context"
	"fmt"

	"github.com/wardle/concierge/apiv1"
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
	ec, err := term.client.GetExtendedConcept(ctx, &snomed.SctID{Identifier: sctID.Integer()})
	if err != nil {
		return nil, fmt.Errorf("could not resolve SNOMED CT '%s': %w", id.GetValue(), err)
	}
	return ec, nil
}
