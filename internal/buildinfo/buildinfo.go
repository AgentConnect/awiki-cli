package buildinfo

import (
	"runtime"
	"strings"
)

var (
	Version            = "dev"
	Commit             = "unknown"
	BuildDate          = "unknown"
	CGOEnabled         = "unknown"
	HostAgent          = "unknown"
	HostVersion        = ""
	HostCapabilities   = ""
	SkillFormatVersion = "v1"
)

type Info struct {
	Version            string   `json:"version"`
	Commit             string   `json:"commit"`
	BuildDate          string   `json:"build_date"`
	GoVersion          string   `json:"go_version"`
	GOOS               string   `json:"goos"`
	GOARCH             string   `json:"goarch"`
	Compiler           string   `json:"compiler"`
	CGOEnabled         string   `json:"cgo_enabled"`
	HostAgent          string   `json:"host_agent"`
	HostVersion        string   `json:"host_version,omitempty"`
	HostCapabilities   []string `json:"host_capabilities,omitempty"`
	SkillFormatVersion string   `json:"skill_format_version"`
}

func Current() Info {
	return Info{
		Version:            Version,
		Commit:             Commit,
		BuildDate:          BuildDate,
		GoVersion:          runtime.Version(),
		GOOS:               runtime.GOOS,
		GOARCH:             runtime.GOARCH,
		Compiler:           runtime.Compiler,
		CGOEnabled:         CGOEnabled,
		HostAgent:          HostAgent,
		HostVersion:        HostVersion,
		HostCapabilities:   splitCapabilities(HostCapabilities),
		SkillFormatVersion: SkillFormatVersion,
	}
}

func splitCapabilities(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		parts = append(parts, trimmed)
	}
	if len(parts) == 0 {
		return nil
	}
	return parts
}
