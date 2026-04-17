package skillbundle

import "io/fs"

const skillStateSchemaVersion = 1

type Bundle struct {
	Manifest Manifest
	FS       fs.FS
}

type Manifest struct {
	SchemaVersion int            `json:"schema_version"`
	CLIName       string         `json:"cli_name"`
	CLIVersion    string         `json:"cli_version"`
	BundleVersion string         `json:"bundle_version"`
	GeneratedAt   string         `json:"generated_at,omitempty"`
	BundleSHA256  string         `json:"bundle_sha256"`
	RootSkill     RootSkillMeta  `json:"root_skill"`
	Children      []ChildDocMeta `json:"children"`
	Metadata      ManifestMeta   `json:"metadata"`
}

type RootSkillMeta struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	SHA256      string `json:"sha256"`
	Description string `json:"description,omitempty"`
}

type ChildDocMeta struct {
	Path    string   `json:"path"`
	Kind    string   `json:"kind"`
	Title   string   `json:"title"`
	Summary string   `json:"summary,omitempty"`
	SHA256  string   `json:"sha256"`
	Tags    []string `json:"tags,omitempty"`
}

type ManifestMeta struct {
	DefaultSchemaLookup string `json:"default_schema_lookup"`
	SupportsExport      bool   `json:"supports_export"`
}

type ManagedFile struct {
	SchemaVersion   int      `json:"schema_version"`
	Type            string   `json:"type"`
	SkillName       string   `json:"skill_name"`
	CLIVersion      string   `json:"cli_version"`
	BundleVersion   string   `json:"bundle_version"`
	RootSkillSHA256 string   `json:"root_skill_sha256"`
	InstalledAt     string   `json:"installed_at"`
	ManagedPaths    []string `json:"managed_paths"`
	Adopted         bool     `json:"adopted"`
}

type SkillState struct {
	SchemaVersion        int                `json:"schema_version"`
	CurrentCLIVersion    string             `json:"current_cli_version"`
	CurrentBundleVersion string             `json:"current_bundle_version"`
	CurrentBundleSHA256  string             `json:"current_bundle_sha256"`
	RootSkillSync        RootSkillSyncState `json:"root_skill_sync"`
	CachePolicy          CachePolicy        `json:"cache_policy"`
}

type RootSkillSyncState struct {
	Dir             string `json:"dir,omitempty"`
	RootSkillSHA256 string `json:"root_skill_sha256,omitempty"`
	LastSyncedAt    string `json:"last_synced_at,omitempty"`
	LastSyncStatus  string `json:"last_sync_status,omitempty"`
	LastError       string `json:"last_error,omitempty"`
}

type CachePolicy struct {
	KeepVersions []string `json:"keep_versions,omitempty"`
	TTLDays      int      `json:"ttl_days,omitempty"`
}

type GetResult struct {
	Path          string `json:"path"`
	Kind          string `json:"kind"`
	CLIVersion    string `json:"cli_version"`
	BundleVersion string `json:"bundle_version"`
	SHA256        string `json:"sha256"`
	Content       string `json:"content"`
	CacheMode     string `json:"cache_mode"`
	CacheHit      bool   `json:"cache_hit"`
	OutputFile    string `json:"output_file,omitempty"`
}

type SyncResult struct {
	Dir             string `json:"dir"`
	SkillDir        string `json:"skill_dir"`
	SkillPath       string `json:"skill_path"`
	ManagedPath     string `json:"managed_path"`
	BackupPath      string `json:"backup_path,omitempty"`
	WroteFiles      bool   `json:"wrote_files"`
	Adopted         bool   `json:"adopted"`
	Conflict        bool   `json:"conflict"`
	ConflictReason  string `json:"conflict_reason,omitempty"`
	RootSkillSHA256 string `json:"root_skill_sha256,omitempty"`
}

type ExportResult struct {
	Dir          string   `json:"dir"`
	ExportedRoot bool     `json:"exported_root"`
	ExportedDocs []string `json:"exported_docs"`
	ManifestPath string   `json:"manifest_path"`
}

type IndexResult struct {
	Name          string         `json:"name"`
	CLIVersion    string         `json:"cli_version"`
	BundleVersion string         `json:"bundle_version"`
	RootSkill     RootSkillMeta  `json:"root_skill"`
	Children      []ChildDocMeta `json:"children"`
}
