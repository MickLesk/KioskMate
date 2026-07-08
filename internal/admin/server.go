package admin

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MickLesk/KioskMate/v2/internal/actions"
	"github.com/MickLesk/KioskMate/v2/internal/config"
	"github.com/MickLesk/KioskMate/v2/internal/hardware"
	"github.com/MickLesk/KioskMate/v2/internal/mqttclient"
	"github.com/MickLesk/KioskMate/v2/internal/supervisor"
	"github.com/MickLesk/KioskMate/v2/internal/updater"
)

//go:embed web/index.html
var content embed.FS

const sessionCookieName = "kioskmate_session"

type Browser interface {
	Start(context.Context) error
	Stop(context.Context) error
	Restart(context.Context) error
	Next(context.Context) error
	Previous(context.Context) error
	ResetSession(context.Context) error
	Reload(context.Context) error
	SetActive(context.Context, int) error
	Status() supervisor.Status
}

type Server struct {
	cfg      *config.Config
	browser  Browser
	updater  *updater.Service
	actions  *actions.Service
	hardware *hardware.Service
	logger   *slog.Logger
	version  string
	mu       sync.Mutex
	sessions map[string]Session
	attempts map[string][]time.Time
}

type Session struct {
	ID        string    `json:"id"`
	Created   time.Time `json:"created"`
	LastSeen  time.Time `json:"last_seen"`
	Remote    string    `json:"remote"`
	UserAgent string    `json:"user_agent"`
}

func NewServer(cfg *config.Config, browser Browser, updates *updater.Service, actions *actions.Service, hw *hardware.Service, version string, logger *slog.Logger) *Server {
	return &Server{cfg: cfg, browser: browser, updater: updates, actions: actions, hardware: hw, version: version, logger: logger, sessions: map[string]Session{}, attempts: map[string][]time.Time{}}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/api/auth/status", s.authStatus)
	mux.HandleFunc("/api/auth/setup", s.authSetup)
	mux.HandleFunc("/api/auth/login", s.authLogin)
	mux.HandleFunc("/api/auth/logout", s.authLogout)
	mux.HandleFunc("/api/auth/sessions", s.auth(s.authSessions))
	mux.HandleFunc("/api/auth/logout-all", s.auth(s.authLogoutAll))
	mux.HandleFunc("/api/auth/password", s.auth(s.authPassword))
	mux.HandleFunc("/api/status", s.auth(s.status))
	mux.HandleFunc("/api/logs", s.auth(s.logs))
	mux.HandleFunc("/api/terminal/run", s.auth(s.terminalRun))
	mux.HandleFunc("/api/ssh-key", s.auth(s.sshKey))
	mux.HandleFunc("/api/config", s.auth(s.config))
	mux.HandleFunc("/api/config/export", s.auth(s.configExport))
	mux.HandleFunc("/api/config/import", s.auth(s.configImport))
	mux.HandleFunc("/api/config/backups", s.auth(s.configBackups))
	mux.HandleFunc("/api/config/restore", s.auth(s.configRestore))
	mux.HandleFunc("/api/mqtt/test", s.auth(s.mqttTest))
	mux.HandleFunc("/api/browser/start", s.auth(s.browserAction("start")))
	mux.HandleFunc("/api/browser/stop", s.auth(s.browserAction("stop")))
	mux.HandleFunc("/api/browser/restart", s.auth(s.browserAction("restart")))
	mux.HandleFunc("/api/browser/reload", s.auth(s.browserAction("reload")))
	mux.HandleFunc("/api/browser/next", s.auth(s.browserAction("next")))
	mux.HandleFunc("/api/browser/previous", s.auth(s.browserAction("previous")))
	mux.HandleFunc("/api/browser/reset-session", s.auth(s.browserAction("reset-session")))
	mux.HandleFunc("/api/browser/page", s.auth(s.browserPage))
	mux.HandleFunc("/api/browser/check-page", s.auth(s.browserCheckPage))
	mux.HandleFunc("/api/update", s.auth(s.update))
	mux.HandleFunc("/api/update/install", s.auth(s.updateInstall))
	mux.HandleFunc("/api/update/jobs/", s.auth(s.updateJob))
	mux.HandleFunc("/api/system/", s.auth(s.systemAction))
	mux.HandleFunc("/api/privilege", s.auth(s.privilege))
	mux.HandleFunc("/api/hardware", s.auth(s.hardwareStatus))
	mux.HandleFunc("/api/hardware/", s.auth(s.hardwareAction))
	mux.HandleFunc("/api/jobs/", s.auth(s.job))

	server := &http.Server{
		Addr:              s.cfg.Admin.Addr(),
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}
	errc := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		errc <- server.Shutdown(shutdownCtx)
	}()
	go func() {
		errc <- server.Serve(listener)
	}()
	err = <-errc
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) systemAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Mode     string `json:"mode"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Remember && body.Password != "" {
		if err := s.actions.RememberPrivilege(body.Mode, body.Password); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/system/")
	job, err := s.actions.StartPrivileged(context.Background(), name, body.Mode, body.Password)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) privilege(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.actions.PrivilegeStatus())
	case http.MethodDelete, http.MethodPost:
		s.actions.ClearPrivilege()
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) hardwareStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.hardware.Status(ctx))
}

func (s *Server) hardwareAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Value any `json:"value"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	name := strings.TrimPrefix(r.URL.Path, "/api/hardware/")
	var err error
	switch name {
	case "display":
		err = s.hardware.SetDisplay(ctx, fmt.Sprint(body.Value))
	case "brightness":
		err = s.hardware.SetBrightness(ctx, intValue(body.Value))
	case "volume":
		err = s.hardware.SetAudioVolume(ctx, intValue(body.Value), false)
	case "microphone":
		err = s.hardware.SetAudioVolume(ctx, intValue(body.Value), true)
	case "keyboard":
		err = s.hardware.SetKeyboard(ctx, fmt.Sprint(body.Value))
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown hardware action"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "action": name})
}

