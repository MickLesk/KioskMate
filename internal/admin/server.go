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

	"github.com/MickLesk/KioskMate/internal/actions"
	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/hardware"
	"github.com/MickLesk/KioskMate/internal/mqttclient"
	"github.com/MickLesk/KioskMate/internal/supervisor"
	"github.com/MickLesk/KioskMate/internal/updater"
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

type MQTTDiscoveryPublisher interface {
	PublishNow() error
}

type Server struct {
	cfg      *config.Config
	browser  Browser
	mqtt     MQTTDiscoveryPublisher
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

type mqttTestRequest struct {
	URL                string `json:"url"`
	Version            string `json:"version"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	Discovery          string `json:"discovery"`
	BaseTopic          string `json:"base_topic"`
	Node               string `json:"node"`
	ClientID           string `json:"client_id"`
	KeepAliveSeconds   int    `json:"keepalive_seconds"`
	ForceDisableRetain bool   `json:"force_disable_retain"`
}

func NewServer(cfg *config.Config, browser Browser, mqtt MQTTDiscoveryPublisher, updates *updater.Service, actions *actions.Service, hw *hardware.Service, version string, logger *slog.Logger) *Server {
	return &Server{cfg: cfg, browser: browser, mqtt: mqtt, updater: updates, actions: actions, hardware: hw, version: version, logger: logger, sessions: map[string]Session{}, attempts: map[string][]time.Time{}}
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
	mux.HandleFunc("/api/mqtt/test/live", s.auth(s.mqttTestLive))
	mux.HandleFunc("/api/mqtt/discovery", s.auth(s.mqttDiscovery))
	mux.HandleFunc("/api/browser/start", s.auth(s.browserAction("start")))
	mux.HandleFunc("/api/browser/stop", s.auth(s.browserAction("stop")))
	mux.HandleFunc("/api/browser/restart", s.auth(s.browserAction("restart")))
	mux.HandleFunc("/api/browser/reload", s.auth(s.browserAction("reload")))
	mux.HandleFunc("/api/browser/next", s.auth(s.browserAction("next")))
	mux.HandleFunc("/api/browser/previous", s.auth(s.browserAction("previous")))
	mux.HandleFunc("/api/browser/reset-session", s.auth(s.browserAction("reset-session")))
	mux.HandleFunc("/api/browser/page", s.auth(s.browserPage))
	mux.HandleFunc("/api/browser/check-page", s.auth(s.browserCheckPage))
	mux.HandleFunc("/api/browser/diagnostics", s.auth(s.browserDiagnostics))
	mux.HandleFunc("/api/browser/safe-mode", s.auth(s.browserSafeMode))
	mux.HandleFunc("/api/update", s.auth(s.update))
	mux.HandleFunc("/api/update/install", s.auth(s.updateInstall))
	mux.HandleFunc("/api/update/jobs/", s.auth(s.updateJob))
	mux.HandleFunc("/api/repair", s.auth(s.repair))
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
	logFile := config.LogFilePath(s.cfg.Path)
	if fileLines := readTail(logFile, 220); len(fileLines) > 0 {
		lines = append(lines, "$ tail "+logFile)
		lines = append(lines, fileLines...)
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

func readTail(path string, limit int) []string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil
	}
	parts := strings.Split(strings.TrimSpace(string(data)), "\n")
	if limit > 0 && len(parts) > limit {
		parts = parts[len(parts)-limit:]
	}
	return parts
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

func (s *Server) repair(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := *s.cfg
		data, _ := json.Marshal(s.cfg)
		_ = json.Unmarshal(data, &cfg)
		writeJSON(w, http.StatusOK, config.Repair(&cfg))
	case http.MethodPost:
		report := config.Repair(s.cfg)
		if err := config.Save(s.cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, report)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
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
		if file.Path == body.Path && file.Kind == "config" {
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
		{s.cfg.Path + ".bak", "config"},
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

func (s *Server) browserDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	status := s.browser.Status()
	command := status.Command
	resolved := ""
	commandExists := false
	commandError := ""
	if command == "" {
		commandError = "browser command is empty"
	} else if path, err := exec.LookPath(command); err == nil {
		resolved = path
		commandExists = true
	} else if filepath.IsAbs(command) {
		if info, statErr := os.Stat(command); statErr == nil && !info.IsDir() {
			resolved = command
			commandExists = true
		} else {
			commandError = err.Error()
		}
	} else {
		commandError = err.Error()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"running":          status.Running,
		"pid":              status.PID,
		"command":          command,
		"resolved_command": resolved,
		"command_exists":   commandExists,
		"command_error":    commandError,
		"args":             status.Args,
		"active_page":      status.Active,
		"page_name":        status.PageName,
		"url":              status.URL,
		"page_count":       s.cfg.Kiosk.PageCount(),
		"profile":          s.cfg.Performance.Profile,
		"gpu_mode":         s.cfg.Performance.GPUMode,
		"reduce_motion":    s.cfg.Performance.ReduceMotion,
		"watchdog":         s.cfg.Watchdog.Enabled,
		"last_error":       status.LastError,
		"last_exit":        status.LastExit,
	})
}

func (s *Server) browserSafeMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Restart bool `json:"restart"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	config.ApplyRaspberrySafeMode(s.cfg)
	if err := config.Save(s.cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if body.Restart {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		if err := s.browser.Restart(ctx); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "config": s.cfg})
}

func (s *Server) mqttTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := parseMQTTTestRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, runMQTTTest(body, nil))
}

