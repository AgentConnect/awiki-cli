package listener

import (
	"fmt"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	"github.com/agentconnect/awiki-cli/internal/update"
)

const (
	upgradeMethodAvailable = "system.upgrade_available"
	upgradeMethodRequired  = "system.upgrade_required"
)

type UpgradeEvent struct {
	Method     string
	Decision   update.Decision
	ValidApply bool
}

func ParseUpgradeEvent(notification map[string]any) (UpgradeEvent, bool) {
	method := stringValue(notification["method"])
	if method != upgradeMethodAvailable && method != upgradeMethodRequired {
		return UpgradeEvent{}, false
	}
	params, _ := notification["params"].(map[string]any)
	artifact, _ := params["artifact"].(map[string]any)
	skillBundle, _ := params["skill_bundle"].(map[string]any)
	decision := update.Decision{
		CurrentVersion:      strings.TrimSpace(buildinfo.Version),
		LatestVersion:       stringValue(params["latest_version"]),
		MinSupportedVersion: stringValue(params["min_supported_version"]),
		Channel:             stringValue(params["channel"]),
		PublishedAt:         stringValue(params["published_at"]),
		Source:              "ws_push",
		ArtifactAvailable:   stringValue(artifact["url"]) != "",
		ArtifactURL:         stringValue(artifact["url"]),
		ArtifactSHA256:      stringValue(artifact["sha256"]),
		SkillBundleVersion:  stringValue(skillBundle["bundle_version"]),
		SkillBundleSHA256:   stringValue(skillBundle["bundle_sha256"]),
		RootSkillSHA256:     stringValue(skillBundle["root_skill_sha256"]),
		Blocked:             method == upgradeMethodRequired,
	}
	if decision.CurrentVersion != "" && decision.LatestVersion != "" {
		if cmp, ok := compareVersions(decision.LatestVersion, decision.CurrentVersion); ok && cmp > 0 {
			decision.HasNewerVersion = true
		}
	}
	return UpgradeEvent{
		Method:   method,
		Decision: decision,
		ValidApply: strings.TrimSpace(decision.LatestVersion) != "" &&
			strings.TrimSpace(decision.ArtifactURL) != "" &&
			strings.TrimSpace(decision.ArtifactSHA256) != "",
	}, true
}

func normalizeAutoUpgradeMode(enabled bool, mode string) string {
	if !enabled {
		return "notify"
	}
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "", "apply":
		return "apply"
	case "predownload":
		return "predownload"
	case "notify":
		return "notify"
	default:
		return "apply"
	}
}

func compareVersions(a, b string) (int, bool) {
	av, okA := parseSemVersion(a)
	bv, okB := parseSemVersion(b)
	if !okA || !okB {
		return 0, false
	}
	if av.Major != bv.Major {
		if av.Major > bv.Major {
			return 1, true
		}
		return -1, true
	}
	if av.Minor != bv.Minor {
		if av.Minor > bv.Minor {
			return 1, true
		}
		return -1, true
	}
	if av.Patch != bv.Patch {
		if av.Patch > bv.Patch {
			return 1, true
		}
		return -1, true
	}
	if av.Pre == bv.Pre {
		return 0, true
	}
	if av.Pre == "" {
		return 1, true
	}
	if bv.Pre == "" {
		return -1, true
	}
	if av.Pre > bv.Pre {
		return 1, true
	}
	if av.Pre < bv.Pre {
		return -1, true
	}
	return 0, true
}

type semVersion struct {
	Major int
	Minor int
	Patch int
	Pre   string
}

func parseSemVersion(raw string) (semVersion, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return semVersion{}, false
	}
	if raw[0] == 'v' || raw[0] == 'V' {
		raw = raw[1:]
	}
	var pre string
	if idx := strings.IndexByte(raw, '-'); idx >= 0 {
		pre = raw[idx+1:]
		raw = raw[:idx]
	}
	parts := strings.Split(raw, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return semVersion{}, false
	}
	parsePart := func(s string) (int, bool) {
		if s == "" {
			return 0, true
		}
		var value int
		_, err := fmt.Sscanf(s, "%d", &value)
		if err != nil || value < 0 {
			return 0, false
		}
		return value, true
	}
	major, ok := parsePart(parts[0])
	if !ok {
		return semVersion{}, false
	}
	minor := 0
	patch := 0
	if len(parts) >= 2 {
		if minor, ok = parsePart(parts[1]); !ok {
			return semVersion{}, false
		}
	}
	if len(parts) == 3 {
		if patch, ok = parsePart(parts[2]); !ok {
			return semVersion{}, false
		}
	}
	return semVersion{Major: major, Minor: minor, Patch: patch, Pre: pre}, true
}
