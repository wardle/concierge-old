package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/wardle/concierge/apiv1"
	"github.com/wardle/concierge/identifiers"
)

func TestServiceLogin(t *testing.T) {
	auth, err := NewAuthenticationServerWithTemporaryKey()
	if err != nil {
		t.Fatal(err)
	}
	id := &apiv1.Identifier{
		System: identifiers.ConciergeServiceUser,
		Value:  "a123456789",
	}
	r, err := auth.Login(context.Background(), &apiv1.LoginRequest{
		User:     id,
		Password: "a123456789",
	})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("token: %s", r.GetToken())
	token := r.GetToken()
	user, err := auth.parseToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if user.authenticatedUser.GetSystem() != id.System || user.authenticatedUser.GetValue() != id.Value {
		t.Fatalf("did not get correct system/value identifier from token. got: %s|%s", user.authenticatedUser.GetSystem(), user.authenticatedUser.GetValue())
	}
}
