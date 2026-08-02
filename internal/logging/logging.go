package logging

import (
	"io"

	"github.com/rs/zerolog"
)

// NewConsole creates the default server logger. ConsoleWriter keeps structured
// fields readable while adding colored levels and values.
func NewConsole(out io.Writer) *zerolog.Logger {
	console := zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.Out = out
	})
	logger := zerolog.New(console).With().Timestamp().Logger()
	return &logger
}
