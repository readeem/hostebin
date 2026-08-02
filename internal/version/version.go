// Package version is the single place that carries build identity. Every
// build path — go install, the justfile, Docker, and GoReleaser — overrides
// these variables with -ldflags, so nothing else in the tree hardcodes a
// version string.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Version is the release version, normally the git tag without its "v" prefix.
// Override with:
//
//	-ldflags "-X github.com/readeem/hostebin/internal/version.Version=1.2.3"
var Version = ""

// Commit is the git commit the binary was built from.
var Commit = ""

// Date is the build or commit timestamp, ideally RFC 3339.
var Date = ""

// Get returns the version, falling back to the module version recorded by the
// Go toolchain so that `go install ...@v1.2.3` reports something useful even
// without ldflags.
func Get() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 12 {
				return "devel-" + s.Value[:12]
			}
		}
	}
	return "dev"
}

// String is the multi-value form printed by `hostebin version`.
func String() string {
	out := "hostebin " + Get()
	if Commit != "" {
		out += " (" + Commit
		if Date != "" {
			out += ", " + Date
		}
		out += ")"
	} else if Date != "" {
		out += " (" + Date + ")"
	}
	return fmt.Sprintf("%s %s %s/%s", out, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
