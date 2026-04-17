package update

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	"github.com/agentconnect/awiki-cli/internal/identity"
)

const (
	defaultMetadataCacheTTLSeconds = 3600
	updateCheckEndpoint            = "/api/cli/updates/check"
	defaultSkillFormatVersion      = "v1"
)

var newHTTPClient = func() *http.Client {
	return &http.Client{Timeout: 3 * time.Second}
}

type Artifact struct {
	Platform  string `json:"platform,omitempty"`
	URL       string `json:"url,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Signature string `json:"signature,omitempty"`
}

type SkillBundle struct {
	BundleVersion   string `json:"bundle_version,omitempty"`
	BundleSHA256    string `json:"bundle_sha256,omitempty"`
	RootSkillSHA256 string `json:"root_skill_sha256,omitempty"`
}

// Metadata captures the remote version strategy state that we cache locally.
type Metadata struct {
	LatestVersion       string      `json:"latest_version"`
	MinSupportedVersion string      `json:"min_supported_version"`
	Channel             string      `json:"channel,omitempty"`
	PublishedAt         string      `json:"published_at,omitempty"`
	Artifact            Artifact    `json:"artifact,omitempty"`
	SkillBundle         SkillBundle `json:"skill_bundle,omitempty"`
	RetrievedAt         time.Time   `json:"retrieved_at"`
	Source              string      `json:"source"` // "network" | "cache" | "cache_stale"
}

// Decision describes how the CLI should behave given the current / remote versions.
type Decision struct {
	CurrentVersion      string `json:"current_version"`
	LatestVersion       string `json:"latest_version"`
	MinSupportedVersion string `json:"min_supported_version"`
	Channel             string `json:"channel,omitempty"`
	PublishedAt         string `json:"published_at,omitempty"`
	Source              string `json:"source,omitempty"`

	ArtifactAvailable  bool   `json:"artifact_available"`
	ArtifactURL        string `json:"artifact_url,omitempty"`
	ArtifactSHA256     string `json:"artifact_sha256,omitempty"`
	SkillBundleVersion string `json:"skill_bundle_version,omitempty"`
	SkillBundleSHA256  string `json:"skill_bundle_sha256,omitempty"`
	RootSkillSHA256    string `json:"root_skill_sha256,omitempty"`

	StrictDisabled  bool `json:"strict_disabled"`
	DevBuild        bool `json:"dev_build"`
	HasNewerVersion bool `json:"has_newer_version"`
	Blocked         bool `json:"blocked"`
}

type requestIdentity struct {
	CurrentDID string
	JWTToken   string
}

type updateCheckRequest struct {
	SchemaVersion      int      `json:"schema_version"`
	CurrentVersion     string   `json:"current_version"`
	CurrentDID         string   `json:"current_did,omitempty"`
	Channel            string   `json:"channel,omitempty"`
	GOOS               string   `json:"goos"`
	GOARCH             string   `json:"goarch"`
	HostAgent          string   `json:"host_agent,omitempty"`
	HostVersion        string   `json:"host_version,omitempty"`
	HostCapabilities   []string `json:"host_capabilities,omitempty"`
	SkillFormatVersion string   `json:"skill_format_version,omitempty"`
}

type updateCheckResponse struct {
	SchemaVersion       int         `json:"schema_version"`
	LatestVersion       string      `json:"latest_version"`
	MinSupportedVersion string      `json:"min_supported_version"`
	Channel             string      `json:"channel,omitempty"`
	PublishedAt         string      `json:"published_at,omitempty"`
	Artifact            Artifact    `json:"artifact,omitempty"`
	SkillBundle         SkillBundle `json:"skill_bundle,omitempty"`
}

// Check resolves the effective version policy (including config + env overrides),
// loads remote metadata with caching, and returns the decision for the current
// awiki-cli binary.
func Check(resolved *appconfig.Resolved) (Decision, error) {
	current := strings.TrimSpace(buildinfo.Version)
	if current == "" {
		current = "dev"
	}
	devBuild := isDevVersion(current)

	strictDisabled := resolved != nil && resolved.UpdateDisableStrictVersion
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

	meta, err := loadMetadata(resolved, ttlSeconds)
	if err != nil {
		return decision, err
	}

	decision.LatestVersion = meta.LatestVersion
	decision.MinSupportedVersion = meta.MinSupportedVersion
	decision.Channel = meta.Channel
	decision.PublishedAt = meta.PublishedAt
	decision.Source = meta.Source
	decision.ArtifactURL = strings.TrimSpace(meta.Artifact.URL)
	decision.ArtifactSHA256 = strings.TrimSpace(meta.Artifact.SHA256)
	decision.ArtifactAvailable = decision.ArtifactURL != ""
	decision.SkillBundleVersion = strings.TrimSpace(meta.SkillBundle.BundleVersion)
	decision.SkillBundleSHA256 = strings.TrimSpace(meta.SkillBundle.BundleSHA256)
	decision.RootSkillSHA256 = strings.TrimSpace(meta.SkillBundle.RootSkillSHA256)

	if devBuild {
		if newer, ok := compareVersions(meta.LatestVersion, current); ok && newer > 0 {
			decision.HasNewerVersion = true
		}
		return decision, nil
	}

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

func loadMetadata(resolved *appconfig.Resolved, ttlSeconds int) (Metadata, error) {
	var zero Metadata

	var cached Metadata
	cacheFile, cacheErr := cachePath(resolved)
	if cacheErr == nil {
		if m, ok, err := readCache(cacheFile, ttlSeconds); err == nil {
			if ok {
				return m, nil
			}
			cached = m
		}
	}

	network, err := fetchFromService(resolved)
	if err != nil {
		if cached.LatestVersion != "" {
			cached.Source = "cache_stale"
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
		return meta, false, nil
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

func fetchFromService(resolved *appconfig.Resolved) (Metadata, error) {
	if resolved == nil {
		return Metadata{}, errors.New("config is nil")
	}
	requestURL := appconfig.JoinBaseURL(resolved.ServiceBaseURL, updateCheckEndpoint)
	if strings.TrimSpace(requestURL) == "" {
		return Metadata{}, errors.New("service base url is empty")
	}
	payload := buildRequestPayload(resolved)
	identityContext, _ := loadRequestIdentity(resolved)
	if identityContext != nil && strings.TrimSpace(identityContext.CurrentDID) != "" {
		payload.CurrentDID = strings.TrimSpace(identityContext.CurrentDID)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Metadata{}, err
	}

	headers := map[string]string{"Content-Type": "application/json"}
	if identityContext != nil && strings.TrimSpace(identityContext.JWTToken) != "" {
		headers["Authorization"] = "Bearer " + strings.TrimSpace(identityContext.JWTToken)
	}

	response, err := doJSONRequest(newHTTPClient(), requestURL, body, headers)
	if err != nil {
		return Metadata{}, err
	}
	latest := strings.TrimSpace(response.LatestVersion)
	if latest == "" {
		return Metadata{}, errors.New("update metadata missing latest_version")
	}
	return Metadata{
		LatestVersion:       latest,
		MinSupportedVersion: strings.TrimSpace(response.MinSupportedVersion),
		Channel:             strings.TrimSpace(response.Channel),
		PublishedAt:         strings.TrimSpace(response.PublishedAt),
		Artifact:            response.Artifact,
		SkillBundle:         response.SkillBundle,
		RetrievedAt:         time.Now().UTC(),
		Source:              "network",
	}, nil
}

func buildRequestPayload(resolved *appconfig.Resolved) updateCheckRequest {
	info := buildinfo.Current()
	channel := defaultChannel(resolved)
	skillFormatVersion := strings.TrimSpace(info.SkillFormatVersion)
	if skillFormatVersion == "" {
		skillFormatVersion = defaultSkillFormatVersion
	}
	return updateCheckRequest{
		SchemaVersion:      1,
		CurrentVersion:     strings.TrimSpace(info.Version),
		Channel:            channel,
		GOOS:               strings.TrimSpace(info.GOOS),
		GOARCH:             strings.TrimSpace(info.GOARCH),
		HostAgent:          strings.TrimSpace(info.HostAgent),
		HostVersion:        strings.TrimSpace(info.HostVersion),
		HostCapabilities:   append([]string(nil), info.HostCapabilities...),
		SkillFormatVersion: skillFormatVersion,
	}
}

func defaultChannel(resolved *appconfig.Resolved) string {
	if resolved == nil {
		return "stable"
	}
	if channel := strings.TrimSpace(resolved.UpdateChannel); channel != "" {
		return channel
	}
	return "stable"
}

func loadRequestIdentity(resolved *appconfig.Resolved) (*requestIdentity, error) {
	if resolved == nil {
		return nil, errors.New("config is nil")
	}
	manager := identity.NewManager(resolved.Paths)
	identityName := strings.TrimSpace(resolved.ActiveIdentity)
	if identityName == "" {
		current, err := manager.Current()
		if err != nil || current == nil {
			return nil, nil
		}
		identityName = current.IdentityName
	}
	record, err := manager.Load(identityName)
	if err != nil || record == nil {
		return nil, nil
	}
	return &requestIdentity{
		CurrentDID: strings.TrimSpace(record.DID),
		JWTToken:   strings.TrimSpace(record.JWTToken),
	}, nil
}

func doJSONRequest(client *http.Client, requestURL string, body []byte, headers map[string]string) (updateCheckResponse, error) {
	if client == nil {
		client = newHTTPClient()
	}
	request, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return updateCheckResponse{}, err
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return updateCheckResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		var errorBody struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(response.Body).Decode(&errorBody)
		if strings.TrimSpace(errorBody.Message) != "" {
			return updateCheckResponse{}, fmt.Errorf("update service responded with status %d: %s", response.StatusCode, strings.TrimSpace(errorBody.Message))
		}
		return updateCheckResponse{}, fmt.Errorf("update service responded with status %d", response.StatusCode)
	}
	var decoded updateCheckResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return updateCheckResponse{}, err
	}
	return decoded, nil
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
	return semVersion{Major: major, Minor: minor, Patch: patch, Pre: pre}, true
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
