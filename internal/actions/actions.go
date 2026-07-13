package actions

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
)

type Service struct {
	cfg        *config.Config
	mu         sync.Mutex
	jobs       map[string]*Job
	credential *Credential
}

type Job struct {
	mu       sync.Mutex `json:"-"`
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Started  time.Time  `json:"started"`
	Finished *time.Time `json:"finished,omitempty"`
	ExitCode int        `json:"exit_code"`
	Output   []string   `json:"output"`
}

type Credential struct {
	Mode       string    `json:"mode"`
	Configured bool      `json:"configured"`
	Created    time.Time `json:"created"`
	password   string
}

func New(cfg *config.Config) *Service {
	return &Service{cfg: cfg, jobs: map[string]*Job{}}
}

func (s *Service) Start(ctx context.Context, name string) (*Job, error) {
	return s.StartPrivileged(ctx, name, "", "")
}

func (s *Service) StartPrivileged(ctx context.Context, name string, mode string, password string) (*Job, error) {
	command, args, ok := s.command(name)
	if !ok {
		return nil, fmt.Errorf("unknown action: %s", name)
	}
	if password == "" {
		if storedMode, storedPassword, ok := s.privilegeCredential(); ok {
			mode = storedMode
			password = storedPassword
		}
	}
	input := ""
	if password != "" {
		switch mode {
		case "sudo":
			command, args, input = sudoPasswordCommand(command, args, password)
		case "su":
			command, args, input = suPasswordCommand(command, args, password)
		default:
			return nil, fmt.Errorf("privilege mode must be sudo or su")
		}
	}
	job := &Job{ID: fmt.Sprintf("%d", time.Now().UnixNano()), Name: name, Started: time.Now(), ExitCode: -1}
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.pruneJobsLocked(100)
	s.mu.Unlock()
	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), actionTimeout(name))
	go func() {
		defer cancel()
		run(jobCtx, job, command, args, input)
	}()
	return job, nil
}

func (s *Service) StartTimeConfig(ctx context.Context, timezone string, server string, mode string, password string) (*Job, error) {
	timezone = strings.TrimSpace(timezone)
	server = strings.TrimSpace(server)
	if timezone == "" || server == "" {
		return nil, fmt.Errorf("timezone and NTP server are required")
	}
	if strings.ContainsAny(timezone+server, "\r\n\x00") {
		return nil, fmt.Errorf("invalid time configuration")
	}
	zonePath := "/usr/share/zoneinfo/" + timezone
	if timezone != "UTC" {
		if info, err := os.Stat(zonePath); err != nil || info.IsDir() {
			return nil, fmt.Errorf("unknown timezone: %s", timezone)
		}
	}
	command := "sudo"
	args := []string{"-n", "sh", "-c", `set -eu
timedatectl set-timezone "$1"
mkdir -p /etc/systemd/timesyncd.conf.d
printf '[Time]\nNTP=%s\n' "$2" > /etc/systemd/timesyncd.conf.d/kioskmate.conf
timedatectl set-ntp true
if systemctl list-unit-files systemd-timesyncd.service >/dev/null 2>&1; then systemctl restart systemd-timesyncd.service; fi`, "kioskmate-time", timezone, server}
	input := ""
	if password == "" {
		if storedMode, storedPassword, ok := s.privilegeCredential(); ok {
			mode, password = storedMode, storedPassword
		}
	}
	if password != "" {
		switch mode {
		case "sudo":
			command, args, input = sudoPasswordCommand(command, args, password)
		case "su":
			command, args, input = suPasswordCommand(command, args, password)
		default:
			return nil, fmt.Errorf("privilege mode must be sudo or su")
		}
	}
	return s.startCommand(ctx, "time-config", command, args, input, 2*time.Minute), nil
}

func (s *Service) startCommand(ctx context.Context, name string, command string, args []string, input string, timeout time.Duration) *Job {
	job := &Job{ID: fmt.Sprintf("%d", time.Now().UnixNano()), Name: name, Started: time.Now(), ExitCode: -1}
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.pruneJobsLocked(100)
	s.mu.Unlock()
	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	go func() {
		defer cancel()
		run(jobCtx, job, command, args, input)
	}()
	return job
}

func (s *Service) RememberPrivilege(mode string, password string) error {
	if password == "" {
		return nil
	}
	if mode != "sudo" && mode != "su" {
		return fmt.Errorf("privilege mode must be sudo or su")
	}
	s.mu.Lock()
	s.credential = &Credential{Mode: mode, Configured: true, Created: time.Now(), password: password}
	s.mu.Unlock()
	return nil
}

func (s *Service) ClearPrivilege() {
	s.mu.Lock()
	s.credential = nil
	s.mu.Unlock()
}

