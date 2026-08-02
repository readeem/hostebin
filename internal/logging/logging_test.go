package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewConsoleWritesColoredStructuredOutput(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	var output bytes.Buffer
	logger := NewConsole(&output)

	logger.Info().Str("address", ":8080").Msg("listening")

	got := output.String()
	for _, want := range []string{"\x1b[", "INF", "listening", "address=", ":8080"} {
		if !strings.Contains(got, want) {
			t.Errorf("log output %q does not contain %q", got, want)
		}
	}
}