func (s *Server) job(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	job, ok := s.actions.Job(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := content.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": s.authenticated(r),
		"setupRequired": s.cfg.Admin.PasswordHash == "",
		"tokenRequired": s.cfg.Admin.PasswordHash == "",
		"configPath":    s.cfg.Path,
		"version":       s.version,
	})
}

func (s *Server) authSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.Admin.PasswordHash != "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "setup already completed"})
		return
	}
	if !s.allowAttempt(r) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts"})
		return
	}
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Token), []byte(s.cfg.Admin.Token)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid setup token"})
		return
	}
	if err := validatePassword(body.Password); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	hash, err := HashPassword(body.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.cfg.Admin.PasswordHash = hash
	if err := config.Save(s.cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.createSession(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.allowAttempt(r) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts"})
		return
	}
	var body struct {
		Password string `json:"password"`
		Token    string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ok := false
	if s.cfg.Admin.PasswordHash != "" {
		ok = VerifyPassword(body.Password, s.cfg.Admin.PasswordHash)
	} else if body.Token != "" {
		ok = subtle.ConstantTimeCompare([]byte(body.Token), []byte(s.cfg.Admin.Token)) == 1
	}
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	s.createSession(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		s.mu.Lock()
		delete(s.sessions, cookie.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) authSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	out := make([]Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		out = append(out, session)
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) authLogoutAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	s.sessions = map[string]Session{}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) authPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Current string `json:"current"`
		Next    string `json:"next"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if s.cfg.Admin.PasswordHash != "" && !VerifyPassword(body.Current, s.cfg.Admin.PasswordHash) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid current password"})
		return
	}
	if err := validatePassword(body.Next); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	hash, err := HashPassword(body.Next)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.cfg.Admin.PasswordHash = hash
	if err := config.Save(s.cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, map[string]any{
		"browser":  s.browser.Status(),
		"hardware": s.hardware.Status(ctx),
		"config": map[string]any{
			"path":        s.cfg.Path,
			"profile":     s.cfg.Performance.Profile,
			"gpu_mode":    s.cfg.Performance.GPUMode,
			"theme":       s.cfg.Kiosk.Theme,
			"zoom":        s.cfg.Kiosk.ZoomPercent,
			"widget":      s.cfg.Kiosk.Widget,
			"watchdog":    s.cfg.Watchdog.Enabled,
			"admin_addr":  s.cfg.Admin.Addr(),
			"kiosk_urls":  s.cfg.Kiosk.URLs,
			"kiosk_pages": s.cfg.Kiosk.Pages,
			"scheduler":   s.cfg.Kiosk.Scheduler,
			"rotation":    s.cfg.Kiosk.Rotation,
			"time_rules":  s.cfg.Kiosk.TimeRules,
			"browser_cmd": s.cfg.Kiosk.BrowserCommand,
			"mqtt":        s.cfg.MQTT.Enabled,
		},
	})
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	lines := 300
	if raw := r.URL.Query().Get("lines"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 && value <= 2000 {
			lines = value
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if _, err := exec.LookPath("journalctl"); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"lines": s.fallbackLogs(ctx, "journalctl not found"), "warning": "journalctl not found"})
		return
	}
	out, err := exec.CommandContext(ctx, "journalctl", "--user", "-u", s.cfg.Update.Service, "-n", strconv.Itoa(lines), "--no-pager").CombinedOutput()
	if err != nil {
		fallback := s.fallbackLogs(ctx, strings.TrimSpace(string(out))+" "+err.Error())
		writeJSON(w, http.StatusOK, map[string]any{"lines": fallback, "warning": "journalctl failed"})
		return
	}
	text := strings.TrimSpace(string(out))
	if text == "" || strings.Contains(text, "No journal files were found") || strings.Contains(text, "-- No entries --") {
		writeJSON(w, http.StatusOK, map[string]any{"lines": s.fallbackLogs(ctx, text), "warning": "no journal entries"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": strings.Split(text, "\n")})
}

func (s *Server) fallbackLogs(ctx context.Context, reason string) []string {
	var lines []string
	if reason != "" {
		lines = append(lines, reason)
	}
	if out, err := exec.CommandContext(ctx, "systemctl", "--user", "status", s.cfg.Update.Service, "--no-pager", "--full").CombinedOutput(); len(out) > 0 || err != nil {
		lines = append(lines, "$ systemctl --user status "+s.cfg.Update.Service+" --no-pager --full")
		lines = append(lines, strings.Split(strings.TrimSpace(string(out)), "\n")...)
		if err != nil {
			lines = append(lines, "error: "+err.Error())
		}
	}
	lines = append(lines,
		"Config: "+s.cfg.Path,
		"Config backup: "+s.cfg.Path+".bak",
		"Tip: run `kioskmate --admin-info` on the kiosk for full recovery paths.",
	)
	return lines
}

func (s *Server) terminalRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	body.Command = strings.TrimSpace(body.Command)
	if body.Command == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "command required"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	shell := "sh"
	args := []string{"-lc", body.Command}
	if _, err := exec.LookPath("bash"); err == nil {
		shell = "bash"
	}
	out, err := exec.CommandContext(ctx, shell, args...).CombinedOutput()
	code := 0
	if err != nil {
		code = 1
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"exit_code": code,
		"output":    string(out),
		"error":     errorString(err),
	})
}

func (s *Server) sshKey(w http.ResponseWriter, r *http.Request) {
	keyPath, err := sshKeyPath()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	switch r.Method {
	case http.MethodGet:
		pub, _ := os.ReadFile(keyPath + ".pub")
		writeJSON(w, http.StatusOK, map[string]any{
			"exists":     fileExists(keyPath),
			"path":       keyPath,
			"public_key": strings.TrimSpace(string(pub)),
		})
	case http.MethodPost:
		if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !fileExists(keyPath) {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, "ssh-keygen", "-t", "ed25519", "-N", "", "-f", keyPath, "-C", "kioskmate").CombinedOutput()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": strings.TrimSpace(string(out)) + " " + err.Error()})
				return
			}
			_ = os.Chmod(keyPath, 0o600)
			_ = os.Chmod(keyPath+".pub", 0o644)
		}
		pub, _ := os.ReadFile(keyPath + ".pub")
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "path": keyPath, "public_key": strings.TrimSpace(string(pub))})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.cfg)
	case http.MethodPost:
		data, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		next, err := s.decodeConfig(data)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := config.Save(next); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		*s.cfg = *next
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) configExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="kioskmate-config.json"`)
	_, _ = w.Write(append(data, '\n'))
}

func (s *Server) configImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	next, err := s.decodeConfig(data)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := config.Save(next); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	*s.cfg = *next
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) configBackups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":    s.cfg.Path,
		"backups": s.backupFiles(),
	})
}

