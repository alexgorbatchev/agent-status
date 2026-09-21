package output_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/output"
	"github.com/alexgorbatchev/agent-status/packages/agent-debug/internal/scanner"
)

func TestRenderScannerAudit_Empty(t *testing.T) {
	var bufHuman bytes.Buffer
	output.RenderScannerAudit(&bufHuman, nil, false)
	if !strings.Contains(bufHuman.String(), "No processes or sessions found") {
		t.Errorf("expected empty message, got %s", bufHuman.String())
	}

	var bufAgent bytes.Buffer
	output.RenderScannerAudit(&bufAgent, []scanner.AuditRecord{}, true)
	if !strings.Contains(bufAgent.String(), "scanner_audit count:0") {
		t.Errorf("expected count:0, got %s", bufAgent.String())
	}
}

func TestRenderScannerAudit_Records(t *testing.T) {
	deltaPos := int64(82)
	deltaNeg := int64(-149)
	records := []scanner.AuditRecord{
		{
			Harness:       "pi",
			PID:           5998,
			CreatedAt:     1758400000000,
			CWD:           "/path/to/repo",
			SessionID:     "ses-123",
			DeltaMs:       &deltaPos,
			Status:        scanner.StatusPaired,
			StatusDetails: "ACTIVE",
		},
		{
			Harness:       "pi",
			PID:           27680,
			CreatedAt:     1758340000000,
			CWD:           "/path/to/repo",
			SessionID:     "ses-456",
			DeltaMs:       &deltaNeg,
			Status:        scanner.StatusPaired,
			StatusDetails: "IDLE 31h",
		},
		{
			Harness:   "claude",
			PID:       91201,
			CreatedAt: 1758410000000,
			CWD:       "/path/to/other",
			Status:    scanner.StatusOrphanProcess,
		},
		{
			Harness:   "codex",
			SessionID: "ses-orphan",
			CWD:       "/path/to/orphan",
			Status:    scanner.StatusOrphanSession,
		},
	}

	// 1. Human Mode
	var bufHuman bytes.Buffer
	output.RenderScannerAudit(&bufHuman, records, false)
	humanOut := bufHuman.String()
	if !strings.Contains(humanOut, "Process Discovery & Session Pairing Audit") {
		t.Errorf("expected title, got %s", humanOut)
	}
	if !strings.Contains(humanOut, "ses-123") || !strings.Contains(humanOut, "5998") {
		t.Errorf("missing record 1, got %s", humanOut)
	}
	if !strings.Contains(humanOut, "+82ms") || !strings.Contains(humanOut, "-149ms") {
		t.Errorf("missing delta formatting, got %s", humanOut)
	}
	if !strings.Contains(humanOut, "PAIRED (ACTIVE)") || !strings.Contains(humanOut, "PAIRED (IDLE 31h)") {
		t.Errorf("missing paired status details, got %s", humanOut)
	}
	if !strings.Contains(humanOut, "ORPHAN_PROCESS") || !strings.Contains(humanOut, "ORPHAN_SESSION") {
		t.Errorf("missing orphan statuses, got %s", humanOut)
	}

	// 2. Agent Mode
	var bufAgent bytes.Buffer
	output.RenderScannerAudit(&bufAgent, records, true)
	agentOut := bufAgent.String()
	if !strings.Contains(agentOut, "scanner_audit harness:pi pid:5998") {
		t.Errorf("missing agent record line, got %s", agentOut)
	}
	if !strings.Contains(agentOut, `delta_ms:82 status:PAIRED status_details:"ACTIVE"`) {
		t.Errorf("missing agent delta/details, got %s", agentOut)
	}
}
