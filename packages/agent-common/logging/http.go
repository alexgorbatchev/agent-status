package logging

import (
	"context"
	"log"
	"log/slog"
)

// HTTPErrorLogger adapts net/http's ErrorLog to the shared structured logger.
// Keep the native HTTP diagnostic in detail and a stable action for filtering.
func HTTPErrorLogger(logger *Logger) *log.Logger {
	return slog.NewLogLogger(httpErrorHandler{Handler: logger.Handler()}, LevelError)
}

type httpErrorHandler struct{ Handler }

func (h httpErrorHandler) Handle(ctx context.Context, r slog.Record) error {
	r.AddAttrs(slog.String("detail", r.Message))
	r.Message = "http_server_error"
	return h.Handler.Handle(ctx, r)
}

func (h httpErrorHandler) WithAttrs(attrs []Attr) Handler {
	return httpErrorHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h httpErrorHandler) WithGroup(name string) Handler {
	return httpErrorHandler{Handler: h.Handler.WithGroup(name)}
}