func (s *Service) PrivilegeStatus() Credential {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.credential == nil || time.Since(s.credential.Created) > 15*time.Minute {
		s.credential = nil
		return Credential{}
	}
	return Credential{Mode: s.credential.Mode, Configured: true, Created: s.credential.Created}
}

func (s *Service) privilegeCredential() (string, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.credential == nil || s.credential.password == "" || time.Since(s.credential.Created) > 15*time.Minute {
		s.credential = nil
		return "", "", false
	}
	return s.credential.Mode, s.credential.password, true
}

func actionTimeout(name string) time.Duration {
	switch name {
	case "apt-update", "apt-upgrade":
		return 2 * time.Hour
	default:
		return 2 * time.Minute
	}
}

func (s *Service) pruneJobsLocked(keep int) {
	for len(s.jobs) > keep {
		var oldestID string
		var oldest time.Time
		for id, job := range s.jobs {
			if oldestID == "" || job.Started.Before(oldest) {
				oldestID = id
				oldest = job.Started
			}
		}
		delete(s.jobs, oldestID)
	}
}

func (s *Service) Job(id string) (*Job, bool) {
	s.mu.Lock()
	job, ok := s.jobs[id]
	s.mu.Unlock()
	if !ok {
		return nil, false
	}
	return job.snapshot(), true
}

func (s *Service) Jobs(limit int) []*Job {
	s.mu.Lock()
	jobs := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job.snapshot())
	}
	s.mu.Unlock()
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Started.After(jobs[j].Started) })
	if limit > 0 && len(jobs) > limit {
		jobs = jobs[:limit]
	}
	return jobs
}

func (s *Service) command(name string) (string, []string, bool) {
	switch name {
	case "apt-update":
		return "sudo", []string{"-n", "apt-get", "update"}, true
	case "apt-upgrade":
		return "sudo", []string{"-n", "apt-get", "upgrade", "-y"}, true
	case "restart-service":
		return "systemctl", []string{"--user", "restart", s.cfg.Snapshot().Update.Service}, true
	case "reboot":
		return "sudo", []string{"-n", "reboot"}, true
	case "shutdown":
		return "sudo", []string{"-n", "shutdown", "-h", "now"}, true
	default:
		return "", nil, false
	}
}

func sudoPasswordCommand(command string, args []string, password string) (string, []string, string) {
	if command != "sudo" {
		return command, args, ""
	}
	filtered := []string{"-S", "-p", ""}
	for _, arg := range args {
		if arg != "-n" {
			filtered = append(filtered, arg)
		}
	}
	return command, filtered, password + "\n"
}

func suPasswordCommand(command string, args []string, password string) (string, []string, string) {
	line := command + " " + shellJoin(args)
	return "su", []string{"-", "root", "-c", line}, password + "\n"
}

func run(ctx context.Context, job *Job, command string, args []string, input string) {
	defer job.finishDefault(1)
	job.append(command + " " + strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, command, args...)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	writer := &jobWriter{job: job}
	cmd.Stdout = writer
	cmd.Stderr = writer
	err := cmd.Run()
	writer.flush()
	if err != nil {
		job.append("error: " + err.Error())
		job.setExit(1)
		return
	}
	job.setExit(0)
}

type jobWriter struct {
	mu      sync.Mutex
	job     *Job
	pending string
}

func (w *jobWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending += string(data)
	for {
		index := strings.IndexByte(w.pending, '\n')
		if index < 0 {
			break
		}
		line := strings.TrimSpace(w.pending[:index])
		w.pending = w.pending[index+1:]
		if line != "" {
			w.job.append(line)
		}
	}
	return len(data), nil
}

func (w *jobWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if line := strings.TrimSpace(w.pending); line != "" {
		w.job.append(line)
	}
	w.pending = ""
}

var _ io.Writer = (*jobWriter)(nil)

func shellJoin(args []string) string {
	var out []string
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t\n'\"\\$") {
			out = append(out, "'"+strings.ReplaceAll(arg, "'", "'\\''")+"'")
		} else {
			out = append(out, arg)
		}
	}
	return strings.Join(out, " ")
}

func (j *Job) append(line string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Output = append(j.Output, line)
}

func (j *Job) setExit(code int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.ExitCode = code
}

func (j *Job) finishDefault(code int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.ExitCode < 0 {
		j.ExitCode = code
	}
	now := time.Now()
	j.Finished = &now
}

func (j *Job) snapshot() *Job {
	j.mu.Lock()
	defer j.mu.Unlock()
	return &Job{
		ID:       j.ID,
		Name:     j.Name,
		Started:  j.Started,
		Finished: j.Finished,
		ExitCode: j.ExitCode,
		Output:   append([]string(nil), j.Output...),
	}
}
