// Package version exposes build metadata injected with -ldflags:
//
//	-X github.com/aurimasniekis/go-pwr-tracker/internal/version.Version=v1.2.3
//	-X github.com/aurimasniekis/go-pwr-tracker/internal/version.Commit=abc123
//	-X github.com/aurimasniekis/go-pwr-tracker/internal/version.Date=2026-01-01T00:00:00Z
package version

import "runtime/debug"

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// Get returns build metadata, falling back to VCS info recorded by the Go
// toolchain when ldflags were not set.
func Get() Info {
	i := Info{Version: Version, Commit: Commit, Date: Date}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if i.Commit == "" {
					i.Commit = s.Value
				}
			case "vcs.time":
				if i.Date == "" {
					i.Date = s.Value
				}
			}
		}
	}
	return i
}

// UserAgent is the default identifiable User-Agent for upstream requests.
func UserAgent() string {
	return "pwr-tracker/" + Version + " (+https://github.com/aurimasniekis/go-pwr-tracker)"
}
