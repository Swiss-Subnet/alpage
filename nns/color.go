package nns

import (
	"os"
)

// Color controls whether reconcile Render output wraps status fields in ANSI
// color. Off by default; the command layer sets it via DetectColor so library
// use and piped/redirected output stay plain.
var Color bool

const (
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
)

// DetectColor reports whether stdout is a terminal and NO_COLOR is unset, the
// usual gate for emitting ANSI. Stdlib-only: no isatty dependency.
func DetectColor() bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// colorize wraps s in an ANSI color when Color is on. s is expected to be
// already padded to its column width, so the escape bytes do not disturb
// alignment.
func colorize(s, ansi string) string {
	if !Color {
		return s
	}
	return ansi + s + ansiReset
}
