package store

import "errors"

const (
	SchemaVersion = 11
)

var (
	ErrUnsupportedLegacySchema = errors.New("unsupported legacy sqlite schema version")
	ErrLegacyDatabaseNotFound  = errors.New("legacy sqlite database not found")
	ErrUnsafeSQL               = errors.New("unsafe sql statement")
)

type OpenOptions struct {
	ReadOnly bool
}

type LegacyScan struct {
	Path          string   `json:"path"`
	Exists        bool     `json:"exists"`
	SchemaVersion int      `json:"schema_version"`
	Tables        []string `json:"tables,omitempty"`
}

type ImportReport struct {
	SourcePath          string         `json:"source_path"`
	SourceSchemaVersion int            `json:"source_schema_version"`
	ImportedRows        map[string]int `json:"imported_rows"`
	SkippedTables       []string       `json:"skipped_tables,omitempty"`
	Warnings            []string       `json:"warnings,omitempty"`
}

type MessageRecord struct {
	MsgID          string
	OwnerDID       string
	ThreadID       string
	Direction      int
	SenderDID      string
	ReceiverDID    string
	GroupID        string
	GroupDID       string
	ContentType    string
	Content        string
	Title          string
	ServerSeq      *int64
	SentAt         string
	IsE2EE         bool
	IsRead         bool
	SenderName     string
	Metadata       string
	CredentialName string
}

type ContactRecord struct {
	OwnerDID          string
	DID               string
	Name              string
	Handle            string
	NickName          string
	Bio               string
	ProfileMD         string
	Tags              string
	Relationship      string
	SourceType        string
	SourceName        string
	SourceGroupID     string
	ConnectedAt       string
	RecommendedReason string
	Followed          *bool
	Messaged          *bool
	Note              string
	FirstSeenAt       string
	LastSeenAt        string
	Metadata          string
}

type RelationshipEventRecord struct {
	EventID        string
	OwnerDID       string
	TargetDID      string
	TargetHandle   string
	EventType      string
	SourceType     string
	SourceName     string
	SourceGroupID  string
	Reason         string
	Score          *float64
	Status         string
	CreatedAt      string
	UpdatedAt      string
	Metadata       string
	CredentialName string
}

type GroupRecord struct {
	OwnerDID          string
	GroupID           string
	GroupDID          string
	Name              string
	GroupMode         string
	Slug              string
	Description       string
	Goal              string
	Rules             string
	MessagePrompt     string
	DocURL            string
	GroupOwnerDID     string
	GroupOwnerHandle  string
	MyRole            string
	MembershipStatus  string
	JoinEnabled       *bool
	JoinCode          string
	JoinCodeExpiresAt string
	MemberCount       *int64
	LastSyncedSeq     *int64
	LastReadSeq       *int64
	LastMessageAt     string
	RemoteCreatedAt   string
	RemoteUpdatedAt   string
	Metadata          string
	CredentialName    string
}

type GroupMemberRecord struct {
	OwnerDID         string
	GroupID          string
	UserID           string
	MemberDID        string
	MemberHandle     string
	ProfileURL       string
	Role             string
	Status           string
	JoinedAt         string
	SentMessageCount *int64
	Metadata         string
	CredentialName   string
}

type E2EEOutboxRecord struct {
	OutboxID        string
	OwnerDID        string
	PeerDID         string
	SessionID       string
	OriginalType    string
	Plaintext       string
	LocalStatus     string
	AttemptCount    int
	SentMsgID       string
	SentServerSeq   *int64
	LastErrorCode   string
	RetryHint       string
	FailedMsgID     string
	FailedServerSeq *int64
	Metadata        string
	LastAttemptAt   string
	CreatedAt       string
	UpdatedAt       string
	CredentialName  string
}

type E2EESessionRecord struct {
	OwnerDID       string
	PeerDID        string
	SessionID      string
	IsInitiator    bool
	SendChainKey   string
	RecvChainKey   string
	SendSeq        int
	RecvSeq        int
	ExpiresAt      *float64
	CreatedAt      string
	ActiveAt       string
	PeerConfirmed  bool
	UpdatedAt      string
	CredentialName string
}
