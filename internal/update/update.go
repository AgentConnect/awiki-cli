package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/traceutil"
)

const (
	defaultMetadataCacheTTLSeconds = 43200 // 12h
	npmLatestURL                   = "https://registry.npmjs.org/@awiki%2Fcli/latest"
	npmMirrorLatestURL             = "https://registry.npmmirror.com/@awiki/cli/latest"
)

var npmLatestURLs = []string{
	npmLatestURL,
	npmMirrorLatestURL,
}

// Metadata captures the remote version strategy state that we cache locally.
type Metadata struct {
	LatestVersion       string    `json:"latest_version"`
	MinSupportedVersion string    `json:"min_supported_version"`
	RetrievedAt         time.Time `json:"retrieved_at"`
	Source              string    `json:"source"` // "network" | "cache" | "cache_stale"
}

// Decision describes how the CLI should behave given the current / remote versions.
type Decision struct {
	CurrentVersion      string `json:"current_version"`
	LatestVersion       string `json:"latest_version"`
	MinSupportedVersion string `json:"min_supported_version"`
	MetadataSource      string `json:"metadata_source,omitempty"`

	StrictDisabled  bool `json:"strict_disabled"`
	DevBuild        bool `json:"dev_build"`
	HasNewerVersion bool `json:"has_newer_version"`
	Blocked         bool `json:"blocked"`
}

type checkOptions struct {
	preferFresh bool
}

// Check resolves the effective version policy (including config + env overrides),
// loads remote metadata with caching, and returns the decision for the current
// awiki-cli binary.
//
// This function is intentionally tolerant:
// - Network / cache errors never crash the CLI; callers can choose how hard to fail.
// - When metadata is missing or unparsable, the Decision falls back to "no block".
func Check(resolved *appconfig.Resolved) (Decision, error) {
	return check(context.Background(), resolved, checkOptions{})
}

// CheckFresh behaves like Check, but for explicit user-initiated upgrade flows
// it prefers fresh network metadata and falls back to cached metadata only when
// both registries are unavailable.
func CheckFresh(ctx context.Context, resolved *appconfig.Resolved) (Decision, error) {
	return check(ctx, resolved, checkOptions{preferFresh: true})
}

func check(ctx context.Context, resolved *appconfig.Resolved, opts checkOptions) (Decision, error) {
	current := strings.TrimSpace(buildinfo.Version)
	if current == "" {
		current = "dev"
	}
	devBuild := isDevVersion(current)

	strictDisabled := resolved != nil && resolved.UpdateDisableStrictVersion
	// AWIKI_CLI_DISABLE_STRICT_VERSION is a last-resort escape hatch for debugging.
	if raw := strings.TrimSpace(os.Getenv("AWIKI_CLI_DISABLE_STRICT_VERSION")); raw != "" {
		strictDisabled = parseBool(raw)
	}

	ttlSeconds := defaultMetadataCacheTTLSeconds
	if resolved != nil && resolved.UpdateMetadataCacheTTLSeconds > 0 {
		ttlSeconds = resolved.UpdateMetadataCacheTTLSeconds
	}
	if raw := strings.TrimSpace(os.Getenv("AWIKI_CLI_UPDATE_CACHE_TTL")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			ttlSeconds = parsed
		}
	}

	decision := Decision{
		CurrentVersion: current,
		StrictDisabled: strictDisabled,
		DevBuild:       devBuild,
	}

	finish := traceutil.PhaseContextWithDetail(ctx, "update_check", updateCheckPhaseDetail(opts.preferFresh))
	defer finish()

	meta, err := loadMetadata(ctx, resolved, ttlSeconds, opts.preferFresh)
	if err != nil {
		// Propagate the error so callers can log or surface it, but keep the
		// decision usable (no block by default).
		return decision, err
	}

	decision.LatestVersion = meta.LatestVersion
	decision.MinSupportedVersion = meta.MinSupportedVersion
	decision.MetadataSource = meta.Source

	// Dev builds should never be blocked, but they can still see "newer available".
	if devBuild {
		if newer, ok := compareVersions(meta.LatestVersion, current); ok && newer > 0 {
			decision.HasNewerVersion = true
		}
		return decision, nil
	}

	// Compute "has newer" and "blocked" flags based on semantic version ordering.
	if newer, ok := compareVersions(meta.LatestVersion, current); ok && newer > 0 {
		decision.HasNewerVersion = true
	}

	if !strictDisabled {
		if cmp, ok := compareVersions(current, meta.MinSupportedVersion); ok && cmp < 0 {
			decision.Blocked = true
		}
	}

	return decision, nil
}

func isDevVersion(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" || v == "dev" {
		return true
	}
	if strings.Contains(v, "-dev") {
		return true
	}
	if strings.HasPrefix(v, "0.0.0-") {
		return true
	}
	return false
}

func parseBool(raw string) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func cachePath(resolved *appconfig.Resolved) (string, error) {
	if resolved == nil {
		return "", errors.New("config is nil")
	}
	cacheDir := strings.TrimSpace(resolved.Paths.CacheDir)
	if cacheDir == "" {
		return "", errors.New("cache dir is empty")
	}
	return filepath.Join(cacheDir, "update", "metadata.json"), nil
}

