package listener

type SessionStatus struct {
	IdentityName string `json:"identity_name"`
	DID          string `json:"did,omitempty"`
	Connected    bool   `json:"connected"`
	LastError    string `json:"last_error,omitempty"`
}

type HostNotifyStatus struct {
	Enabled   bool   `json:"enabled"`
	Sink      string `json:"sink"`
	FilePath  string `json:"file_path,omitempty"`
	HookURL   string `json:"hook_url,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	HookName  string `json:"hook_name,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

type Status struct {
	Mode       string           `json:"mode"`
	Running    bool             `json:"running"`
	PID        int              `json:"pid,omitempty"`
	PIDFile    string           `json:"pid_file,omitempty"`
	SocketPath string           `json:"socket_path,omitempty"`
	LogFile    string           `json:"log_file,omitempty"`
	StatusFile string           `json:"status_file,omitempty"`
	StartedAt  string           `json:"started_at,omitempty"`
	Sessions   []SessionStatus  `json:"sessions,omitempty"`
	HostNotify HostNotifyStatus `json:"host_notify"`
	Warnings   []string         `json:"warnings,omitempty"`
}
