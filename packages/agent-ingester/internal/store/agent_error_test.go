package store

import (
	"errors"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"testing"
)

func TestAgentMutationErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Store) (*types.IAgentState, error)
	}{
		{"visibility", func(st *Store) (*types.IAgentState, error) { return st.SetAgentVisibility("missing", true, nil) }},
		{"dismissal", func(st *Store) (*types.IAgentState, error) { return st.DismissAgentCompletion("missing") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, err := Open(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			if agent, err := tc.mutate(st); agent != nil || !errors.Is(err, ErrAgentNotFound) {
				t.Fatalf("missing agent: got %v, %v; want ErrAgentNotFound", agent, err)
			}
			if err := st.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := tc.mutate(st); err == nil || errors.Is(err, ErrAgentNotFound) {
				t.Fatalf("closed database: got %v; want distinct storage error", err)
			}
		})
	}
}