func loadMetadata(ctx context.Context, resolved *appconfig.Resolved, ttlSeconds int, preferFresh bool) (Metadata, error) {
	var zero Metadata

	var cached Metadata
	cacheFile, cacheErr := cachePath(resolved)
	if cacheErr == nil {
		if m, ok, err := readCache(cacheFile, ttlSeconds); err == nil {
			if ok {
				if !preferFresh {
					return m, nil
				}
				cached = m
			} else {
				// ok == false -> expired or empty cache; fall through to network,
				// but remember the last good snapshot in case the network is down.
				cached = m
			}
		} else {
			// Any cache read error is treated as soft; we still try network.
			cached = Metadata{}
		}
	}

	network, err := fetchFromRegistry(ctx)
	if err != nil {
		// If we had a usable cached value (even if TTL expired), fall back to it
		// rather than failing hard.
		if cached.LatestVersion != "" {
			cached.Source = "cache_stale"
			traceutil.MarkFallback(ctx, "update_check:cache_stale", err)
			return cached, nil
		}
		return zero, err
	}

	if cacheErr == nil {
		_ = writeCache(cacheFile, network)
	}
	return network, nil
}

func readCache(path string, ttlSeconds int) (Metadata, bool, error) {
	var meta Metadata
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return meta, false, nil
		}
		return meta, false, err
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return Metadata{}, false, err
	}
	if ttlSeconds <= 0 || meta.RetrievedAt.IsZero() {
		return meta, true, nil
	}
	if time.Since(meta.RetrievedAt) > time.Duration(ttlSeconds)*time.Second {
		return Metadata{}, false, nil
	}
	return meta, true, nil
}

func writeCache(path string, meta Metadata) error {
	meta.RetrievedAt = time.Now().UTC()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func fetchFromRegistry(ctx context.Context) (Metadata, error) {
	client := &http.Client{
		Timeout: 3 * time.Second,
	}
	return fetchFromRegistryURLs(ctx, client, npmLatestURLs)
}

func fetchFromRegistryURLs(ctx context.Context, client *http.Client, urls []string) (Metadata, error) {
	if len(urls) == 0 {
		return Metadata{}, errors.New("no npm registry URLs configured")
	}
	if client == nil {
		client = &http.Client{
			Timeout: 3 * time.Second,
		}
	}

	var errs []string
	for i, url := range urls {
		meta, err := fetchFromRegistryURL(ctx, client, url)
		if err == nil {
			return meta, nil
		}
		if i < len(urls)-1 {
			traceutil.MarkFallback(ctx, "update_registry_fetch:"+registryHost(url), err)
		}
		errs = append(errs, fmt.Sprintf("%s: %v", url, err))
	}
	return Metadata{}, fmt.Errorf("failed to fetch awiki-cli metadata from npm registries: %s", strings.Join(errs, "; "))
}

func fetchFromRegistryURL(ctx context.Context, client *http.Client, url string) (Metadata, error) {
	finish := traceutil.PhaseContextWithDetail(ctx, "update_registry_fetch", registryHost(url))
	defer finish()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return Metadata{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return Metadata{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Metadata{}, fmt.Errorf("registry responded with status %d", resp.StatusCode)
	}

	var body struct {
		Version  string `json:"version"`
		AwikiCli struct {
			MinSupportedVersion string `json:"minSupportedVersion"`
		} `json:"awikiCli"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Metadata{}, err
	}

	latest := strings.TrimSpace(body.Version)
	if latest == "" {
		return Metadata{}, errors.New("npm metadata missing version")
	}

	minSupported := strings.TrimSpace(body.AwikiCli.MinSupportedVersion)
	if minSupported == "" {
		// If the manifest does not provide an explicit floor, default to "no floor".
		// The decision logic will treat an empty min as "no block".
		minSupported = ""
	}

	return Metadata{
		LatestVersion:       latest,
		MinSupportedVersion: minSupported,
		RetrievedAt:         time.Now().UTC(),
		Source:              "network",
	}, nil
}

func updateCheckPhaseDetail(preferFresh bool) string {
	if preferFresh {
		return "fresh"
	}
	return "cached"
}

func registryHost(rawURL string) string {
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	host := strings.TrimSpace(parsed.Host)
	if host == "" {
		return rawURL
	}
	return host
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
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0, false
		}
		if n < 0 {
			return 0, false
		}
		return n, true
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
	return semVersion{
		Major: major,
		Minor: minor,
		Patch: patch,
		Pre:   pre,
	}, true
}

// compareVersions returns:
//
//	>0 if a > b
//	 0 if a == b
//	<0 if a < b
//
// The second return value is false if either side could not be parsed.
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
	// Pre-release comparison: empty pre means stable and is considered newer than pre-release.
	if av.Pre == bv.Pre {
		return 0, true
	}
	if av.Pre == "" {
		return 1, true
	}
	if bv.Pre == "" {
		return -1, true
	}
	return comparePrerelease(av.Pre, bv.Pre), true
}

func comparePrerelease(a, b string) int {
	left := strings.Split(a, ".")
	right := strings.Split(b, ".")
	limit := len(left)
	if len(right) > limit {
		limit = len(right)
	}
	for i := 0; i < limit; i++ {
		if i >= len(left) {
			return -1
		}
		if i >= len(right) {
			return 1
		}
		if left[i] == right[i] {
			continue
		}

		leftNumeric := isNumericIdentifier(left[i])
		rightNumeric := isNumericIdentifier(right[i])
		switch {
		case leftNumeric && rightNumeric:
			return compareNumericIdentifiers(left[i], right[i])
		case leftNumeric:
			return -1
		case rightNumeric:
			return 1
		case left[i] > right[i]:
			return 1
		default:
			return -1
		}
	}
	return 0
}

func isNumericIdentifier(raw string) bool {
	if raw == "" {
		return false
	}
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func compareNumericIdentifiers(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if a == "" {
		a = "0"
	}
	if b == "" {
		b = "0"
	}
	if len(a) != len(b) {
		if len(a) > len(b) {
			return 1
		}
		return -1
	}
	if a > b {
		return 1
	}
	if a < b {
		return -1
	}
	return 0
}
