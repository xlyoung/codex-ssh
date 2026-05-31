package health

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Status represents the health status of a check.
type Status string

const (
	Healthy  Status = "healthy"
	Degraded Status = "degraded"
	Critical Status = "critical"
	Unknown  Status = "unknown"
)

// Check represents a single health check result.
type Check struct {
	Name    string  `json:"name"`
	Status  Status  `json:"status"`
	Message string  `json:"message"`
	Value   string  `json:"value,omitempty"`
}

// Report represents the health report for a host.
type Report struct {
	Host    string   `json:"host"`
	Status  Status   `json:"status"`
	Checks  []Check  `json:"checks"`
	Time    time.Time `json:"time"`
}

// CheckHost performs health checks on a single host via SSH.
func CheckHost(host string) Report {
	report := Report{
		Host:   host,
		Status: Healthy,
		Time:   time.Now(),
	}

	// Check 1: SSH connectivity
	connectivity := checkSSHConnectivity(host)
	report.Checks = append(report.Checks, connectivity)
	if connectivity.Status == Critical {
		report.Status = Critical
		return report
	}

	// Check 2: CPU usage
	cpu := checkCPU(host)
	report.Checks = append(report.Checks, cpu)
	if cpu.Status == Critical {
		report.Status = Critical
	}

	// Check 3: Memory usage
	mem := checkMemory(host)
	report.Checks = append(report.Checks, mem)
	if mem.Status == Critical {
		report.Status = Critical
	} else if mem.Status == Degraded && report.Status == Healthy {
		report.Status = Degraded
	}

	// Check 4: Disk usage
	disk := checkDisk(host)
	report.Checks = append(report.Checks, disk)
	if disk.Status == Critical {
		report.Status = Critical
	} else if disk.Status == Degraded && report.Status == Healthy {
		report.Status = Degraded
	}

	// Check 5: Load average
	load := checkLoad(host)
	report.Checks = append(report.Checks, load)
	if load.Status == Degraded && report.Status == Healthy {
		report.Status = Degraded
	}

	return report
}

func checkSSHConnectivity(host string) Check {
	cmd := exec.Command("codex-ssh", "exec", host, "--", "echo ok")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Check{
			Name:    "ssh_connectivity",
			Status:  Critical,
			Message: fmt.Sprintf("SSH connection failed: %v", err),
		}
	}
	if strings.TrimSpace(string(output)) == "ok" {
		return Check{
			Name:    "ssh_connectivity",
			Status:  Healthy,
			Message: "SSH connection successful",
		}
	}
	return Check{
		Name:    "ssh_connectivity",
		Status:  Degraded,
		Message: "SSH connected but unexpected output",
	}
}

func checkCPU(host string) Check {
	cmd := exec.Command("codex-ssh", "exec", host, "--", "top -bn1 | grep 'Cpu(s)' | awk '{print $2}'")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Check{
			Name:    "cpu",
			Status:  Unknown,
			Message: fmt.Sprintf("Failed to check CPU: %v", err),
		}
	}

	usage := strings.TrimSpace(string(output))
	var pct float64
	fmt.Sscanf(usage, "%f", &pct)

	if pct > 90 {
		return Check{Name: "cpu", Status: Critical, Message: fmt.Sprintf("CPU usage critical: %.1f%%", pct), Value: usage}
	}
	if pct > 70 {
		return Check{Name: "cpu", Status: Degraded, Message: fmt.Sprintf("CPU usage high: %.1f%%", pct), Value: usage}
	}
	return Check{Name: "cpu", Status: Healthy, Message: fmt.Sprintf("CPU usage normal: %.1f%%", pct), Value: usage}
}

func checkMemory(host string) Check {
	cmd := exec.Command("codex-ssh", "exec", host, "--", "free | awk '/Mem:/ {printf \"%.1f\", $3/$2*100}'")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Check{Name: "memory", Status: Unknown, Message: fmt.Sprintf("Failed to check memory: %v", err)}
	}

	usage := strings.TrimSpace(string(output))
	var pct float64
	fmt.Sscanf(usage, "%f", &pct)

	if pct > 90 {
		return Check{Name: "memory", Status: Critical, Message: fmt.Sprintf("Memory usage critical: %.1f%%", pct), Value: usage}
	}
	if pct > 70 {
		return Check{Name: "memory", Status: Degraded, Message: fmt.Sprintf("Memory usage high: %.1f%%", pct), Value: usage}
	}
	return Check{Name: "memory", Status: Healthy, Message: fmt.Sprintf("Memory usage normal: %.1f%%", pct), Value: usage}
}

func checkDisk(host string) Check {
	cmd := exec.Command("codex-ssh", "exec", host, "--", "df -h / | awk 'NR==2 {print $5}' | tr -d '%'")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Check{Name: "disk", Status: Unknown, Message: fmt.Sprintf("Failed to check disk: %v", err)}
	}

	usage := strings.TrimSpace(string(output))
	var pct int
	fmt.Sscanf(usage, "%d", &pct)

	if pct > 90 {
		return Check{Name: "disk", Status: Critical, Message: fmt.Sprintf("Disk usage critical: %d%%", pct), Value: usage}
	}
	if pct > 70 {
		return Check{Name: "disk", Status: Degraded, Message: fmt.Sprintf("Disk usage high: %d%%", pct), Value: usage}
	}
	return Check{Name: "disk", Status: Healthy, Message: fmt.Sprintf("Disk usage normal: %d%%", pct), Value: usage}
}

func checkLoad(host string) Check {
	cmd := exec.Command("codex-ssh", "exec", host, "--", "cat /proc/loadavg | awk '{print $1}'")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Check{Name: "load", Status: Unknown, Message: fmt.Sprintf("Failed to check load: %v", err)}
	}

	load := strings.TrimSpace(string(output))
	var load1 float64
	fmt.Sscanf(load, "%f", &load1)

	if load1 > 10 {
		return Check{Name: "load", Status: Critical, Message: fmt.Sprintf("Load average critical: %s", load), Value: load}
	}
	if load1 > 5 {
		return Check{Name: "load", Status: Degraded, Message: fmt.Sprintf("Load average high: %s", load), Value: load}
	}
	return Check{Name: "load", Status: Healthy, Message: fmt.Sprintf("Load average normal: %s", load), Value: load}
}

// FormatTable formats a health report as a table.
func FormatTable(report Report) string {
	var sb strings.Builder

	statusEmoji := map[Status]string{
		Healthy:  "✅",
		Degraded: "⚠️",
		Critical: "❌",
		Unknown:  "❓",
	}

	sb.WriteString(fmt.Sprintf("🖥️  Host: %s\n", report.Host))
	sb.WriteString(fmt.Sprintf("📊 Overall: %s %s\n\n", statusEmoji[report.Status], strings.ToUpper(string(report.Status))))

	for _, check := range report.Checks {
		sb.WriteString(fmt.Sprintf("  %s %-15s %s\n", statusEmoji[check.Status], check.Name, check.Message))
	}

	return sb.String()
}

// FormatJSON formats a health report as JSON.
func FormatJSON(report Report) string {
	return fmt.Sprintf(`{"host":"%s","status":"%s","time":"%s","checks":%d}`,
		report.Host, report.Status, report.Time.Format(time.RFC3339), len(report.Checks))
}
