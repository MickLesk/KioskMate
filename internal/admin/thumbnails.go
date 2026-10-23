package admin

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
)

// Page thumbnails: once a page has been on screen for a while, KioskMate
// keeps a small JPEG of it so the Admin UI can show what each page looks
// like. One capture per page at most every thumbnailMaxAge keeps the cost on
// a Raspberry Pi negligible.

const (
	thumbnailWidth   = 320
	thumbnailSettle  = 20 * time.Second
	thumbnailMaxAge  = 30 * time.Minute
	thumbnailTick    = 15 * time.Second
	thumbnailTimeout = 10 * time.Second
)

var thumbnailID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type thumbnailer interface {
	CaptureThumbnail(ctx context.Context, width int) ([]byte, error)
}

func (s *Server) thumbnailDir() string {
	return filepath.Join(config.ConfigDir(s.cfg.Path), "thumbnails")
}

func (s *Server) thumbnailPath(pageID string) string {
	return filepath.Join(s.thumbnailDir(), pageID+".jpg")
}

// runThumbnails captures the page on screen when it has settled and its
// thumbnail is missing or old.
func (s *Server) runThumbnails(ctx context.Context) {
	capture, ok := s.browser.(thumbnailer)
	if !ok {
		return
	}
	ticker := time.NewTicker(thumbnailTick)
	defer ticker.Stop()
	var shownID string
	var shownSince time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		status := s.browser.Status()
		pageID := s.cfg.Snapshot().Kiosk.EnabledPageID(status.Active)
		if !status.Running || !status.Ready || !status.DevTools || status.Blanked || status.AuthGuard.Tripped || pageID == "" {
			shownID = ""
			continue
		}
		if pageID != shownID {
			shownID, shownSince = pageID, time.Now()
			continue
		}
		if time.Since(shownSince) < thumbnailSettle {
			continue
		}
		if info, err := os.Stat(s.thumbnailPath(pageID)); err == nil && time.Since(info.ModTime()) < thumbnailMaxAge {
			continue
		}
		captureCtx, cancel := context.WithTimeout(ctx, thumbnailTimeout)
		data, err := capture.CaptureThumbnail(captureCtx, thumbnailWidth)
		cancel()
		// The page may have changed while capturing.
		if err != nil || s.cfg.Snapshot().Kiosk.EnabledPageID(s.browser.Status().Active) != pageID {
			continue
		}
		if err := s.saveThumbnail(pageID, data); err != nil {
			s.logger.Debug("page thumbnail not saved", "page", pageID, "error", err)
		}
	}
}

func (s *Server) saveThumbnail(pageID string, data []byte) error {
	if err := os.MkdirAll(s.thumbnailDir(), 0o700); err != nil {
		return err
	}
	tmp := s.thumbnailPath(pageID) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.thumbnailPath(pageID))
}

// pageThumbnails serves GET /api/pages/thumbnails (page_id -> capture time)
// and GET /api/pages/thumbnails/<page_id> (the JPEG).
func (s *Server) pageThumbnails(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/pages/thumbnails"), "/")
	if id == "" {
		out := map[string]time.Time{}
		for _, page := range s.cfg.Snapshot().Kiosk.Pages {
			if info, err := os.Stat(s.thumbnailPath(page.PageID)); err == nil && thumbnailID.MatchString(page.PageID) {
				out[page.PageID] = info.ModTime().UTC()
			}
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	if !thumbnailID.MatchString(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid page id"})
		return
	}
	data, err := os.ReadFile(s.thumbnailPath(id))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no thumbnail yet"})
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = w.Write(data)
}
