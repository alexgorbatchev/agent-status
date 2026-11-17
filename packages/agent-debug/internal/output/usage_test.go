package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/types"
	"github.com/alexgorbatchev/agent-status/packages/agent-common/usage"
)

type usageFailWriter struct{}

func (usageFailWriter) Write([]byte) (int, error) { return 0, errors.New("output closed") }

func TestRenderUsage(t *testing.T) {
	for _, mode := range []bool{false, true} {
		for _, empty := range []bool{false, true} {
			r := usage.NewReport("agent", types.HarnessPi)
			if !empty {
				r.Skills = append(r.Skills, usage.Entry{Name: "\x1b[31mreview\x1b[0m", Operation: "load", Count: 2, FirstUsedAt: 1000, LastUsedAt: 2000})
				r.Commands = append(r.Commands, usage.Entry{Name: "/review", Operation: "request", Count: 1})
				r.MCP = append(r.MCP, usage.Entry{Name: "list_issues", Server: "github", Operation: "invoke", Count: 1})
			}
			var out bytes.Buffer
			if err := RenderUsage(&out, *r, mode); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), "\x1b") || !strings.Contains(out.String(), "partial") {
				t.Fatalf("unsanitized output or missing coverage: %s", &out)
			}
			if !empty && (!strings.Contains(out.String(), "review") || !strings.Contains(out.String(), "github")) {
				t.Fatalf("missing observations: %s", &out)
			}
			if mode && strings.Contains(out.String(), "+---") {
				t.Fatalf("agent mode printed a table: %s", &out)
			}
			if err := RenderUsage(usageFailWriter{}, *r, mode); err == nil {
				t.Fatal("output failure swallowed")
			}
		}
	}
}
