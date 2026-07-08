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
	cfg     *config.Config
	version string
	client  *http.Client

	mu   sync.Mutex
	jobs map[string]*Job
}

type Job struct {
	mu       sync.Mutex `json:"-"`
	ID       string     `json:"id"`
	Started  time.Time  `json:"started"`
	Finished *time.Time `json:"finished,omitempty"`
	ExitCode int        `json:"exit_code"`
	Output   []string   `json:"output"`
}

type ReleaseInfo struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	URL             string `json:"url"`
	Changelog       string `json:"changelog"`
	Asset           Asset  `json:"asset"`
	Supported       bool   `json:"supported"`
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

func New(cfg *config.Config, version string) *Service {
	return &Service{
		cfg:     cfg,
		version: version,
		client:  &http.Client{Timeout: 45 * time.Second},
		jobs:    map[string]*Job{},
	}
}

func (s *Service) Check(ctx context.Context) (ReleaseInfo, error) {
	release, err := s.latest(ctx)
	if err != nil {
		return ReleaseInfo{}, err
	}
	asset := selectAsset(release.Assets)
	latest := strings.TrimPrefix(release.TagName, "v")
	return ReleaseInfo{
		CurrentVersion:  s.version,
		LatestVersion:   latest,
		UpdateAvailable: compareVersion(latest, s.version) > 0,
		URL:             release.HTMLURL,
		Changelog:       release.Body,
		Asset:           asset,
		Supported:       runtime.GOOS == "linux" && asset.URL != "",
	}, nil
}

func (s *Service) Install(ctx context.Context) *Job {
	job := &Job{ID: fmt.Sprintf("%d", time.Now().UnixNano()), Started: time.Now(), ExitCode: -1}
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()
	go s.install(ctx, job)
	return job
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

func (s *Service) install(ctx context.Context, job *Job) {
	defer func() {
		job.finishDefault(1)
	}()
	s.log(job, "checking latest release")
	info, err := s.Check(ctx)
	if err != nil {
		s.fail(job, err)
		return
	}
	if !info.UpdateAvailable {
		s.log(job, "already up to date")
		job.setExit(0)
		return
	}
	if !info.Supported {
		s.fail(job, errors.New("no supported .deb asset for this platform"))
		return
	}
	file := filepath.Join(os.TempDir(), info.Asset.Name)
	if err := s.download(ctx, info.Asset.URL, file, job); err != nil {
		s.fail(job, err)
		return
	}
	if err := verify(file, info.Asset.Digest); err != nil {
		s.fail(job, err)
		return
	}
	if err := run(ctx, job, "sudo", "-n", "apt-get", "install", "-y", file); err != nil {
		s.fail(job, err)
		return
	}
	_ = run(ctx, job, "systemctl", "--user", "daemon-reload")
	_ = run(ctx, job, "systemctl", "--user", "restart", s.cfg.Update.Service)
	job.setExit(0)
}

func (s *Service) latest(ctx context.Context) (githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+s.cfg.Update.Repository+"/releases", nil)
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
		if !release.Draft && (!release.Prerelease || s.cfg.Update.Prerelease) {
			return release, nil
		}
	}
	return githubRelease{}, errors.New("no release found")
}

func (s *Service) download(ctx context.Context, url, file string, job *Job) error {
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
	out, err := os.OpenFile(file, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
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
		return nil
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

func run(ctx context.Context, job *Job, command string, args ...string) error {
	job.append(command + " " + strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		job.append(strings.TrimSpace(string(output)))
	}
	return err
}

func (s *Service) log(job *Job, line string) {
	job.append(line)
}

func (s *Service) fail(job *Job, err error) {
	s.log(job, "error: "+err.Error())
	job.setExit(1)
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
		Started:  j.Started,
		Finished: j.Finished,
		ExitCode: j.ExitCode,
		Output:   append([]string(nil), j.Output...),
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
