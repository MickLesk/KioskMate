package supervisor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func hardenChromiumProfile(profile string) error {
	if profile == "" {
		return errors.New("browser profile path is empty")
	}
	defaultDir := filepath.Join(profile, "Default")
	if err := os.MkdirAll(defaultDir, 0o700); err != nil {
		return err
	}
	if err := quarantineStoredCredentials(profile); err != nil {
		return err
	}
	path := filepath.Join(defaultDir, "Preferences")
	preferences := map[string]any{}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &preferences); err != nil {
			backup := fmt.Sprintf("%s.kioskmate-invalid-%s", path, time.Now().Format("20060102-150405"))
			if renameErr := os.Rename(path, backup); renameErr != nil {
				return fmt.Errorf("back up invalid Chromium preferences: %w", renameErr)
			}
			preferences = map[string]any{}
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	preferences["credentials_enable_service"] = false
	preferences["password_manager_leak_detection"] = false
	objectSetting(preferences, "profile")["password_manager_enabled"] = false
	objectSetting(preferences, "profile")["exit_type"] = "Normal"
	objectSetting(preferences, "profile")["exited_cleanly"] = true
	objectSetting(preferences, "autofill")["profile_enabled"] = false
	objectSetting(preferences, "autofill")["credit_card_enabled"] = false
	objectSetting(preferences, "session")["restore_on_startup"] = 4
	objectSetting(preferences, "signin")["allowed"] = false

	data, err := json.Marshal(preferences)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(defaultDir, ".kioskmate-preferences-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return replaceProfileFile(tmpPath, path)
}

func quarantineStoredCredentials(profile string) error {
	marker := filepath.Join(profile, ".kioskmate-auth-safety-v1")
	if _, err := os.Stat(marker); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	backupRoot := filepath.Join(profile, "SessionBackups", "auth-safety-"+time.Now().Format("20060102-150405"))
	moved := false
	for _, name := range []string{"Login Data", "Login Data-journal", "Login Data For Account", "Login Data For Account-journal"} {
		source := filepath.Join(profile, "Default", name)
		if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := os.MkdirAll(backupRoot, 0o700); err != nil {
			return err
		}
		if err := os.Rename(source, filepath.Join(backupRoot, name)); err != nil {
			return fmt.Errorf("quarantine stored Chromium credentials: %w", err)
		}
		moved = true
	}
	if !moved {
		_ = os.RemoveAll(backupRoot)
	}
	return os.WriteFile(marker, []byte("KioskMate disabled Chromium credential replay.\n"), 0o600)
}

func replaceProfileFile(source string, target string) error {
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		return os.Rename(source, target)
	} else if err != nil {
		return err
	}
	backup := target + ".kioskmate-previous"
	_ = os.Remove(backup)
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(source, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	_ = os.Remove(backup)
	return nil
}

func objectSetting(parent map[string]any, key string) map[string]any {
	if current, ok := parent[key].(map[string]any); ok {
		return current
	}
	current := map[string]any{}
	parent[key] = current
	return current
}
