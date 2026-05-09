package identity

import "errors"

const (
	IndexSchemaVersion           = 3
	IndexFileName                = "index.json"
	LegacyBackupDirName          = ".legacy-backup"
	IdentityFileName             = "identity.json"
	AuthFileName                 = "auth.json"
	DIDDocumentFileName          = "did_document.json"
	Key1PrivateFileName          = "key-1-private.pem"
	Key1PublicFileName           = "key-1-public.pem"
	E2EESigningPrivateFileName   = "e2ee-signing-private.pem"
	E2EEAgreementPrivateFileName = "e2ee-agreement-private.pem"
	E2EEStateFileName            = "e2ee-state.json"
	LegacyE2EEPrefix             = "e2ee_"
	LegacyLayoutHint             = "Legacy credential layout detected. Import it before relying on the v2 identity store."
	DefaultEmailVerificationSecs = 300
	DefaultEmailPollIntervalSecs = 5
)

var (
	ErrIdentityNotFound         = errors.New("identity not found")
	ErrIdentityConflict         = errors.New("identity conflict")
	ErrNoDefaultIdentity        = errors.New("no default identity")
	ErrLegacyNotFound           = errors.New("legacy identity not found")
	ErrInvalidInput             = errors.New("invalid input")
	ErrAuthRequired             = errors.New("authentication required")
	ErrUserRegistrationRequired = errors.New("registered handle user is required")
)

