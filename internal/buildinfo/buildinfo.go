package buildinfo

import "runtime"

var (
	Version    = "dev"
	Commit     = "unknown"
	BuildDate  = "unknown"
	CGOEnabled = "unknown"
)

type Info struct {
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	BuildDate  string `json:"build_date"`
	GoVersion  string `json:"go_version"`
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
	Compiler   string `json:"compiler"`
	CGOEnabled string `json:"cgo_enabled"`
}

func Current() Info {
	return Info{
		Version:    Version,
		Commit:     Commit,
		BuildDate:  BuildDate,
		GoVersion:  runtime.Version(),
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		Compiler:   runtime.Compiler,
		CGOEnabled: CGOEnabled,
	}
}