func (s *Server) configRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	allowed := false
	for _, file := range s.backupFiles() {
		if file.Path == body.Path && (file.Kind == "v2" || file.Kind == "previous") {
			allowed = true
			break
		}
	}
	if !allowed {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "backup is not restorable through this endpoint"})
		return
	}
	data, err := os.ReadFile(body.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	next, err := s.decodeConfig(data)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := config.Save(next); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	*s.cfg = *next
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

type backupFile struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Kind     string    `json:"kind"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

func (s *Server) decodeConfig(data []byte) (*config.Config, error) {
	var next config.Config
	if err := json.Unmarshal(data, &next); err != nil {
		return nil, err
	}
	next.Path = s.cfg.Path
	if next.Admin.Token == "" {
		next.Admin.Token = s.cfg.Admin.Token
	}
	return &next, nil
}

func (s *Server) backupFiles() []backupFile {
	paths := []struct {
		path string
		kind string
	}{
		{s.cfg.Path + ".bak", "v2"},
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		previousV2 := filepath.Join(home, ".config", config.LegacyAppLower()+"-v2", "config.json")
		previousBrand := filepath.Join(home, ".config", config.PreviousBrandLower(), "config.json")
		paths = append(paths,
			struct {
				path string
				kind string
			}{previousBrand, "previous"},
			struct {
				path string
				kind string
			}{previousBrand + ".bak", "previous"},
			struct {
				path string
				kind string
			}{previousV2, "previous"},
			struct {
				path string
				kind string
			}{previousV2 + ".bak", "previous"},
			struct {
				path string
				kind string
			}{filepath.Join(home, ".config", config.LegacyAppLower(), "Arguments.json"), "legacy"},
			struct {
				path string
				kind string
			}{filepath.Join(home, ".config", config.LegacyAppLower(), "Arguments.json.bak"), "legacy"},
			struct {
				path string
				kind string
			}{filepath.Join(home, ".config", config.LegacyAppTitle(), "Arguments.json"), "legacy"},
			struct {
				path string
				kind string
			}{filepath.Join(home, ".config", config.LegacyAppTitle(), "Arguments.json.bak"), "legacy"},
		)
	}
	var files []backupFile
	seen := map[string]bool{}
	for _, item := range paths {
		if seen[item.path] {
			continue
		}
		seen[item.path] = true
		info, err := os.Stat(item.path)
		if err != nil || info.IsDir() {
			continue
		}
		files = append(files, backupFile{
			Path:     item.path,
			Name:     filepath.Base(item.path),
			Kind:     item.kind,
			Size:     info.Size(),
			Modified: info.ModTime(),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Modified.After(files[j].Modified)
	})
	return files
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	info, err := s.updater.Check(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) updateInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	job := s.updater.Install(context.Background())
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) updateJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/update/jobs/")
	job, ok := s.updater.Job(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) browserAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		var err error
		switch action {
		case "start":
			err = s.browser.Start(ctx)
		case "stop":
			err = s.browser.Stop(ctx)
		case "restart":
			err = s.browser.Restart(ctx)
		case "reload":
			err = s.browser.Reload(ctx)
		case "next":
			err = s.browser.Next(ctx)
		case "previous":
			err = s.browser.Previous(ctx)
		case "reset-session":
			err = s.browser.ResetSession(ctx)
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "action": action})
	}
}

func (s *Server) mqttTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		URL      string `json:"url"`
		Version  string `json:"version"`
		Username string `json:"username"`
		Password string `json:"password"`
		Node     string `json:"node"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if body.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mqtt url required"})
		return
	}
	if body.Version == "" {
		body.Version = "3.1.1"
	}
	if body.Version != "3.1.1" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "internal MQTT client currently supports MQTT 3.1.1; MQTT 5.0 is planned"})
		return
	}
	client := &mqttclient.Client{URL: body.URL, ClientID: firstNonEmpty(body.Node, "kioskmate") + "_test", Username: body.Username, Password: body.Password}
	start := time.Now()
	err := client.Ping()
	_ = client.Close()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "latency_ms": time.Since(start).Milliseconds(), "version": body.Version})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "latency_ms": time.Since(start).Milliseconds(), "version": body.Version})
}