type IndexEntry struct {
	CredentialName string `json:"credential_name"`
	DirName        string `json:"dir_name"`
	DID            string `json:"did"`
	UniqueID       string `json:"unique_id"`
	UserID         string `json:"user_id,omitempty"`
	Name           string `json:"name,omitempty"`
	Handle         string `json:"handle,omitempty"`
	FullHandle     string `json:"full_handle,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
	IsDefault      bool   `json:"is_default,omitempty"`
}

type IndexPayload struct {
	SchemaVersion         int                   `json:"schema_version"`
	DefaultCredentialName string                `json:"default_credential_name,omitempty"`
	Credentials           map[string]IndexEntry `json:"credentials"`
}

type Paths struct {
	RootDir                  string `json:"root_dir"`
	DirName                  string `json:"dir_name"`
	IdentityDir              string `json:"identity_dir"`
	IdentityPath             string `json:"identity_path"`
	AuthPath                 string `json:"auth_path"`
	DIDDocumentPath          string `json:"did_document_path"`
	Key1PrivatePath          string `json:"key1_private_path"`
	Key1PublicPath           string `json:"key1_public_path"`
	E2EESigningPrivatePath   string `json:"e2ee_signing_private_path"`
	E2EEAgreementPrivatePath string `json:"e2ee_agreement_private_path"`
	E2EEStatePath            string `json:"e2ee_state_path"`
}

type StoredIdentity struct {
	IdentityName            string         `json:"identity_name"`
	DirName                 string         `json:"dir_name"`
	DID                     string         `json:"did"`
	UniqueID                string         `json:"unique_id"`
	UserID                  string         `json:"user_id,omitempty"`
	DisplayName             string         `json:"display_name,omitempty"`
	Handle                  string         `json:"handle,omitempty"`
	FullHandle              string         `json:"full_handle,omitempty"`
	CreatedAt               string         `json:"created_at,omitempty"`
	JWTToken                string         `json:"jwt_token,omitempty"`
	DIDDocument             map[string]any `json:"did_document,omitempty"`
	Key1PrivatePEM          string         `json:"key1_private_pem,omitempty"`
	Key1PublicPEM           string         `json:"key1_public_pem,omitempty"`
	E2EESigningPrivatePEM   string         `json:"e2ee_signing_private_pem,omitempty"`
	E2EEAgreementPrivatePEM string         `json:"e2ee_agreement_private_pem,omitempty"`
	IsDefault               bool           `json:"is_default,omitempty"`
}

type IdentitySummary struct {
	IdentityName            string    `json:"identity_name"`
	DID                     string    `json:"did"`
	UniqueID                string    `json:"unique_id"`
	UserID                  string    `json:"user_id,omitempty"`
	DisplayName             string    `json:"display_name,omitempty"`
	Handle                  string    `json:"handle,omitempty"`
	FullHandle              string    `json:"full_handle,omitempty"`
	CreatedAt               string    `json:"created_at,omitempty"`
	DirName                 string    `json:"dir_name"`
	IsDefault               bool      `json:"is_default"`
	HasJWT                  bool      `json:"has_jwt"`
	HasDIDDocument          bool      `json:"has_did_document"`
	HasKey1Private          bool      `json:"has_key1_private"`
	HasKey1Public           bool      `json:"has_key1_public"`
	HasE2EESigningPrivate   bool      `json:"has_e2ee_signing_private"`
	HasE2EEAgreementPrivate bool      `json:"has_e2ee_agreement_private"`
	UserState               UserState `json:"user_state"`
}

type UserState struct {
	RegistrationState string   `json:"registration_state"`
	ReadyForMessaging bool     `json:"ready_for_messaging"`
	Missing           []string `json:"missing,omitempty"`
}

type LegacyFlatIdentity struct {
	CredentialName string `json:"credential_name"`
	Path           string `json:"path"`
	DID            string `json:"did"`
	UniqueID       string `json:"unique_id"`
	Handle         string `json:"handle,omitempty"`
}

type LegacyScan struct {
	RootDir           string                `json:"root_dir"`
	IndexedLayout     bool                  `json:"indexed_layout"`
	IndexedEntries    map[string]IndexEntry `json:"indexed_entries,omitempty"`
	LegacyCredentials []LegacyFlatIdentity  `json:"legacy_credentials,omitempty"`
	InvalidJSONFiles  []map[string]string   `json:"invalid_json_files,omitempty"`
	OrphanE2EEFiles   []map[string]string   `json:"orphan_e2ee_files,omitempty"`
	HasLegacy         bool                  `json:"has_legacy"`
	Hint              string                `json:"hint,omitempty"`
}

type SaveInput struct {
	IdentityName            string
	DID                     string
	UniqueID                string
	UserID                  string
	DisplayName             string
	Handle                  string
	FullHandle              string
	JWTToken                string
	DIDDocument             map[string]any
	Key1PrivatePEM          string
	Key1PublicPEM           string
	E2EESigningPrivatePEM   string
	E2EEAgreementPrivatePEM string
	ReplaceExisting         bool
}

type GenerateOptions struct {
	Hostname           string
	PathPrefix         []string
	ProofDomain        string
	ANPServiceEndpoint string
	ANPServiceDID      string
}

type GeneratedIdentity struct {
	DID                     string
	UniqueID                string
	DIDDocument             map[string]any
	Key1PrivatePEM          string
	Key1PublicPEM           string
	E2EESigningPrivatePEM   string
	E2EEAgreementPrivatePEM string
}

type CommandResult struct {
	Data     map[string]any
	Summary  string
	Warnings []string
}

type RegisterParams struct {
	IdentityName        string
	Handle              string
	Phone               string
	Email               string
	OTP                 string
	InviteCode          string
	Wait                bool
	VerificationTimeout int
	PollIntervalSeconds float64
}

type BindParams struct {
	Phone               string
	Email               string
	OTP                 string
	Wait                bool
	VerificationTimeout int
	PollIntervalSeconds float64
}

type RecoverParams struct {
	IdentityName string
	Handle       string
	Phone        string
	OTP          string
}

type ReplaceDIDParams struct {
	IdentityName string
	IsPublic     *bool
	IsAgent      *bool
	Role         *string
	EndpointURL  *string
}

type UpdateProfileParams struct {
	DisplayName  string
	Bio          string
	TagsCSV      string
	Markdown     string
	MarkdownFile string
}

type ImportResult struct {
	Imported []IdentitySummary `json:"imported,omitempty"`
	Skipped  []string          `json:"skipped,omitempty"`
}
