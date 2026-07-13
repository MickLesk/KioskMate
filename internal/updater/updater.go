package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
)

type Service struct {
	cfg       *config.Config
	version   string
	client    *http.Client
	privilege PrivilegeProvider

	mu         sync.Mutex
	checkMu    sync.Mutex
	jobs       map[string]*Job
	status     ReleaseInfo
	installing bool
	history    []HistoryEntry
}

type PrivilegeProvider interface {
	ResolvePrivilege(mode string, password string) (string, string, bool)
}

var (
	ErrPrivilegeRequired = errors.New("administrator privileges are required to install updates")
	ErrUpdateInProgress  = errors.New("an update installation is already running")
)

type Job struct {
	mu       sync.Mutex `json:"-"`
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Stage    string     `json:"stage"`
	Target   string     `json:"target_version,omitempty"`
	Error    string     `json:"error,omitempty"`
	Started  time.Time  `json:"started"`
	Finished *time.Time `json:"finished,omitempty"`
	ExitCode int        `json:"exit_code"`
	Output   []string   `json:"output"`
}

type ReleaseInfo struct {
	CurrentVersion  string     `json:"current_version"`
	LatestVersion   string     `json:"latest_version"`
	UpdateAvailable bool       `json:"update_available"`
	URL             string     `json:"url"`
	Changelog       string     `json:"changelog"`
	Asset           Asset      `json:"asset"`
	Supported       bool       `json:"supported"`
	Checking        bool       `json:"checking"`
	Installing      bool       `json:"installing"`
	CheckedAt       *time.Time `json:"checked_at,omitempty"`
	Error           string     `json:"error,omitempty"`
}

