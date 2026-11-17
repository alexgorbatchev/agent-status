package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestHTTPErrorLogger(t *testing.T) {
	for _, agent := range []bool{false, true} {
		var out bytes.Buffer
		logger := NewWithMode(&out, "server", "warn", agent).With("listener", "test")
		HTTPErrorLogger(logger).Println("http: unexpected handler failure")
		if !strings.Contains(out.String(), "http_server_error") || !strings.Contains(out.String(), "detail=\"http: unexpected handler failure\"") || !strings.Contains(out.String(), "listener=test") {
			t.Fatalf("HTTP diagnostics did not retain structured context: %s", &out)
		}
		if agent && (!strings.Contains(out.String(), "level=ERROR") || !strings.Contains(out.String(), "action=http_server_error")) {
			t.Fatalf("HTTP diagnostics have incorrect action or level: %s", &out)
		}
	}
}

func TestHTTPErrorHandlerPreservesGroupsAndAttributes(t *testing.T) {
	var out bytes.Buffer
	h := httpErrorHandler{Handler: NewWithMode(&out, "server", "info", true).Handler()}
	handler := h.WithGroup("http").WithAttrs([]Attr{{Key: "listener", Value: slog.StringValue("test")}})
	if err := handler.Handle(context.Background(), slog.NewRecord(time.Now(), LevelError, "failure", 0)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "http.listener=test") || !strings.Contains(out.String(), "action=http_server_error") {
		t.Fatalf("HTTP handler lost context: %s", &out)
	}
}
