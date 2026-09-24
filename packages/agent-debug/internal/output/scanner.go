package output

import (
	"fmt"
	"io"
	"time"

	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/scanner"
)

// RenderScannerAudit renders process pairing audit records in human or agent mode.
func RenderScannerAudit(w io.Writer, records []scanner.AuditRecord, agentMode bool) {
	if len(records) == 0 {
		if agentMode {
			_, _ = fmt.Fprintln(w, "scanner_audit count:0")
			return
		}
		RenderDivider(w, '-')
		_, _ = fmt.Fprintln(w, "No processes or sessions found matching filter.")
		RenderDivider(w, '-')
		return
	}

	if agentMode {
		for _, r := range records {
			deltaStr := "-"
			if r.DeltaMs != nil {
				deltaStr = fmt.Sprintf("%d", *r.DeltaMs)
			}
			_, _ = fmt.Fprintf(w, "scanner_audit harness:%s pid:%d created_at:%d cwd:%q session_id:%q delta_ms:%s status:%s status_details:%q\n",
				SanitizeTranscriptText(r.Harness), r.PID, r.CreatedAt, SanitizeTranscriptText(r.CWD), SanitizeTranscriptText(r.SessionID), deltaStr, r.Status, SanitizeTranscriptText(r.StatusDetails))
		}
		return
	}

	RenderDivider(w, '-')
	_, _ = fmt.Fprintln(w, "Process Discovery & Session Pairing Audit")
	RenderDivider(w, '-')

	table := newTable(w, []string{
		"Harness", "PID", "Created At", "CWD", "Session ID", "Delta", "Status",
	})

	for _, r := range records {
		pidStr := "-"
		if r.PID > 0 {
			pidStr = fmt.Sprintf("%d", r.PID)
		}

		createdStr := "-"
		if r.CreatedAt > 0 {
			createdStr = time.UnixMilli(r.CreatedAt).UTC().Format("2006-01-02 15:04:05")
		}

		sessID := "-"
		if clean := SanitizeTranscriptText(r.SessionID); clean != "" {
			sessID = clean
		}

		deltaStr := "-"
		if r.DeltaMs != nil {
			if *r.DeltaMs > 0 {
				deltaStr = fmt.Sprintf("+%dms", *r.DeltaMs)
			} else {
				deltaStr = fmt.Sprintf("%dms", *r.DeltaMs)
			}
		}

		statusStr := string(r.Status)
		if r.Status == scanner.StatusPaired && r.StatusDetails != "" {
			statusStr = fmt.Sprintf("PAIRED (%s)", SanitizeTranscriptText(r.StatusDetails))
		}

		_ = table.Append([]string{
			SanitizeTranscriptText(r.Harness),
			pidStr,
			createdStr,
			SanitizeTranscriptText(r.CWD),
			sessID,
			deltaStr,
			statusStr,
		})
	}
	_ = table.Render()
}