func (s *Server) browserPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Index int `json:"index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.browser.SetActive(ctx, body.Index); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) browserCheckPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Index int    `json:"index"`
		URL   string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	target := strings.TrimSpace(body.URL)
	if target == "" {
		urls := s.cfg.Kiosk.PageURLs()
		if body.Index < 0 || body.Index >= len(urls) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "page index out of range"})
			return
		}
		target = urls[body.Index]
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only http and https URLs can be checked"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	req.Header.Set("User-Agent", "KioskMate/"+s.version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "url": target, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, 4096)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         resp.StatusCode >= 200 && resp.StatusCode < 400,
		"url":        target,
		"final_url":  resp.Request.URL.String(),
		"status":     resp.Status,
		"statusCode": resp.StatusCode,
	})
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authenticated(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) authenticated(r *http.Request) bool {
	if s.validToken(r) {
		return true
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	s.mu.Lock()
	session, ok := s.sessions[cookie.Value]
	if ok {
		session.LastSeen = time.Now()
		s.sessions[cookie.Value] = session
	}
	s.mu.Unlock()
	return ok
}

func (s *Server) validToken(r *http.Request) bool {
	token := r.Header.Get("X-Go-Kiosk-Token")
	if token == "" {
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if token == "" || s.cfg.Admin.Token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.Admin.Token)) == 1
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	id := randomID(32)
	session := Session{
		ID:        id,
		Created:   time.Now(),
		LastSeen:  time.Now(),
		Remote:    remoteIP(r),
		UserAgent: r.UserAgent(),
	}
	s.mu.Lock()
	s.sessions[id] = session
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    id,
		Path:     "/",
		MaxAge:   int((7 * 24 * time.Hour).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) allowAttempt(r *http.Request) bool {
	key := remoteIP(r)
	cutoff := time.Now().Add(-5 * time.Minute)
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept []time.Time
	for _, ts := range s.attempts[key] {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	kept = append(kept, time.Now())
	s.attempts[key] = kept
	return len(kept) <= 8
}

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	iterations := 120000
	hash := passwordDigest([]byte(password), salt, iterations)
	return fmt.Sprintf("sha256iter$%d$%s$%s", iterations, base64.RawURLEncoding.EncodeToString(salt), base64.RawURLEncoding.EncodeToString(hash)), nil
}

func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "sha256iter" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 10000 {
		return false
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got := passwordDigest([]byte(password), salt, iterations)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func passwordDigest(password, salt []byte, iterations int) []byte {
	mac := hmac.New(sha256.New, password)
	mac.Write(salt)
	sum := mac.Sum(nil)
	for i := 1; i < iterations; i++ {
		mac = hmac.New(sha256.New, password)
		mac.Write(sum)
		sum = mac.Sum(nil)
	}
	return sum
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("password must have at least 8 characters")
	}
	return nil
}

func randomID(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func intValue(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		i, _ := strconv.Atoi(v)
		return i
	default:
		return 0
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sshKeyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "kioskmate", "Keys", "id_ed25519_kioskmate"), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
