package sds

import (
	"context"
	"testing"

	"github.com/wardle/concierge/apiv1"
	"github.com/wardle/concierge/identifiers"
)

var tests = []struct {
	code       string
	jobTitle   string
	deprecated bool
}{
	{"R0030", "Professor", false},
	{"R0120", "Senior Registrar", true},
	{"R6300", "Sessional GP", false},
}

func TestRoleResolution(t *testing.T) {
	for _, test := range tests {
		o, err := identifiers.Resolve(context.Background(), &apiv1.Identifier{
			System: SDSJobRoleNameURI,
			Value:  test.code,
		})
		if err != nil {
			t.Fatal(err)
		}
		if o.ProtoReflect().Descriptor().FullName() != "apiv1.Role" {
			t.Fatalf("expected 'apiv1.Role' got: %s", o.ProtoReflect().Descriptor().FullName())
		}
		if role, ok := o.(*apiv1.Role); ok {
			if role.GetJobTitle() != test.jobTitle || role.GetDeprecated() != test.deprecated {
				t.Fatalf("expected: '%+v' got: '%+v'", test, role)
			}
		} else {
			t.Fatalf("expected 'apiv1.Role' got: %s", o.ProtoReflect().Descriptor().FullName())
		}

	}
}