type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	HTMLURL     string        `json:"html_url"`
	Body        string        `json:"body"`
	Assets      []githubAsset `json:"assets"`
	PublishedAt string        `json:"published_at"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
	Size               int64  `json:"size"`
}

func New(cfg *config.Config, version string, privilege ...PrivilegeProvider) *Service {
	var provider PrivilegeProvider
	if len(privilege) > 0 {
		provider = privilege[0]
	}
	service := &Service{
		cfg: cfg, version: version, privilege: provider,
		client: &http.Client{Timeout: 45 * time.Second},
		jobs:   map[string]*Job{}, status: ReleaseInfo{CurrentVersion: version},
	}
	return service.loadHistory()
}

func (s *Service) Check(ctx context.Context) (ReleaseInfo, error) {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	s.setChecking(true)
	release, err := s.latest(ctx)
	if err != nil {
		s.storeCheck(ReleaseInfo{CurrentVersion: s.version, Error: err.Error()}, err)
		return ReleaseInfo{}, err
	}
	asset := selectAsset(release.Assets)
	latest := strings.TrimPrefix(release.TagName, "v")
	info := ReleaseInfo{
		CurrentVersion:  s.version,
		LatestVersion:   latest,
		UpdateAvailable: compareVersion(latest, s.version) > 0,
		URL:             release.HTMLURL,
		Changelog:       release.Body,
		Asset:           asset,
		Supported:       runtime.GOOS == "linux" && asset.URL != "",
	}
	s.storeCheck(info, nil)
	return s.Status(), nil
}

func (s *Service) Status() ReleaseInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.status
	status.Installing = s.installing
	return status
}

func (s *Service) setChecking(checking bool) {
	s.mu.Lock()
	s.status.CurrentVersion = s.version
	s.status.Checking = checking
	s.mu.Unlock()
}

func (s *Service) storeCheck(info ReleaseInfo, checkErr error) {
	now := time.Now()
	info.CurrentVersion = s.version
	info.Checking = false
	info.CheckedAt = &now
	s.mu.Lock()
	if checkErr != nil {
		info.LatestVersion = s.status.LatestVersion
		info.UpdateAvailable = s.status.UpdateAvailable
		info.URL = s.status.URL
		info.Changelog = s.status.Changelog
		info.Asset = s.status.Asset
		info.Supported = s.status.Supported
		info.Error = checkErr.Error()
	}
	info.Installing = s.installing
	s.status = info
	s.mu.Unlock()
}

func (s *Service) Run(ctx context.Context) {
	check := func() {
		checkCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		_, _ = s.Check(checkCtx)
	}
	check()
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}

func (s *Service) Install(ctx context.Context, mode string, password string) (*Job, error) {
	return s.startInstall(ctx, "update-install", "", mode, password, false)
}

func (s *Service) Rollback(ctx context.Context, mode string, password string) (*Job, error) {
	target := s.History().RollbackTarget
	if target == "" {
		return nil, errors.New("no previously installed version is available for rollback")
	}
	return s.startInstall(ctx, "update-rollback", target, mode, password, true)
}

func (s *Service) startInstall(ctx context.Context, name, target, mode, password string, rollback bool) (*Job, error) {
	s.mu.Lock()
	if s.installing {
		s.mu.Unlock()
		return nil, ErrUpdateInProgress
	}
	s.mu.Unlock()
	privilege, err := s.preparePrivilege(ctx, mode, password)
	if err != nil {
		return nil, err
	}
	job := &Job{ID: fmt.Sprintf("%d", time.Now().UnixNano()), Name: name, Stage: "preparing", Target: target, Started: time.Now(), ExitCode: -1}
	s.mu.Lock()
	if s.installing {
		s.mu.Unlock()
		return nil, ErrUpdateInProgress
	}
	s.installing = true
	s.jobs[job.ID] = job
	s.pruneJobsLocked(50)
	s.mu.Unlock()
	s.beginHistory(job, strings.TrimPrefix(name, "update-"), target)
	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Minute)
	go func() {
		defer cancel()
		if rollback {
			s.rollback(jobCtx, job, privilege, target)
		} else {
			s.install(jobCtx, job, privilege)
		}
	}()
	return job, nil
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

type privilegeCommand struct {
	Mode     string
	Password string
}

func (s *Service) preparePrivilege(ctx context.Context, mode string, password string) (privilegeCommand, error) {
	configured := false
	if s.privilege != nil {
		mode, password, configured = s.privilege.ResolvePrivilege(mode, password)
	} else if password != "" {
		configured = mode == "sudo" || mode == "su"
	}
	if mode == "" {
		mode = "sudo"
	}
	if !configured && mode != "sudo" {
		return privilegeCommand{}, ErrPrivilegeRequired
	}
	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var command *exec.Cmd
	if password == "" {
		command = exec.CommandContext(testCtx, "sudo", "-n", "true")
	} else if mode == "sudo" {
		command = exec.CommandContext(testCtx, "sudo", "-S", "-p", "", "-v")
		command.Stdin = strings.NewReader(password + "\n")
	} else if mode == "su" {
		command = exec.CommandContext(testCtx, "su", "-", "root", "-c", "true")
		command.Stdin = strings.NewReader(password + "\n")
	} else {
		return privilegeCommand{}, fmt.Errorf("privilege mode must be sudo or su")
	}
	if output, err := command.CombinedOutput(); err != nil {
		if password == "" {
			return privilegeCommand{}, ErrPrivilegeRequired
		}
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return privilegeCommand{}, fmt.Errorf("administrator authentication failed: %s", detail)
	}
	return privilegeCommand{Mode: mode, Password: password}, nil
}

func (s *Service) install(ctx context.Context, job *Job, privilege privilegeCommand) {
	defer s.finishInstallJob(job)
	s.setJobStage(job, "checking")
	s.log(job, "checking latest release")
	info, err := s.Check(ctx)
	if err != nil {
		s.fail(job, err)
		return
	}
	if !info.UpdateAvailable {
		s.log(job, "already up to date")
		job.setTarget(s.version)
		s.setJobStage(job, "completed")
		job.setExit(0)
		s.updateHistory(job, "installed")
		return
	}
	s.applyRelease(ctx, job, privilege, info, false)
}

func (s *Service) rollback(ctx context.Context, job *Job, privilege privilegeCommand, target string) {
	defer s.finishInstallJob(job)
	s.setJobStage(job, "checking")
	s.log(job, "checking rollback release "+target)
	release, err := s.releaseVersion(ctx, target)
	if err != nil {
		s.fail(job, err)
		return
	}
	info := s.releaseInfo(release)
	info.UpdateAvailable = true
	s.applyRelease(ctx, job, privilege, info, true)
}

func (s *Service) applyRelease(ctx context.Context, job *Job, privilege privilegeCommand, info ReleaseInfo, allowDowngrade bool) {
	job.setTarget(info.LatestVersion)
	s.updateHistory(job, "running")
	if !info.Supported {
		s.fail(job, errors.New("no supported .deb asset for this platform"))
		return
	}
	if err := s.validateEnvironment(info.Asset); err != nil {
		s.fail(job, err)
		return
	}
	file := filepath.Join(os.TempDir(), info.Asset.Name)
	defer os.Remove(file)
	s.setJobStage(job, "downloading")
	if err := s.download(ctx, info.Asset.URL, file, info.Asset.Size, job); err != nil {
		s.fail(job, err)
		return
	}
	if err := verify(file, info.Asset.Digest); err != nil {
		s.fail(job, err)
		return
	}
	if err := validateDebPackage(ctx, file); err != nil {
		s.fail(job, err)
		return
	}
	backup, err := s.backupConfig(job)
	if err != nil {
		s.fail(job, fmt.Errorf("backup current configuration: %w", err))
		return
	}
	s.log(job, "configuration backup: "+backup)
	s.setJobStage(job, "installing")
	args := []string{"install", "-y"}
	if allowDowngrade {
		args = append(args, "--allow-downgrades")
	}
	args = append(args, file)
	if err := runPrivileged(ctx, job, privilege, "apt-get", args...); err != nil {
		s.fail(job, err)
		return
	}
	s.setJobStage(job, "restarting")
	s.log(job, "installation complete; KioskMate will restart in 2 seconds")
	job.setExit(0)
	s.updateHistory(job, "restarting")
	go s.restartServiceAfterUpdate(job)
}

func (s *Service) finishInstallJob(job *Job) {
	job.finishDefault(1)
	if job.snapshot().Stage == "restarting" {
		return
	}
	s.mu.Lock()
	s.installing = false
	s.mu.Unlock()
}

func (s *Service) restartServiceAfterUpdate(job *Job) {
	time.Sleep(2 * time.Second)
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		s.log(job, "warning: systemd daemon-reload failed: "+err.Error())
	}
	if err := exec.Command("systemctl", "--user", "restart", s.cfg.Snapshot().Update.Service).Run(); err != nil {
		s.fail(job, fmt.Errorf("restart KioskMate service: %w", err))
		s.mu.Lock()
		s.installing = false
		s.mu.Unlock()
	}
}

func (s *Service) latest(ctx context.Context) (githubRelease, error) {
	cfg := s.cfg.Snapshot()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+cfg.Update.Repository+"/releases", nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := s.client.Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return githubRelease{}, fmt.Errorf("github releases failed: %s", resp.Status)
	}
	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return githubRelease{}, err
	}
	for _, release := range releases {
		if !release.Draft && (!release.Prerelease || cfg.Update.Prerelease) {
			return release, nil
		}
	}
	return githubRelease{}, errors.New("no release found")
}

func (s *Service) releaseVersion(ctx context.Context, version string) (githubRelease, error) {
	cfg := s.cfg.Snapshot()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+cfg.Update.Repository+"/releases", nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := s.client.Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return githubRelease{}, fmt.Errorf("github releases failed: %s", resp.Status)
	}
	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return githubRelease{}, err
	}
	for _, release := range releases {
		if !release.Draft && strings.TrimPrefix(release.TagName, "v") == strings.TrimPrefix(version, "v") {
			return release, nil
		}
	}
	return githubRelease{}, fmt.Errorf("release %s not found", version)
}

func (s *Service) releaseInfo(release githubRelease) ReleaseInfo {
	latest := strings.TrimPrefix(release.TagName, "v")
	asset := selectAsset(release.Assets)
	return ReleaseInfo{
		CurrentVersion: s.version, LatestVersion: latest, UpdateAvailable: compareVersion(latest, s.version) != 0,
		URL: release.HTMLURL, Changelog: release.Body, Asset: asset,
		Supported: runtime.GOOS == "linux" && asset.URL != "",
	}
}

func (s *Service) download(ctx context.Context, url, file string, expectedSize int64, job *Job) error {
	s.log(job, "downloading "+filepath.Base(file))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	const maxPackageSize = int64(512 << 20)
	if resp.ContentLength > maxPackageSize || expectedSize > maxPackageSize {
		return errors.New("release package exceeds 512 MiB safety limit")
	}
	out, err := os.OpenFile(file, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	total := expectedSize
	if total <= 0 {
		total = resp.ContentLength
	}
	reader := &progressReader{reader: resp.Body, total: total, job: job}
	written, err := io.Copy(out, io.LimitReader(reader, maxPackageSize+1))
	if err != nil {
		return err
	}
	if written > maxPackageSize {
		return errors.New("release package exceeds 512 MiB safety limit")
	}
	if expectedSize > 0 && written != expectedSize {
		return fmt.Errorf("release package size mismatch: got %d, want %d", written, expectedSize)
	}
	s.log(job, fmt.Sprintf("download complete: %.1f MiB", float64(written)/(1<<20)))
	return nil
}

type progressReader struct {
	reader io.Reader
	total  int64
	read   int64
	last   int
	job    *Job
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += int64(n)
	if r.total > 0 {
		percent := int(r.read * 100 / r.total)
		if percent >= r.last+10 || percent == 100 {
			r.last = percent - percent%10
			if percent >= 100 {
				r.last = 100
			}
			r.job.append(fmt.Sprintf("download progress: %d%%", min(percent, 100)))
		}
	}
	return n, err
}

func selectAsset(assets []githubAsset) Asset {
	arch := runtime.GOARCH
	suffix := "_" + arch + ".deb"
	for _, asset := range assets {
		if strings.HasPrefix(asset.Name, "kioskmate_") && strings.HasSuffix(asset.Name, suffix) {
			return Asset{Name: asset.Name, URL: asset.BrowserDownloadURL, Digest: asset.Digest, Size: asset.Size}
		}
	}
	return Asset{}
}

func verify(file, digest string) error {
	if digest == "" {
		return errors.New("release asset has no SHA-256 digest")
	}
	parts := strings.SplitN(digest, ":", 2)
	if len(parts) != 2 || parts[0] != "sha256" {
		return fmt.Errorf("unsupported digest: %s", digest)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != parts[1] {
		return errors.New("sha256 verification failed")
	}
	return nil
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

func runPrivileged(ctx context.Context, job *Job, privilege privilegeCommand, command string, args ...string) error {
	executable, commandArgs, input, err := buildPrivilegedCommand(privilege, command, args...)
	if err != nil {
		return err
	}
	job.append("running privileged command: " + command + " " + strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, executable, commandArgs...)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	writer := &jobWriter{job: job}
	cmd.Stdout = writer
	cmd.Stderr = writer
	err = cmd.Run()
	writer.flush()
	return err
}

func buildPrivilegedCommand(privilege privilegeCommand, command string, args ...string) (string, []string, string, error) {
	switch privilege.Mode {
	case "sudo":
		sudoArgs := []string{"-n"}
		input := ""
		if privilege.Password != "" {
			sudoArgs = []string{"-S", "-p", ""}
			input = privilege.Password + "\n"
		}
		return "sudo", append(sudoArgs, append([]string{command}, args...)...), input, nil
	case "su":
		parts := append([]string{command}, args...)
		quoted := make([]string, len(parts))
		for i, part := range parts {
			quoted[i] = "'" + strings.ReplaceAll(part, "'", "'\"'\"'") + "'"
		}
		return "su", []string{"-", "root", "-c", strings.Join(quoted, " ")}, privilege.Password + "\n", nil
	default:
		return "", nil, "", fmt.Errorf("unsupported privilege mode: %s", privilege.Mode)
	}
}

type jobWriter struct {
	mu      sync.Mutex
	job     *Job
	pending string
}

func (w *jobWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending += string(p)
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
	return len(p), nil
}

func (w *jobWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if line := strings.TrimSpace(w.pending); line != "" {
		w.job.append(line)
	}
	w.pending = ""
}

func (s *Service) log(job *Job, line string) {
	job.append(line)
}

func (s *Service) fail(job *Job, err error) {
	s.log(job, "error: "+err.Error())
	job.setError(err.Error())
	job.setExit(1)
	s.updateHistory(job, "failed")
}

func (s *Service) setJobStage(job *Job, stage string) {
	job.setStage(stage)
	s.updateHistory(job, "running")
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

func (j *Job) setStage(stage string) {
	j.mu.Lock()
	j.Stage = stage
	j.mu.Unlock()
}

func (j *Job) setTarget(target string) {
	j.mu.Lock()
	j.Target = target
	j.mu.Unlock()
}

func (j *Job) setError(message string) {
	j.mu.Lock()
	j.Error = message
	j.mu.Unlock()
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
		ID: j.ID, Name: j.Name, Stage: j.Stage, Target: j.Target, Error: j.Error,
		Started: j.Started, Finished: j.Finished, ExitCode: j.ExitCode,
		Output: append([]string(nil), j.Output...),
	}
}

func compareVersion(left, right string) int {
	a := versionParts(left)
	b := versionParts(right)
	for i := 0; i < len(a) || i < len(b); i++ {
		var av, bv int
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av != bv {
			return av - bv
		}
	}
	return 0
}

func versionParts(version string) []int {
	fields := strings.Split(strings.TrimPrefix(version, "v"), ".")
	out := make([]int, 0, len(fields))
	for _, field := range fields {
		var value int
		for _, r := range field {
			if r < '0' || r > '9' {
				break
			}
			value = value*10 + int(r-'0')
		}
		out = append(out, value)
	}
	return out
}
