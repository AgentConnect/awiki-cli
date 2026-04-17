package upgrader

const releaseStateSchemaVersion = 1

type PendingUpdate struct {
	Version        string `json:"version,omitempty"`
	ArtifactURL    string `json:"artifact_url,omitempty"`
	ArtifactSHA256 string `json:"artifact_sha256,omitempty"`
	RecordedAt     string `json:"recorded_at,omitempty"`
}

type FailedUpdate struct {
	Version  string `json:"version,omitempty"`
	Stage    string `json:"stage,omitempty"`
	Message  string `json:"message,omitempty"`
	FailedAt string `json:"failed_at,omitempty"`
}

type ReleaseState struct {
	SchemaVersion         int            `json:"schema_version"`
	CurrentVersion        string         `json:"current_version,omitempty"`
	CurrentArtifactSHA256 string         `json:"current_artifact_sha256,omitempty"`
	Channel               string         `json:"channel,omitempty"`
	LastCheckedAt         string         `json:"last_checked_at,omitempty"`
	LastCheckSource       string         `json:"last_check_source,omitempty"`
	LastWSUpgradeEventAt  string         `json:"last_ws_upgrade_event_at,omitempty"`
	PendingUpdate         *PendingUpdate `json:"pending_update,omitempty"`
	LastAppliedVersion    string         `json:"last_applied_version,omitempty"`
	LastFailedUpdate      *FailedUpdate  `json:"last_failed_update,omitempty"`
	UpdatedAt             string         `json:"updated_at,omitempty"`
}

type ApplyResult struct {
	InstalledVersion    string        `json:"installed_version,omitempty"`
	InstalledBinaryPath string        `json:"installed_binary_path,omitempty"`
	CurrentBinaryPath   string        `json:"current_binary_path,omitempty"`
	AlreadyCurrent      bool          `json:"already_current"`
	ReleaseState        *ReleaseState `json:"release_state,omitempty"`
}