func (s *Server) mqttTestLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := parseMQTTTestRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	encoder := json.NewEncoder(w)
	emit := func(event map[string]any) {
		_ = encoder.Encode(event)
		flusher.Flush()
	}
	result := runMQTTTest(body, emit)
	status := "error"
	if ok, _ := result["ok"].(bool); ok {
		status = "ok"
	}
	emit(map[string]any{"step": "result", "status": status, "message": "MQTT test finished", "result": result})
}

func (s *Server) mqttDiscovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.mqtt == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "mqtt service unavailable"})
		return
	}
	if err := s.mqtt.PublishNow(); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func parseMQTTTestRequest(r *http.Request) (mqttTestRequest, error) {
	var body mqttTestRequest
	err := json.NewDecoder(r.Body).Decode(&body)
	return body, err
}

func runMQTTTest(body mqttTestRequest, emit func(map[string]any)) map[string]any {
	start := time.Now()
	published := []string{}
	event := func(step string, status string, message string, fields map[string]any) {
		if emit == nil {
			return
		}
		item := map[string]any{
			"step":       step,
			"status":     status,
			"message":    message,
			"elapsed_ms": time.Since(start).Milliseconds(),
			"time":       time.Now().Format(time.RFC3339),
		}
		for key, value := range fields {
			item[key] = value
		}
		emit(item)
	}
	fail := func(step string, err error) map[string]any {
		event(step, "error", err.Error(), map[string]any{"published_topics": published})
		return map[string]any{"ok": false, "error": err.Error(), "latency_ms": time.Since(start).Milliseconds(), "version": body.Version, "published_topics": published}
	}
	event("validate", "running", "Validating MQTT settings", nil)
	if body.URL == "" {
		return fail("validate", errors.New("mqtt url required"))
	}
	if body.Version == "" {
		body.Version = "3.1.1"
	}
	if body.Version != "3.1.1" && body.Version != "5.0" {
		return fail("validate", errors.New("supported MQTT versions are 3.1.1 and 5.0"))
	}
	node := strings.Trim(firstNonEmpty(body.Node, "kioskmate"), "/")
	discovery := strings.Trim(firstNonEmpty(body.Discovery, "homeassistant"), "/")
	baseTopic := strings.Trim(firstNonEmpty(body.BaseTopic, "kioskmate"), "/")
	root := baseTopic + "/" + node
	clientID := firstNonEmpty(strings.TrimSpace(body.ClientID), node) + "_test"
	keepAlive := time.Duration(body.KeepAliveSeconds) * time.Second
	if keepAlive <= 0 {
		keepAlive = 60 * time.Second
	}
	retained := !body.ForceDisableRetain
	client := &mqttclient.Client{URL: body.URL, ClientID: clientID, Username: body.Username, Password: body.Password, Version: body.Version, Timeout: 5 * time.Second, KeepAlive: keepAlive}
	event("validate", "ok", "Settings accepted", map[string]any{"broker": body.URL, "version": body.Version, "base_topic": baseTopic, "node": node, "client_id": clientID, "discovery_prefix": discovery, "root": root, "keepalive_seconds": int(keepAlive / time.Second), "retain": retained})
	event("connect", "running", "Opening MQTT connection and waiting for CONNACK", map[string]any{"broker": body.URL, "client_id": clientID})
	if err := client.Connect(); err != nil {
		_ = client.Close()
		return fail("connect", err)
	}
	event("connect", "ok", "Broker accepted the connection", nil)
	event("ping", "running", "Sending MQTT ping", nil)
	if err := client.Ping(); err != nil {
		_ = client.Close()
		return fail("ping", err)
	}
	event("ping", "ok", "Ping response received", nil)
	availabilityTopic := root + "/availability"
	event("publish_availability", "running", "Publishing retained availability", map[string]any{"topic": availabilityTopic})
	if err := client.Publish(availabilityTopic, []byte("online"), retained); err != nil {
		_ = client.Close()
		return fail("publish_availability", err)
	}
	published = append(published, availabilityTopic)
	event("publish_availability", "ok", "Availability topic published", map[string]any{"topic": availabilityTopic})
	stateTopic := root + "/connection_test/state"
	event("publish_state", "running", "Publishing retained test state", map[string]any{"topic": stateTopic})
	if err := client.Publish(stateTopic, []byte(time.Now().UTC().Format(time.RFC3339)), retained); err != nil {
		_ = client.Close()
		return fail("publish_state", err)
	}
	published = append(published, stateTopic)
	event("publish_state", "ok", "Test state topic published", map[string]any{"topic": stateTopic})
	configTopic := discovery + "/sensor/" + node + "/connection_test/config"
	payload, _ := json.Marshal(map[string]any{
		"name":               "Connection Test",
		"unique_id":          node + "_connection_test",
		"state_topic":        stateTopic,
		"icon":               "mdi:lan-connect",
		"entity_category":    "diagnostic",
		"availability_topic": availabilityTopic,
		"device": map[string]any{
			"identifiers":  []string{node},
			"name":         "KioskMate",
			"manufacturer": "KioskMate",
		},
	})
	event("publish_discovery", "running", "Publishing retained Home Assistant discovery config", map[string]any{"topic": configTopic})
	if err := client.Publish(configTopic, payload, retained); err != nil {
		_ = client.Close()
		return fail("publish_discovery", err)
	}
	published = append(published, configTopic)
	event("publish_discovery", "ok", "Discovery config topic published", map[string]any{"topic": configTopic})
	_ = client.Close()
	event("done", "ok", "MQTT test completed successfully", map[string]any{"published_topics": published})
	return map[string]any{"ok": true, "latency_ms": time.Since(start).Milliseconds(), "version": body.Version, "published_topics": published}
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
	token := r.Header.Get("X-KioskMate-Token")
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
