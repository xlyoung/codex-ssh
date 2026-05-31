package playbook

// Playbook represents an operations workflow defined in YAML.
type Playbook struct {
	Name  string `yaml:"name"`
	Hosts string `yaml:"hosts"` // tag, @all, or comma-separated host list
	Steps []Step `yaml:"steps"`
}

// Step defines a single operation to execute on target hosts.
type Step struct {
	Name         string `yaml:"name"`
	Exec         string `yaml:"exec"`
	Sudo         bool   `yaml:"sudo,omitempty"`
	Retries      int    `yaml:"retries,omitempty"`
	Delay        int    `yaml:"delay,omitempty"`
	When         string `yaml:"when,omitempty"`
	IgnoreErrors bool   `yaml:"ignore_errors,omitempty"`
	FailedWhen   string `yaml:"failed_when,omitempty"`
}

// StepResult captures the outcome of executing a step on a single host.
type StepResult struct {
	Alias    string
	StepName string
	Exec     string
	Stdout   string
	Stderr   string
	ExitCode int
	Skipped  bool
	Err      error
}

// PlaybookResult captures the overall outcome of running a playbook.
type PlaybookResult struct {
	Name      string
	Hosts     []string
	Results   []StepResult
	Failed    bool
	Skipped   int
	Changed   int
	Errors    int
}
