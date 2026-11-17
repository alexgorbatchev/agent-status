package output

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-common/usage"
)

func RenderUsage(w io.Writer, r usage.Report, agentMode bool) error {
	w = wrapWriter(w)
	if !agentMode {
		if _, err := fmt.Fprintf(w, "Usage for %s (%s)\n", SanitizeTranscriptText(r.AgentID), SanitizeTranscriptText(string(r.Harness))); err != nil {
			return err
		}
	}
	table := newTable(w, []string{"Kind", "Name", "Server", "Operation", "Count", "First used (UTC)", "Last used (UTC)"})
	count := 0
	for _, group := range []struct {
		kind    string
		entries []usage.Entry
	}{{"skill", r.Skills}, {"command", r.Commands}, {"mcp", r.MCP}} {
		for _, e := range group.entries {
			count++
			name, server, operation := SanitizeTranscriptText(e.Name), SanitizeTranscriptText(e.Server), SanitizeTranscriptText(e.Operation)
			if agentMode {
				if _, err := fmt.Fprintf(w, "kind:%s name:%q server:%q operation:%q count:%d first:%d last:%d source:%q\n", group.kind, name, server, operation, e.Count, e.FirstUsedAt, e.LastUsedAt, SanitizeTranscriptText(e.LatestEvidence.Source)); err != nil {
					return err
				}
			} else {
				if err := table.Append([]string{group.kind, name, server, operation, strconv.FormatInt(e.Count, 10), time.UnixMilli(e.FirstUsedAt).UTC().Format(time.RFC3339), time.UnixMilli(e.LastUsedAt).UTC().Format(time.RFC3339)}); err != nil {
					return err
				}
			}
		}
	}
	if !agentMode {
		if count == 0 {
			if _, err := fmt.Fprintln(w, "No usage observations recorded."); err != nil {
				return err
			}
		} else if err := table.Render(); err != nil {
			return err
		}
	}
	for _, kind := range []string{"skills", "commands", "mcp"} {
		c := r.Coverage[kind]
		if agentMode {
			if _, err := fmt.Fprintf(w, "coverage:%s status:%q detail:%q\n", kind, SanitizeTranscriptText(c.Status), SanitizeTranscriptText(c.Detail)); err != nil {
				return err
			}
		} else if _, err := fmt.Fprintf(w, "%s coverage: %s — %s\n", kind, SanitizeTranscriptText(c.Status), SanitizeTranscriptText(c.Detail)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "history:%q\n", SanitizeTranscriptText(r.History))
	return err
}
