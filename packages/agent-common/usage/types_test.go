package usage

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
)

func TestEmptyReportPreservesCoverage(t *testing.T) {
	for _, h := range []types.HarnessType{types.HarnessClaudeCode, types.HarnessPi, types.HarnessCodex, types.HarnessOpencode, "unknown"} {
		r := NewReport("agent", h)
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "null") || r.AgentID != "agent" || r.Harness != h {
			t.Fatalf("invalid empty report: %s", data)
		}
		for _, kind := range []string{"skills", "commands", "mcp"} {
			c := r.Coverage[kind]
			if c.Detail == "" || c.Status == "complete" || (h == "unknown" && c.Status != "unavailable") {
				t.Fatalf("empty observations falsely imply complete coverage: %+v", c)
			}
		}
	}
}
