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
	LastError string `json:"last_error,omitempty"`
}

type Status struct {
	Mode            string           `json:"mode"`
	Installed       bool             `json:"installed"`
	Running         bool             `json:"running"`
	PID             int              `json:"pid,omitempty"`
	PIDFile         string           `json:"pid_file,omitempty"`
	SocketPath      string           `json:"socket_path,omitempty"`
	LogFile         string           `json:"log_file,omitempty"`
	StatusFile      string           `json:"status_file,omitempty"`
	ServiceName     string           `json:"service_name,omitempty"`
	ServicePlatform string           `json:"service_platform,omitempty"`
	BridgeAvailable bool             `json:"bridge_available"`
	StartedAt       string           `json:"started_at,omitempty"`
	Sessions        []SessionStatus  `json:"sessions,omitempty"`
	HostNotify      HostNotifyStatus `json:"host_notify"`
	Warnings        []string         `json:"warnings,omitempty"`
}
