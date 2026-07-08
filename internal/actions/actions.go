package actions

import (
	"context"
	"fmt"
	"os/exec"
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
	s.mu.Unlock()
	go run(ctx, job, command, args, input)
	return job, nil
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
	if s.credential == nil {
		return Credential{}
	}
	return Credential{Mode: s.credential.Mode, Configured: true, Created: s.credential.Created}
}

func (s *Service) privilegeCredential() (string, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.credential == nil || s.credential.password == "" {
		return "", "", false
	}
	return s.credential.Mode, s.credential.password, true
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

func (s *Service) command(name string) (string, []string, bool) {
	switch name {
	case "apt-update":
		return "sudo", []string{"-n", "apt-get", "update", "-qq"}, true
	case "apt-upgrade":
		return "sudo", []string{"-n", "apt-get", "upgrade", "-y", "-qq"}, true
	case "restart-service":
		return "systemctl", []string{"--user", "restart", s.cfg.Update.Service}, true
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
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		job.append(strings.TrimSpace(string(output)))
	}
	if err != nil {
		job.append("error: " + err.Error())
		job.setExit(1)
		return
	}
	job.setExit(0)
}

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
