package extensions

import (
	"errors"
	"testing"
	"time"
)

func TestCreateRejectsContractsEmptyAfterNormalization(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	in := Registration{
		Name:                 "Review lens",
		OperatorContact:      "ops@example.test",
		Capabilities:         []string{" \t"},
		CallbackURL:          "https://example.test/events",
		ActionURL:            "https://example.test/actions",
		RequestedPermissions: []Permission{{Resource: "pull_requests", Actions: []string{"read"}}},
		SupportedEvents:      []string{" \n"},
		CredentialRotation:   RotationPolicy{IntervalDays: 30},
	}
	if _, err = store.Create("0123456789abcdef0123456789abcdef", in, time.Now()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Create error = %v, want ErrInvalid", err)
	}
}
