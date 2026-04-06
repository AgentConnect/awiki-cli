package listener

type SessionStatus struct {
	IdentityName string `json:"identity_name"`
	DID          string `json:"did,omitempty"`
	Connected    bool   `json:"connected"`
	LastError    string `json:"last_error,omitempty"`
}

type Status struct {
	Mode       string          `json:"mode"`
	Running    bool            `json:"running"`
	PID        int             `json:"pid,omitempty"`
	PIDFile    string          `json:"pid_file,omitempty"`
	SocketPath string          `json:"socket_path,omitempty"`
	LogFile    string          `json:"log_file,omitempty"`
	StatusFile string          `json:"status_file,omitempty"`
	StartedAt  string          `json:"started_at,omitempty"`
	Sessions   []SessionStatus `json:"sessions,omitempty"`
	Warnings   []string        `json:"warnings,omitempty"`
}
