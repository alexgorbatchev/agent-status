package logging

import "io"

// RunCommand initializes console logging before argument parsing, reports the
// returned error once, and closes any disk logger installed by run afterwards.
// Callers should silence the CLI framework's automatic error and usage output.
// It returns an exit status without exiting, so deferred cleanup can finish.
func RunCommand(service string, stderr io.Writer, run func() error) (code int) {
	console := New(stderr, service, "warn")
	SetDefault(console)
	defer func() {
		if err := Close(); err != nil {
			console.Error("log_close_failed", "err", err)
			code = 1
		}
	}()
	if err := run(); err != nil {
		Error("command_failed", "err", err)
		return 1
	}
	return 0
}
