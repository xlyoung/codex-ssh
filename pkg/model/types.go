package model

import "time"

type Paths struct {
	DataDir    string
	ConfigFile string
	HostsFile  string
	LogDir     string
	RunDir     string
	ControlDir string
	TunnelsDir string
	ProxiesDir string
	JobsDir    string
	AskpassDir string
}

type Config struct {
	Version                  int         `toml:"version"`
	DataDir                  string      `toml:"data_dir"`
	LogDir                   string      `toml:"log_dir"`
	RunDir                   string      `toml:"run_dir"`
	DefaultUser              string      `toml:"default_user"`
	DefaultPort              int         `toml:"default_port"`
	DefaultAuth              string      `toml:"default_auth"`
	DefaultKeepaliveInterval int         `toml:"default_keepalive_interval"`
	DefaultKeepaliveCountMax int         `toml:"default_keepalive_count_max"`
	DefaultConnectTimeout    int         `toml:"default_connect_timeout"`
	DefaultControlMaster     string      `toml:"default_control_master"`
	DefaultControlPersist    string      `toml:"default_control_persist"`
	DefaultTunnelTTL         string      `toml:"default_tunnel_ttl"`
	DefaultLongJobMode       string      `toml:"default_long_job_mode"`
	Security                 Security    `toml:"security"`
	Audit                    AuditConfig `toml:"audit"`
}

type Security struct {
	StrictHostKeyChecking bool `toml:"strict_host_key_checking"`
	ReuseSystemKnownHosts bool `toml:"reuse_system_known_hosts"`
	AllowPasswordAuth     bool `toml:"allow_password_auth"`
	AllowRoot             bool `toml:"allow_root"`
}

type AuditConfig struct {
	Format         string `toml:"format"`
	CaptureStdout  bool   `toml:"capture_stdout"`
	CaptureStderr  bool   `toml:"capture_stderr"`
	RedactEnv      bool   `toml:"redact_env"`
	MaxOutputBytes int    `toml:"max_output_bytes"`
}

type Inventory struct {
	Version int             `toml:"version"`
	Hosts   map[string]Host `toml:"hosts"`
}

type Host struct {
	Host         string   `toml:"host" json:"host"`
	User         string   `toml:"user" json:"user"`
	Port         int      `toml:"port" json:"port"`
	Via          []string `toml:"via" json:"via"`
	Tags         []string `toml:"tags" json:"tags,omitempty"`
	Workdir      string   `toml:"workdir" json:"workdir,omitempty"`
	Auth         string   `toml:"auth" json:"auth,omitempty"`
	IdentityFile string   `toml:"identity_file" json:"identity_file,omitempty"`
	SecretRef    string   `toml:"secret_ref" json:"secret_ref,omitempty"`
}

type ResolvedHost struct {
	Alias        string
	Host         string
	User         string
	Port         int
	Via          []ResolvedHost
	Tags         []string
	Workdir      string
	Auth         string
	IdentityFile string
	SecretRef    string
}

type ExecRequest struct {
	Alias        string
	Command      string
	CWD          string
	Timeout      time.Duration
	AuthEnv      map[string]string
	ResolvedHost ResolvedHost
}

type ShellRequest struct {
	Alias        string
	CWD          string
	AuthEnv      map[string]string
	ResolvedHost ResolvedHost
}

type TunnelRequest struct {
	ID           string
	Alias        string
	LocalHost    string
	LocalPort    int
	TargetHost   string
	TargetPort   int
	AuthEnv      map[string]string
	Background   bool
	ResolvedHost ResolvedHost
}

type ProxyRequest struct {
	ID           string
	Alias        string
	LocalHost    string
	LocalPort    int
	AuthEnv      map[string]string
	Background   bool
	ResolvedHost ResolvedHost
}

type JobRequest struct {
	ID           string
	Alias        string
	Command      string
	CWD          string
	Mode         string
	AuthEnv      map[string]string
	ResolvedHost ResolvedHost
}

type AuditEvent struct {
	Timestamp    time.Time `json:"ts"`
	EventID      string    `json:"event_id"`
	Action       string    `json:"action"`
	HostAlias    string    `json:"host_alias"`
	ResolvedHost string    `json:"resolved_host,omitempty"`
	User         string    `json:"user,omitempty"`
	Port         int       `json:"port,omitempty"`
	Via          []string  `json:"via,omitempty"`
	Command      string    `json:"command,omitempty"`
	CWD          string    `json:"cwd,omitempty"`
	Mode         string    `json:"mode,omitempty"`
	StartTime    time.Time `json:"start_time,omitempty"`
	EndTime      time.Time `json:"end_time,omitempty"`
	DurationMS   int64     `json:"duration_ms,omitempty"`
	ExitCode     int       `json:"exit_code,omitempty"`
	StdoutBytes  int       `json:"stdout_bytes,omitempty"`
	StderrBytes  int       `json:"stderr_bytes,omitempty"`
	Status       string    `json:"status"`
	LocalHost    string    `json:"local_host,omitempty"`
	LocalPort    int       `json:"local_port,omitempty"`
	TargetHost   string    `json:"target_host,omitempty"`
	TargetPort   int       `json:"target_port,omitempty"`
	PID          int       `json:"pid,omitempty"`
	Background   bool      `json:"background,omitempty"`
	SessionName  string    `json:"session_name,omitempty"`
	JobID        string    `json:"job_id,omitempty"`
	StdoutPath   string    `json:"stdout_path,omitempty"`
	StderrPath   string    `json:"stderr_path,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

type AuditQuery struct {
	HostAlias string
	Action    string
	Status    string
	Command   string
	Since     time.Time
	Until     time.Time
	Limit     int
	Export    string // "json" or "csv"
}

type ProcessState struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Alias      string    `json:"alias"`
	PID        int       `json:"pid"`
	LocalHost  string    `json:"local_host,omitempty"`
	LocalPort  int       `json:"local_port,omitempty"`
	TargetHost string    `json:"target_host,omitempty"`
	TargetPort int       `json:"target_port,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	LogPath    string    `json:"log_path,omitempty"`
}

type JobState struct {
	ID            string       `json:"id"`
	Alias         string       `json:"alias"`
	Mode          string       `json:"mode"`
	Status        string       `json:"status"`
	Command       string       `json:"command"`
	CWD           string       `json:"cwd,omitempty"`
	Connection    ResolvedHost `json:"connection,omitempty"`
	SessionName   string       `json:"session_name,omitempty"`
	RemotePIDFile string       `json:"remote_pid_file,omitempty"`
	RemoteLogFile string       `json:"remote_log_file,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
}

type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}
