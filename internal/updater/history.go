package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type HistoryEntry struct {
	ID          string     `json:"id"`
	Action      string     `json:"action"`
	FromVersion string     `json:"from_version"`
	Target      string     `json:"target_version"`
	Status      string     `json:"status"`
	Stage       string     `json:"stage"`
	Started     time.Time  `json:"started"`
	Finished    *time.Time `json:"finished,omitempty"`
	Error       string     `json:"error,omitempty"`
	BackupPath  string     `json:"backup_path,omitempty"`
}

type HistoryInfo struct {
	Entries           []HistoryEntry `json:"entries"`
	RollbackAvailable bool           `json:"rollback_available"`
	RollbackTarget    string         `json:"rollback_target,omitempty"`
}

func (s *Service) loadHistory() *Service {
	data, err := os.ReadFile(s.historyPath())
	if err == nil {
		_ = json.Unmarshal(data, &s.history)
	}
	if len(s.history) > 50 {
		s.history = s.history[len(s.history)-50:]
	}
	if len(s.history) == 0 {
		if inferred, ok := inferPackageHistory(s.version); ok {
			s.history = append(s.history, inferred)
			s.saveHistoryLocked()
		}
	}
	s.finalizeHistoryAfterRestart()
	return s
}

func (s *Service) History() HistoryInfo {
	s.mu.Lock()
	entries := append([]HistoryEntry(nil), s.history...)
	s.mu.Unlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Started.After(entries[j].Started) })
	target := rollbackTarget(entries, s.version)
	return HistoryInfo{Entries: entries, RollbackAvailable: target != "", RollbackTarget: target}
}

func rollbackTarget(entries []HistoryEntry, current string) string {
	for _, entry := range entries {
		if entry.Status == "installed" && entry.Target == current && entry.FromVersion != "" && entry.FromVersion != current {
			return entry.FromVersion
		}
	}
	return ""
}

func (s *Service) beginHistory(job *Job, action, target string) {
	entry := HistoryEntry{ID: job.ID, Action: action, FromVersion: s.version, Target: target, Status: "running", Stage: job.Stage, Started: job.Started}
	s.mu.Lock()
	s.history = append(s.history, entry)
	if len(s.history) > 50 {
		s.history = s.history[len(s.history)-50:]
	}
	s.saveHistoryLocked()
	s.mu.Unlock()
}

func (s *Service) updateHistory(job *Job, status string) {
	snapshot := job.snapshot()
	s.mu.Lock()
	for index := range s.history {
		if s.history[index].ID != snapshot.ID {
			continue
		}
		s.history[index].Target = snapshot.Target
		s.history[index].Stage = snapshot.Stage
		s.history[index].Status = status
		s.history[index].Error = snapshot.Error
		if status == "failed" || status == "installed" {
			now := time.Now()
			s.history[index].Finished = &now
		}
		break
	}
	s.saveHistoryLocked()
	s.mu.Unlock()
}

func (s *Service) setHistoryBackup(jobID, path string) {
	s.mu.Lock()
	for index := range s.history {
		if s.history[index].ID == jobID {
			s.history[index].BackupPath = path
			break
		}
	}
	s.saveHistoryLocked()
	s.mu.Unlock()
}

func (s *Service) finalizeHistoryAfterRestart() {
	changed := false
	now := time.Now()
	for index := range s.history {
		entry := &s.history[index]
		if entry.Status != "restarting" {
			continue
		}
		entry.Finished = &now
		if entry.Target == s.version {
			entry.Status = "installed"
			entry.Stage = "completed"
		} else {
			entry.Status = "failed"
			entry.Stage = "restart_failed"
			entry.Error = fmt.Sprintf("service restarted with version %s instead of %s", s.version, entry.Target)
		}
		changed = true
	}
	if changed {
		s.saveHistoryLocked()
	}
}

func (s *Service) backupConfig(job *Job) (string, error) {
	data, err := os.ReadFile(s.cfg.Path)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(filepath.Dir(s.cfg.Path), "update-backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := fmt.Sprintf("config-%s-v%s.json", time.Now().Format("20060102-150405"), safeVersion(s.version))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	s.setHistoryBackup(job.ID, path)
	return path, nil
}

func safeVersion(version string) string {
	clean := ""
	for _, char := range version {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '.' || char == '-' {
			clean += string(char)
		}
	}
	if clean == "" {
		return "unknown"
	}
	return clean
}

func parseDPKGHistory(data []byte, current string) (HistoryEntry, bool) {
	lines := strings.Split(string(data), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		fields := strings.Fields(lines[index])
		if len(fields) < 6 || (fields[2] != "upgrade" && fields[2] != "downgrade") || !strings.HasPrefix(fields[3], "kioskmate:") {
			continue
		}
		fromVersion := strings.TrimPrefix(fields[4], "v")
		targetVersion := strings.TrimPrefix(fields[5], "v")
		if targetVersion != strings.TrimPrefix(current, "v") || fromVersion == targetVersion {
			continue
		}
		started, _ := time.ParseInLocation("2006-01-02 15:04:05", fields[0]+" "+fields[1], time.Local)
		if started.IsZero() {
			started = time.Now()
		}
		finished := started
		return HistoryEntry{
			ID: "dpkg-" + started.Format("20060102-150405"), Action: "install", FromVersion: fromVersion,
			Target: targetVersion, Status: "installed", Stage: "completed", Started: started, Finished: &finished,
		}, true
	}
	return HistoryEntry{}, false
}

func (s *Service) historyPath() string {
	return filepath.Join(filepath.Dir(s.cfg.Path), "update-history.json")
}

func (s *Service) saveHistoryLocked() {
	data, err := json.MarshalIndent(s.history, "", "  ")
	if err != nil {
		return
	}
	path := s.historyPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return
	}
	_ = os.Remove(path)
	_ = os.Rename(temporary, path)
}
