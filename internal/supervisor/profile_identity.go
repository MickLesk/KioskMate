package supervisor

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MickLesk/KioskMate/internal/config"
)

func enabledPage(cfg *config.Config, index int) (config.KioskPage, bool) {
	for _, page := range cfg.Kiosk.Pages {
		if page.Disabled || strings.TrimSpace(page.URL) == "" {
			continue
		}
		if index == 0 {
			return page, true
		}
		index--
	}
	return config.KioskPage{}, false
}

func profileIdentity(cfg *config.Config, index int) string {
	page, ok := enabledPage(cfg, index)
	if !ok || page.PageID == "" {
		return pathSegment(cfg.Kiosk.PageName(index), index)
	}
	sum := sha256.Sum256([]byte(page.PageID))
	return "id-" + hex.EncodeToString(sum[:16])
}

func migratePageProfiles(cfg *config.Config) error {
	if !cfg.Kiosk.IsolateSessions || cfg.Kiosk.UserDataDir == "" {
		return nil
	}
	// Move all known legacy profiles before a later page rename can lose their mapping.
	for index := 0; index < cfg.Kiosk.PageCount(); index++ {
		target := browserUserDataDir(cfg, index)
		legacy := filepath.Join(cfg.Kiosk.UserDataDir, "pages", pathSegment(cfg.Kiosk.PageName(index), index))
		if target == legacy {
			continue
		}
		if _, err := os.Lstat(target); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		info, err := os.Lstat(legacy)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("legacy browser profile is not a directory")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("create browser profile directory: %w", err)
		}
		if err := os.Rename(legacy, target); err != nil {
			return fmt.Errorf("migrate browser profile identity: %w", err)
		}
	}
	return nil
}
