// Package version exposes build metadata, injected at link time.
//
// The values below are the defaults for a plain `go build`. Release builds and
// the release workflow override them with -ldflags, so that `vyom version` and
// the /api/v1/version endpoint report exactly which binary an operator is
// running.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// Build information. Override with:
//
//	go build -ldflags "-X github.com/kaushalacts/vyom/internal/version.Version=v1.2.3 \
//	                   -X github.com/kaushalacts/vyom/internal/version.Commit=$(git rev-parse HEAD) \
//	                   -X github.com/kaushalacts/vyom/internal/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
var (
	// Version is the semantic version of this build.
	Version = "v0.1.0"
	// Commit is the git commit SHA of this build, when available.
	Commit = ""
	// Date is the RFC 3339 build timestamp, when available.
	Date = ""
)

// These are the dependency-free fallbacks used when the binary was built
// without the release ldflags.
const (
	defaultVersion = "v0.1.0"
	unknown        = "unknown"
)

// Info is the full build description returned by `vyom version`.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
	// Module is the Go module path, always present in released binaries.
	Module string `json:"module"`
	// Modified reports whether the working tree was dirty at build time,
	// which matters when triaging a bug report from a developer build.
	Modified bool `json:"modified"`
}

// Module is Vyom's Go module path.
const Module = "github.com/kaushalacts/vyom"

// Get returns the build information, falling back to the module's VCS stamp
// recorded by the Go toolchain when the linker flags were not supplied.
func Get() Info {
	info := Info{
		Version:   strings.TrimSpace(Version),
		Commit:    strings.TrimSpace(Commit),
		Date:      strings.TrimSpace(Date),
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
		Module:    Module,
	}
	if info.Version == "" {
		info.Version = defaultVersion
	}
	if info.Commit == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				switch s.Key {
				case "vcs.revision":
					info.Commit = s.Value
				case "vcs.time":
					if info.Date == "" {
						info.Date = s.Value
					}
				case "vcs.modified":
					info.Modified = s.Value == "true"
				}
			}
		}
	}
	if info.Commit == "" {
		info.Commit = unknown
	}
	if info.Date == "" {
		info.Date = unknown
	}
	return info
}

// ShortCommit returns at most 12 characters of the commit SHA.
func ShortCommit() string {
	c := strings.TrimSpace(Get().Commit)
	if len(c) > 12 {
		return c[:12]
	}
	return c
}

// String renders a single line suitable for CLI headers and log startup lines.
func (i Info) String() string {
	return fmt.Sprintf("vyom %s (commit %s, built %s, %s, %s)",
		i.Version, i.Commit, i.Date, i.GoVersion, i.Platform)
}

// Banner returns the multi-line startup banner used by the CLI.
func Banner() string {
	i := Get()
	return fmt.Sprintf("vyom %s\nCDN media observability and optimization\ncommit %s  built %s  %s  %s",
		i.Version, i.Commit, i.Date, i.GoVersion, i.Platform)
}
