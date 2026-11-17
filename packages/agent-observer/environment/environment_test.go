package environment

import (
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/config"
)

func TestID(t *testing.T) {
	want, err := config.GetEnvironmentID()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ID()
	if err != nil {
		t.Fatal(err)
	}
	if got == "" || got != want {
		t.Fatalf("ID() = %q, want observer identity %q", got, want)
	}
}
