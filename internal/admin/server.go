package admin

import (
	"archive/zip"
	"bytes"
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
	"image/png"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
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
	"github.com/MickLesk/KioskMate/internal/integration"
	"github.com/MickLesk/KioskMate/internal/mqttclient"
	"github.com/MickLesk/KioskMate/internal/supervisor"
	"github.com/MickLesk/KioskMate/internal/systemtime"
	"github.com/MickLesk/KioskMate/internal/updater"
	"golang.org/x/crypto/argon2"
)

//go:embed web/index.html web/assets/*
var content embed.FS

const sessionCookieName = "kioskmate_session"

var errSetupAlreadyCompleted = errors.New("setup already completed")

type Browser interface {
	Start(context.Context) error
	Stop(context.Context) error
	Restart(context.Context) error
	Next(context.Context) error
	Previous(context.Context) error
	ResetSession(context.Context) error
	Reload(context.Context) error
	SetActive(context.Context, int) error
	CaptureScreenshot(context.Context) ([]byte, error)
	TripAuthGuard(string)
	NoteDisplayPower(string)
	Status() supervisor.Status
}

type runtimeBrowser interface {
	Recover(context.Context, string) error
	SetOverride(context.Context, int, time.Duration, string) error
	ClearOverride() error
	Telemetry() supervisor.TelemetryHistory
	ResetTelemetry() error
}

type MQTTDiscoveryPublisher interface {
	PublishNow() error
	ResetDiscovery() (int, error)
	ConnectionStatus() integration.MQTTConnectionStatus
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
	snapshot snapshotCache
}

type snapshotCache struct {
	mu   sync.Mutex
	png  []byte
	url  string
	time time.Time
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
	s := &Server{cfg: cfg, browser: browser, mqtt: mqtt, updater: updates, actions: actions, hardware: hw, version: version, logger: logger, sessions: map[string]Session{}, attempts: map[string][]time.Time{}}
	s.loadSessions()
	return s
}

// sessionsFilePath returns the private, per-installation file used to persist
// Admin sessions across restarts. Empty when no config path is known (e.g. tests).
func (s *Server) sessionsFilePath() string {
	if s.cfg == nil || s.cfg.Path == "" {
		return ""
	}
	return filepath.Join(config.ConfigDir(s.cfg.Path), "sessions.json")
}

// persistSessions writes the current session table to disk so signed-in Admin
// sessions survive a service restart or update instead of forcing a re-login.
func (s *Server) persistSessions() {
	path := s.sessionsFilePath()
	if path == "" {
		return
	}
	s.mu.Lock()
	sessions := make(map[string]Session, len(s.sessions))
	for id, session := range s.sessions {
		sessions[id] = session
	}
	s.mu.Unlock()
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(path)
		_ = os.Rename(temporary, path)
	}
}

// loadSessions restores previously persisted Admin sessions on startup. Sessions
// already expired by the normal 7-day/24h-inactivity rules are dropped immediately.
func (s *Server) loadSessions() {
	path := s.sessionsFilePath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var sessions map[string]Session
	if json.Unmarshal(data, &sessions) != nil {
		return
	}
	s.mu.Lock()
	if sessions == nil {
		sessions = map[string]Session{}
	}
	s.sessions = sessions
	s.pruneSessionsLocked(time.Now())
	s.mu.Unlock()
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/assets/", s.asset)
	mux.HandleFunc("/api/auth/status", s.authStatus)
	mux.HandleFunc("/api/auth/setup", s.authSetup)
	mux.HandleFunc("/api/auth/login", s.authLogin)
	mux.HandleFunc("/api/auth/logout", s.authLogout)
	mux.HandleFunc("/api/auth/sessions", s.auth(s.authSessions))
	mux.HandleFunc("/api/auth/logout-all", s.auth(s.authLogoutAll))
	mux.HandleFunc("/api/auth/password", s.auth(s.authPassword))
	mux.HandleFunc("/api/status", s.auth(s.status))
	mux.HandleFunc("/api/time", s.auth(s.timeStatus))
	mux.HandleFunc("/api/time/zones", s.auth(s.timeZones))
	mux.HandleFunc("/api/logs", s.auth(s.logs))
	mux.HandleFunc("/api/logs/download", s.auth(s.logsDownload))
	mux.HandleFunc("/api/diagnostics/export", s.auth(s.diagnosticsExport))
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
	mux.HandleFunc("/api/mqtt/discovery-reset", s.auth(s.mqttDiscoveryReset))
	mux.HandleFunc("/api/browser/start", s.auth(s.browserAction("start")))
	mux.HandleFunc("/api/browser/stop", s.auth(s.browserAction("stop")))
	mux.HandleFunc("/api/browser/restart", s.auth(s.browserAction("restart")))
	mux.HandleFunc("/api/browser/reload", s.auth(s.browserAction("reload")))
	mux.HandleFunc("/api/browser/next", s.auth(s.browserAction("next")))
	mux.HandleFunc("/api/browser/previous", s.auth(s.browserAction("previous")))
	mux.HandleFunc("/api/browser/reset-session", s.auth(s.browserAction("reset-session")))
	mux.HandleFunc("/api/browser/page", s.auth(s.browserPage))
	mux.HandleFunc("/api/browser/check-page", s.auth(s.browserCheckPage))
	mux.HandleFunc("/api/browser/render-check", s.auth(s.browserRenderCheck))
	mux.HandleFunc("/api/browser/snapshot", s.auth(s.browserSnapshot))
	mux.HandleFunc("/api/browser/doctor", s.auth(s.browserDoctor))
	mux.HandleFunc("/api/browser/recover", s.auth(s.browserRecover))
	mux.HandleFunc("/api/browser/recovery", s.auth(s.browserRecovery))
	mux.HandleFunc("/api/browser/override", s.auth(s.browserOverride))
	mux.HandleFunc("/api/browser/telemetry", s.auth(s.browserTelemetry))
	mux.HandleFunc("/api/browser/profile-recommendation", s.auth(s.browserProfileRecommendation))
	mux.HandleFunc("/api/browser/diagnostics", s.auth(s.browserDiagnostics))
	mux.HandleFunc("/api/browser/safe-mode", s.auth(s.browserSafeMode))
	mux.HandleFunc("/api/update", s.auth(s.update))
	mux.HandleFunc("/api/update/preflight", s.auth(s.updatePreflight))
	mux.HandleFunc("/api/update/history", s.auth(s.updateHistory))
	mux.HandleFunc("/api/update/install", s.auth(s.updateInstall))
	mux.HandleFunc("/api/update/rollback", s.auth(s.updateRollback))
	mux.HandleFunc("/api/update/jobs/", s.auth(s.updateJob))
	mux.HandleFunc("/api/repair", s.auth(s.repair))
	mux.HandleFunc("/api/system/", s.auth(s.systemAction))
	mux.HandleFunc("/api/privilege", s.auth(s.privilege))
	mux.HandleFunc("/api/hardware", s.auth(s.hardwareStatus))
	mux.HandleFunc("/api/hardware/", s.auth(s.hardwareAction))
	mux.HandleFunc("/api/jobs", s.auth(s.jobs))
	mux.HandleFunc("/api/jobs/", s.auth(s.job))

	server := &http.Server{
		Addr:              s.cfg.Snapshot().Admin.Addr(),
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
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
		if s.cfg.Snapshot().Admin.TLSCert != "" && s.cfg.Snapshot().Admin.TLSKey != "" {
			errc <- server.ServeTLS(listener, s.cfg.Snapshot().Admin.TLSCert, s.cfg.Snapshot().Admin.TLSKey)
			return
		}
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
		if err == nil {
			s.browser.NoteDisplayPower(fmt.Sprint(body.Value))
		}
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

func (s *Server) jobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	limit := 25
	if value, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && value > 0 && value <= 100 {
		limit = value
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": s.actions.Jobs(limit)})
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
	data = []byte(strings.ReplaceAll(string(data), "__KIOSKMATE_ASSET_VERSION__", url.QueryEscape(s.version)))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	_, _ = w.Write(data)
}

func (s *Server) asset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/assets/")
	if name == "" || strings.Contains(name, "..") || strings.ContainsAny(name, `\/`) {
		http.NotFound(w, r)
		return
	}
	path := "web/assets/" + name
	data, err := content.ReadFile(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch filepath.Ext(name) {
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(data)
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	snapshot := s.cfg.Snapshot()
	authenticated := s.authenticated(r)
	response := map[string]any{
		"authenticated": authenticated,
		"setupRequired": snapshot.Admin.PasswordHash == "",
		"tokenRequired": snapshot.Admin.PasswordHash == "",
		"configPath":    s.cfg.Path,
		"version":       s.version,
	}
	if authenticated {
		response["config"] = publicConfigSnapshot(snapshot)
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) authSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.Snapshot().Admin.PasswordHash != "" {
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
	if subtle.ConstantTimeCompare([]byte(body.Token), []byte(s.cfg.Snapshot().Admin.Token)) != 1 {
		s.recordFailedAttempt(r)
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
	// Re-check PasswordHash inside the Mutate callback (which holds the config lock for
	// the whole read-modify-write) so two concurrent setup requests cannot race past the
	// earlier, non-atomic check above and both "win" the one-time setup.
	if err := s.cfg.Mutate(func(next *config.Config) error {
		if next.Admin.PasswordHash != "" {
			return errSetupAlreadyCompleted
		}
		next.Admin.PasswordHash = hash
		return nil
	}); err != nil {
		if errors.Is(err, errSetupAlreadyCompleted) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "setup already completed"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.clearAttempts(r)
	s.createSession(w, r)
	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"authenticated": true,
		"version":       s.version,
		"config":        publicConfig(s.cfg),
	})
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
	if s.cfg.Snapshot().Admin.PasswordHash != "" {
		ok = VerifyPassword(body.Password, s.cfg.Snapshot().Admin.PasswordHash)
	} else if body.Token != "" {
		ok = subtle.ConstantTimeCompare([]byte(body.Token), []byte(s.cfg.Snapshot().Admin.Token)) == 1
	}
	if !ok {
		s.recordFailedAttempt(r)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	if strings.HasPrefix(s.cfg.Snapshot().Admin.PasswordHash, "sha256iter$") {
		if hash, err := HashPassword(body.Password); err == nil {
			_ = s.cfg.Mutate(func(next *config.Config) error {
				next.Admin.PasswordHash = hash
				return nil
			})
		}
	}
	s.clearAttempts(r)
	s.createSession(w, r)
	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"authenticated": true,
		"version":       s.version,
		"config":        publicConfig(s.cfg),
	})
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
		s.persistSessions()
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
	s.pruneSessionsLocked(time.Now())
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
	s.persistSessions()
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
	if s.cfg.Snapshot().Admin.PasswordHash != "" && !VerifyPassword(body.Current, s.cfg.Snapshot().Admin.PasswordHash) {
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
	if err := s.cfg.Mutate(func(next *config.Config) error {
		next.Admin.PasswordHash = hash
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// Invalidate every existing Admin session after a password change so a
	// stolen cookie cannot keep working with the old credentials.
	s.mu.Lock()
	s.sessions = map[string]Session{}
	s.mu.Unlock()
	s.persistSessions()
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg.Snapshot()
	mqttStatus := integration.MQTTConnectionStatus{State: "unavailable"}
	if s.mqtt != nil {
		mqttStatus = s.mqtt.ConnectionStatus()
	}
	updateStatus := updater.ReleaseInfo{CurrentVersion: s.version}
	if s.updater != nil {
		updateStatus = s.updater.Status()
	}
	if r.URL.Query().Get("fast") == "1" {
		writeJSON(w, http.StatusOK, map[string]any{
			"browser":                s.browser.Status(),
			"hardware":               hardware.Status{},
			"mqtt":                   mqttStatus,
			"update":                 updateStatus,
			"profile_recommendation": map[string]any{},
			"config":                 statusConfig(cfg),
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	hardwareStatus := s.hardware.Status(ctx)
	recommendation := performanceRecommendation(hardwareStatus)
	writeJSON(w, http.StatusOK, map[string]any{
		"browser":                s.browser.Status(),
		"hardware":               hardwareStatus,
		"mqtt":                   mqttStatus,
		"update":                 updateStatus,
		"profile_recommendation": recommendation,
		"config":                 statusConfig(cfg),
	})
}

func statusConfig(cfg *config.Config) map[string]any {
	return map[string]any{
		"path":                     cfg.Path,
		"profile":                  cfg.Performance.Profile,
		"gpu_mode":                 cfg.Performance.GPUMode,
		"theme":                    cfg.Kiosk.Theme,
		"zoom":                     cfg.Kiosk.ZoomPercent,
		"widget":                   cfg.Kiosk.Widget,
		"watchdog":                 cfg.Watchdog.Enabled,
		"admin_addr":               cfg.Admin.Addr(),
		"kiosk_urls":               cfg.Kiosk.URLs,
		"kiosk_pages":              cfg.Kiosk.Pages,
		"scheduler":                cfg.Kiosk.Scheduler,
		"rotation":                 cfg.Kiosk.Rotation,
		"time_rules":               cfg.Kiosk.TimeRules,
		"browser_cmd":              cfg.Kiosk.BrowserCommand,
		"mqtt":                     cfg.MQTT.Enabled,
		"mqtt_password_configured": cfg.MQTT.Password != "",
	}
}

func (s *Server) timeStatus(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		writeJSON(w, http.StatusOK, systemtime.Read(ctx, s.cfg.Snapshot().Time.NTPServer))
	case http.MethodPost:
		var body struct {
			Timezone  string `json:"timezone"`
			NTPServer string `json:"ntp_server"`
			Mode      string `json:"mode"`
			Password  string `json:"password"`
			Remember  bool   `json:"remember"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if body.Remember && body.Password != "" {
			if err := s.actions.RememberPrivilege(body.Mode, body.Password); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		}
		job, err := s.actions.StartTimeConfig(context.Background(), body.Timezone, body.NTPServer, body.Mode, body.Password)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := s.cfg.Mutate(func(next *config.Config) error {
			next.Time.Timezone = body.Timezone
			next.Time.NTPServer = body.NTPServer
			return nil
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, job)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) timeZones(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"zones": systemtime.Zones()})
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
	source := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("source")))
	if source == "" {
		source = "combined"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	result := s.logLines(ctx, source, lines)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) logsDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	source := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("source")))
	if source == "" {
		source = "combined"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	result := s.logLines(ctx, source, 2000)
	lines, _ := result["lines"].([]string)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="kioskmate-logs.txt"`)
	_, _ = io.WriteString(w, strings.Join(lines, "\n"))
}

func (s *Server) diagnosticsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	addZipText(zw, "summary.txt", strings.Join(s.logPathLines(), "\n"))
	addZipJSON(zw, "config.redacted.json", redactSecrets(s.cfg))
	addZipJSON(zw, "status.json", map[string]any{
		"version":  s.version,
		"browser":  s.browser.Status(),
		"hardware": s.hardware.Status(ctx),
	})
	addZipText(zw, "logs.combined.txt", strings.Join(s.combinedLogs(ctx, 1500), "\n"))
	addZipText(zw, "logs.core.txt", strings.Join(labeledTail("core", config.LogFilePath(s.cfg.Path), 1500), "\n"))
	addZipText(zw, "logs.browser.txt", strings.Join(labeledTail("browser", config.BrowserLogFilePath(s.cfg.Path), 1500), "\n"))
	if err := zw.Close(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="kioskmate-diagnostics.zip"`)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) logLines(ctx context.Context, source string, limit int) map[string]any {
	sources := []string{"combined", "core", "browser", "journal", "status", "paths"}
	result := map[string]any{"source": source, "sources": sources}
	switch source {
	case "core":
		result["lines"] = labeledTail("core", config.LogFilePath(s.cfg.Path), limit)
	case "browser":
		result["lines"] = labeledTail("browser", config.BrowserLogFilePath(s.cfg.Path), limit)
	case "journal":
		lines, warning := s.journalLines(ctx, limit)
		result["lines"] = lines
		if warning != "" {
			result["warning"] = warning
		}
	case "status":
		result["lines"] = s.statusLines(ctx)
	case "paths":
		result["lines"] = s.logPathLines()
	default:
		result["source"] = "combined"
		result["lines"] = s.combinedLogs(ctx, limit)
	}
	return result
}

func (s *Server) combinedLogs(ctx context.Context, limit int) []string {
	var lines []string
	lines = append(lines, labeledTail("core", config.LogFilePath(s.cfg.Path), limit/2)...)
	lines = append(lines, labeledTail("browser", config.BrowserLogFilePath(s.cfg.Path), limit/2)...)
	if journal, warning := s.journalLines(ctx, max(40, limit/4)); len(journal) > 0 {
		lines = append(lines, sectionHeader("journal")...)
		lines = append(lines, journal...)
	} else if warning != "" {
		lines = append(lines, sectionHeader("journal")...)
		lines = append(lines, warning)
	}
	lines = append(lines, s.statusLines(ctx)...)
	lines = append(lines, s.logPathLines()...)
	if len(lines) == 0 {
		return []string{"No logs available yet."}
	}
	return lines
}

func labeledTail(label string, path string, limit int) []string {
	lines := sectionHeader(label + " log")
	lines = append(lines, "$ tail "+path)
	fileLines := readTail(path, max(1, limit))
	if len(fileLines) == 0 {
		lines = append(lines, "(empty or not found)")
		return lines
	}
	return append(lines, fileLines...)
}

func (s *Server) journalLines(ctx context.Context, limit int) ([]string, string) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		return nil, "journalctl not found"
	}
	out, err := exec.CommandContext(ctx, "journalctl", "--user", "-u", s.cfg.Snapshot().Update.Service, "-n", strconv.Itoa(max(1, limit)), "--no-pager").CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return splitLines(text), "journalctl failed: " + err.Error()
	}
	if text == "" || strings.Contains(text, "No journal files were found") || strings.Contains(text, "-- No entries --") {
		return splitLines(text), "no journal entries"
	}
	return splitLines(text), ""
}

func (s *Server) statusLines(ctx context.Context) []string {
	lines := sectionHeader("service status")
	if out, err := exec.CommandContext(ctx, "systemctl", "--user", "status", s.cfg.Snapshot().Update.Service, "--no-pager", "--full").CombinedOutput(); len(out) > 0 || err != nil {
		lines = append(lines, "$ systemctl --user status "+s.cfg.Snapshot().Update.Service+" --no-pager --full")
		lines = append(lines, splitLines(strings.TrimSpace(string(out)))...)
		if err != nil {
			lines = append(lines, "error: "+err.Error())
		}
		return lines
	}
	return append(lines, "(status unavailable)")
}

func (s *Server) logPathLines() []string {
	return []string{
		"== paths ==",
		"Core log: " + config.LogFilePath(s.cfg.Path),
		"Browser log: " + config.BrowserLogFilePath(s.cfg.Path),
		"Config: " + s.cfg.Path,
		"Config backup: " + s.cfg.Path + ".bak",
		"Tip: run `kioskmate --admin-info` on the kiosk for full recovery paths.",
	}
}

func sectionHeader(label string) []string {
	return []string{"", "== " + label + " =="}
}

func splitLines(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return strings.Split(strings.TrimSpace(text), "\n")
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

func addZipText(zw *zip.Writer, name string, text string) {
	file, err := zw.Create(name)
	if err != nil {
		return
	}
	_, _ = io.WriteString(file, text)
}

func addZipJSON(zw *zip.Writer, name string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		data = []byte(`{"error":"json marshal failed"}`)
	}
	addZipText(zw, name, string(data))
}

func redactSecrets(value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	var generic any
	if err := json.Unmarshal(data, &generic); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return redactValue("", generic)
}

func redactValue(key string, value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for childKey, childValue := range typed {
			out[childKey] = redactValue(childKey, childValue)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = redactValue(key, item)
		}
		return out
	case string:
		lower := strings.ToLower(key)
		if strings.Contains(lower, "password") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "hash") {
			if typed == "" {
				return ""
			}
			return "<redacted>"
		}
		return typed
	default:
		return typed
	}
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
		writeJSON(w, http.StatusOK, publicConfig(s.cfg))
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
		if err := s.cfg.Replace(next); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
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
	data, err := json.MarshalIndent(publicConfig(s.cfg), "", "  ")
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
	if err := s.cfg.Replace(next); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
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
		cfg := s.cfg.Snapshot()
		writeJSON(w, http.StatusOK, config.Repair(cfg))
	case http.MethodPost:
		var report config.RepairReport
		if err := s.cfg.Mutate(func(next *config.Config) error {
			report = config.Repair(next)
			return nil
		}); err != nil {
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
	if err := s.cfg.Replace(next); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
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
		next.Admin.Token = s.cfg.Snapshot().Admin.Token
	}
	if next.Admin.PasswordHash == "" {
		next.Admin.PasswordHash = s.cfg.Snapshot().Admin.PasswordHash
	}
	if next.MQTT.Password == "" {
		next.MQTT.Password = s.cfg.Snapshot().MQTT.Password
	}
	next.MQTT.PasswordConfigured = false
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
	if r.URL.Query().Get("cached") == "1" {
		writeJSON(w, http.StatusOK, s.updater.Status())
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
	var body struct {
		Mode     string `json:"mode"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	job, err := s.updater.Install(context.Background(), body.Mode, body.Password)
	if err != nil {
		status := http.StatusBadRequest
		code := "update_failed"
		if errors.Is(err, updater.ErrPrivilegeRequired) {
			status = http.StatusConflict
			code = "privilege_required"
		} else if errors.Is(err, updater.ErrUpdateInProgress) {
			status = http.StatusConflict
			code = "update_in_progress"
		}
		writeJSON(w, status, map[string]string{"error": err.Error(), "code": code})
		return
	}
	if body.Remember && body.Password != "" {
		if err := s.actions.RememberPrivilege(body.Mode, body.Password); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) updatePreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Mode     string `json:"mode"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	report := s.updater.Preflight(ctx, body.Mode, body.Password)
	if report.OK && body.Remember && body.Password != "" {
		if err := s.actions.RememberPrivilege(body.Mode, body.Password); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) updateHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.updater.History())
}

func (s *Server) updateRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Mode     string `json:"mode"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	job, err := s.updater.Rollback(context.Background(), body.Mode, body.Password)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, updater.ErrPrivilegeRequired) || errors.Is(err, updater.ErrUpdateInProgress) {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	if body.Remember && body.Password != "" {
		_ = s.actions.RememberPrivilege(body.Mode, body.Password)
	}
	writeJSON(w, http.StatusAccepted, job)
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
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error":       err.Error(),
				"success":     false,
				"action":      action,
				"browser":     s.browser.Status(),
				"browser_log": labeledTail("browser", config.BrowserLogFilePath(s.cfg.Path), 80),
				"core_log":    labeledTail("core", config.LogFilePath(s.cfg.Path), 40),
			})
			return
		}
		if shouldVerifyBrowserRunning(action) {
			status := waitForBrowserState(s.browser, true, 1500*time.Millisecond)
			if !status.Running {
				message := status.LastError
				if message == "" {
					message = "browser did not stay running after " + action
				}
				writeJSON(w, http.StatusInternalServerError, map[string]any{
					"error":       message,
					"success":     false,
					"action":      action,
					"browser":     status,
					"browser_log": labeledTail("browser", config.BrowserLogFilePath(s.cfg.Path), 80),
					"core_log":    labeledTail("core", config.LogFilePath(s.cfg.Path), 40),
				})
				return
			}
		}
		status := s.browser.Status()
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "action": action, "browser": status})
	}
}

func shouldVerifyBrowserRunning(action string) bool {
	switch action {
	case "start", "restart", "reload", "next", "previous", "reset-session":
		return true
	default:
		return false
	}
}

func waitForBrowserState(browser Browser, running bool, timeout time.Duration) supervisor.Status {
	deadline := time.Now().Add(timeout)
	status := browser.Status()
	for time.Now().Before(deadline) {
		status = browser.Status()
		if status.Running == running {
			return status
		}
		time.Sleep(150 * time.Millisecond)
	}
	return browser.Status()
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
		"page_count":       s.cfg.Snapshot().Kiosk.PageCount(),
		"isolated_pages":   s.cfg.Snapshot().Kiosk.IsolateSessions,
		"profile":          s.cfg.Snapshot().Performance.Profile,
		"gpu_mode":         s.cfg.Snapshot().Performance.GPUMode,
		"reduce_motion":    s.cfg.Snapshot().Performance.ReduceMotion,
		"watchdog":         s.cfg.Snapshot().Watchdog.Enabled,
		"watchdog_status":  status.Watchdog,
		"browser_log":      config.BrowserLogFilePath(s.cfg.Path),
		"core_log":         config.LogFilePath(s.cfg.Path),
		"last_error":       status.LastError,
		"last_exit":        status.LastExit,
	})
}

func (s *Server) browserDoctor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	status := s.browser.Status()
	checks := []map[string]any{}
	add := func(id string, level string, message string, detail any) {
		checks = append(checks, map[string]any{"id": id, "level": level, "message": message, "detail": detail})
	}
	command := status.Command
	if command == "" {
		add("browser_command", "error", "Browser command is empty.", "")
	} else if resolved, err := exec.LookPath(command); err == nil {
		add("browser_command", "ok", "Browser command resolved.", resolved)
	} else if filepath.IsAbs(command) && fileExists(command) {
		add("browser_command", "ok", "Browser command exists.", command)
	} else {
		add("browser_command", "error", "Browser command was not found.", err.Error())
	}
	env := map[string]string{
		"DISPLAY":          os.Getenv("DISPLAY"),
		"WAYLAND_DISPLAY":  os.Getenv("WAYLAND_DISPLAY"),
		"XDG_RUNTIME_DIR":  os.Getenv("XDG_RUNTIME_DIR"),
		"XDG_SESSION_TYPE": os.Getenv("XDG_SESSION_TYPE"),
	}
	if env["DISPLAY"] == "" && env["WAYLAND_DISPLAY"] == "" {
		add("display_env", "error", "No DISPLAY or WAYLAND_DISPLAY is set for the kiosk service.", env)
	} else {
		add("display_env", "ok", "Display environment is present.", env)
	}
	if env["WAYLAND_DISPLAY"] != "" || env["XDG_SESSION_TYPE"] == "wayland" {
		if _, err := exec.LookPath("wlopm"); err == nil {
			add("display_power_wayland", "ok", "wlopm is installed for Wayland display power control.", "")
		} else if _, err := exec.LookPath("wlr-randr"); err == nil {
			add("display_power_wayland", "ok", "wlr-randr is installed for Wayland display power control.", "")
		} else {
			add("display_power_wayland", "warn", "No Wayland display power tool found. Install wlopm (recommended) or wlr-randr so the display can be switched on/off.", "")
		}
	}
	if env["XDG_RUNTIME_DIR"] == "" {
		add("runtime_dir", "warn", "XDG_RUNTIME_DIR is empty. User services may not reach the graphical session.", env)
	} else if info, err := os.Stat(env["XDG_RUNTIME_DIR"]); err == nil && info.IsDir() {
		add("runtime_dir", "ok", "Runtime directory exists.", env["XDG_RUNTIME_DIR"])
	} else {
		add("runtime_dir", "error", "Runtime directory is not accessible.", env["XDG_RUNTIME_DIR"])
	}
	profile := s.cfg.Snapshot().Kiosk.UserDataDir
	if profile == "" {
		add("browser_profile", "error", "Browser profile path is empty.", "")
	} else if err := os.MkdirAll(profile, 0o700); err != nil {
		add("browser_profile", "error", "Browser profile directory cannot be created.", err.Error())
	} else if err := os.WriteFile(filepath.Join(profile, ".kioskmate-write-test"), []byte(time.Now().String()), 0o600); err != nil {
		add("browser_profile", "error", "Browser profile directory is not writable.", err.Error())
	} else {
		_ = os.Remove(filepath.Join(profile, ".kioskmate-write-test"))
		add("browser_profile", "ok", "Browser profile directory is writable.", profile)
	}
	if status.Running {
		add("browser_running", "ok", "Browser is running.", status.PID)
	} else if status.LastError != "" {
		add("browser_running", "error", "Browser is stopped with a last error.", status.LastError)
	} else {
		add("browser_running", "warn", "Browser is currently stopped.", "")
	}
	if status.URL == "" {
		add("active_page", "warn", "No active kiosk URL is available.", "")
	} else if pageResult := checkHTTPPage(ctx, status.URL, s.version); pageResult["ok"] == true {
		add("active_page", "ok", "Active page is reachable from the kiosk host.", pageResult)
	} else {
		if pageResult["statusCode"] == http.StatusForbidden && isHomeAssistantURL(status.URL) {
			s.browser.TripAuthGuard("Home Assistant returned HTTP 403 during Browser Doctor")
		}
		add("active_page", "error", "Active page is not reachable from the kiosk host.", pageResult)
	}
	add("logs", "ok", "Log files checked.", map[string]any{
		"core":    config.LogFilePath(s.cfg.Path),
		"browser": config.BrowserLogFilePath(s.cfg.Path),
		"tail":    append(labeledTail("browser", config.BrowserLogFilePath(s.cfg.Path), 30), labeledTail("core", config.LogFilePath(s.cfg.Path), 20)...),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      !hasDoctorLevel(checks, "error"),
		"checks":  checks,
		"browser": status,
		"advice":  doctorAdvice(checks),
	})
}

func (s *Server) browserRecover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	steps := []map[string]any{}
	step := func(name string, level string, detail any) {
		steps = append(steps, map[string]any{"name": name, "level": level, "detail": detail, "time": time.Now().Format(time.RFC3339)})
	}
	step("stop", "running", "stopping browser")
	if err := s.browser.Stop(ctx); err != nil {
		step("stop", "error", err.Error())
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "steps": steps, "error": err.Error()})
		return
	}
	step("stop", "ok", "browser stopped")
	step("reset_session", "running", "resetting Home Assistant browser session")
	if err := s.browser.ResetSession(ctx); err != nil {
		step("reset_session", "error", err.Error())
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "steps": steps, "error": err.Error(), "browser": s.browser.Status()})
		return
	}
	status := waitForBrowserState(s.browser, true, 2*time.Second)
	if !status.Running {
		errText := firstNonEmpty(status.LastError, "browser did not stay running after session reset")
		step("start", "error", errText)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok":          false,
			"steps":       steps,
			"error":       errText,
			"browser":     status,
			"browser_log": labeledTail("browser", config.BrowserLogFilePath(s.cfg.Path), 80),
			"core_log":    labeledTail("core", config.LogFilePath(s.cfg.Path), 40),
		})
		return
	}
	step("start", "ok", map[string]any{"pid": status.PID, "url": status.URL})
	if status.URL != "" {
		page := checkHTTPPage(ctx, status.URL, s.version)
		level := "error"
		if page["ok"] == true {
			level = "ok"
		}
		step("page_check", level, page)
		if status.Running {
			pngData, err := s.browser.CaptureScreenshot(ctx)
			analysis := analyzePNG(pngData)
			if err != nil {
				step("render_check", "warn", map[string]any{"error": err.Error(), "analysis": analysis})
			} else if analysis["visible"] == true {
				step("render_check", "ok", analysis)
			} else {
				step("render_check", "warn", analysis)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "steps": steps, "browser": s.browser.Status()})
}

func (s *Server) browserRecovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	runtime, ok := s.browser.(runtimeBrowser)
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "runtime recovery is unavailable"})
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if strings.TrimSpace(body.Reason) == "" {
		body.Reason = "requested from Admin UI"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := runtime.Recover(ctx, body.Reason); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error(), "browser": s.browser.Status()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "browser": s.browser.Status()})
}

func (s *Server) browserOverride(w http.ResponseWriter, r *http.Request) {
	runtime, ok := s.browser.(runtimeBrowser)
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "page overrides are unavailable"})
		return
	}
	switch r.Method {
	case http.MethodPost:
		var body struct {
			Page            int    `json:"page"`
			DurationSeconds int    `json:"duration_seconds"`
			Source          string `json:"source"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		if err := runtime.SetOverride(ctx, body.Page, time.Duration(body.DurationSeconds)*time.Second, body.Source); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "browser": s.browser.Status()})
	case http.MethodDelete:
		if err := runtime.ClearOverride(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "browser": s.browser.Status()})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) browserTelemetry(w http.ResponseWriter, r *http.Request) {
	runtime, ok := s.browser.(runtimeBrowser)
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "runtime telemetry is unavailable"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, runtime.Telemetry())
	case http.MethodDelete:
		if err := runtime.ResetTelemetry(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) browserProfileRecommendation(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	recommendation := performanceRecommendation(s.hardware.Status(ctx))
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, recommendation)
	case http.MethodPost:
		profile, _ := recommendation["profile"].(string)
		gpuMode, _ := recommendation["gpu_mode"].(string)
		if err := s.cfg.Mutate(func(next *config.Config) error {
			next.Performance.Profile = profile
			next.Performance.GPUMode = gpuMode
			return nil
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		if err := s.browser.Restart(ctx); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "recommendation": recommendation, "browser": s.browser.Status()})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func performanceRecommendation(status hardware.Status) map[string]any {
	model := strings.ToLower(fmt.Sprint(status.Device["model"]))
	memory := numberValue(status.System["memory_size_gib"])
	session := strings.ToLower(status.Session["type"])
	profile, gpuMode, reason, reasonKey := "balanced", "auto", "Balanced defaults match this device.", "profileReasonBalanced"
	switch {
	case strings.Contains(model, "raspberry pi 3") || (memory > 0 && memory <= 1.5):
		profile, gpuMode, reason, reasonKey = "minimal", "software", "Low-memory hardware benefits from one renderer and conservative graphics.", "profileReasonLowMemory"
	case strings.Contains(model, "raspberry pi 4") || strings.Contains(model, "raspberry pi 400") || (memory > 0 && memory <= 4.5):
		profile, gpuMode, reason, reasonKey = "raspberry", "auto", "Raspberry Pi 4 class hardware benefits from the low-power raster profile.", "profileReasonPi4"
	case strings.Contains(model, "raspberry pi 5"):
		profile, gpuMode, reason, reasonKey = "balanced", "hardware", "Raspberry Pi 5 can use hardware acceleration with balanced process limits.", "profileReasonPi5"
	case memory >= 8:
		profile, gpuMode, reason, reasonKey = "quality", "hardware", "Available memory supports Chromium's normal rendering quality.", "profileReasonHighMemory"
	}
	return map[string]any{"profile": profile, "gpu_mode": gpuMode, "reason": reason, "reason_key": reasonKey, "model": status.Device["model"], "memory_gib": memory, "session": session}
}

func numberValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case json.Number:
		result, _ := typed.Float64()
		return result
	default:
		result, _ := strconv.ParseFloat(fmt.Sprint(value), 64)
		return result
	}
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
	if err := s.cfg.Mutate(func(next *config.Config) error {
		config.ApplyRaspberrySafeMode(next)
		return nil
	}); err != nil {
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
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "config": publicConfig(s.cfg)})
}

func publicConfig(cfg *config.Config) *config.Config {
	return publicConfigSnapshot(cfg.Snapshot())
}

func publicConfigSnapshot(out *config.Config) *config.Config {
	out.MQTT.PasswordConfigured = out.MQTT.Password != ""
	out.Admin.Token = ""
	out.Admin.PasswordHash = ""
	out.MQTT.Password = ""
	return out
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
	s.applyStoredMQTTPassword(&body)
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
	s.applyStoredMQTTPassword(&body)
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

func (s *Server) applyStoredMQTTPassword(body *mqttTestRequest) {
	if body != nil && body.Password == "" {
		body.Password = s.cfg.Snapshot().MQTT.Password
	}
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
	pageCount := s.cfg.Snapshot().Kiosk.PageCount()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"discovery_prefix": strings.Trim(s.cfg.Snapshot().MQTT.Discovery, "/"),
		"root_topic":       strings.Trim(s.cfg.Snapshot().MQTT.BaseTopic, "/") + "/" + strings.Trim(s.cfg.Snapshot().MQTT.Node, "/"),
		"node":             s.cfg.Snapshot().MQTT.Node,
		"page_count":       pageCount,
		"page_entities":    pageCount * 9,
	})
}

func (s *Server) mqttDiscoveryReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.mqtt == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "mqtt service unavailable"})
		return
	}
	count, err := s.mqtt.ResetDiscovery()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "cleared": count})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cleared": count})
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
		urls := s.cfg.Snapshot().Kiosk.PageURLs()
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
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "url": target, "error": err.Error(), "category": "network", "hint": "The kiosk host could not reach this page. Check DNS, network, firewall and the Home Assistant URL."})
		return
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, 4096)
	ok := resp.StatusCode >= 200 && resp.StatusCode < 400
	category, hint := pageCheckHint(target, resp)
	if resp.StatusCode == http.StatusForbidden && isHomeAssistantURL(target) {
		s.browser.TripAuthGuard("Home Assistant returned HTTP 403 during page check")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         ok,
		"url":        target,
		"final_url":  resp.Request.URL.String(),
		"status":     resp.Status,
		"statusCode": resp.StatusCode,
		"category":   category,
		"hint":       hint,
	})
}

func checkHTTPPage(ctx context.Context, target string, version string) map[string]any {
	result := map[string]any{"ok": false, "url": target}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		result["error"] = "only http and https URLs can be checked"
		result["category"] = "invalid_url"
		return result
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		result["error"] = err.Error()
		result["category"] = "invalid_url"
		return result
	}
	req.Header.Set("User-Agent", "KioskMate/"+version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		result["error"] = err.Error()
		result["category"] = "network"
		result["hint"] = "The kiosk host could not reach this page. Check DNS, network, firewall and the Home Assistant URL."
		return result
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, 4096)
	ok := resp.StatusCode >= 200 && resp.StatusCode < 400
	category, hint := pageCheckHint(target, resp)
	result["ok"] = ok
	result["final_url"] = resp.Request.URL.String()
	result["status"] = resp.Status
	result["statusCode"] = resp.StatusCode
	result["category"] = category
	result["hint"] = hint
	return result
}

func hasDoctorLevel(checks []map[string]any, level string) bool {
	for _, check := range checks {
		if check["level"] == level {
			return true
		}
	}
	return false
}

func doctorAdvice(checks []map[string]any) []string {
	var advice []string
	for _, check := range checks {
		id, _ := check["id"].(string)
		level, _ := check["level"].(string)
		if level != "error" && level != "warn" {
			continue
		}
		switch id {
		case "display_env":
			advice = append(advice, "Start KioskMate inside the graphical user session or fix the user service DISPLAY/WAYLAND_DISPLAY environment.")
		case "runtime_dir":
			advice = append(advice, "Run the service as the logged-in kiosk user and verify XDG_RUNTIME_DIR points to /run/user/<uid>.")
		case "browser_command":
			advice = append(advice, "Install Chromium or choose a valid custom browser command in Settings.")
		case "browser_profile":
			advice = append(advice, "Repair or reset the browser profile directory permissions.")
		case "active_page":
			advice = append(advice, "Check the Home Assistant URL, network route and ip_bans.yaml on the HA host.")
		case "browser_running":
			advice = append(advice, "Use the browser log tail in this report; if it exits immediately, try Safe Mode or a fresh browser profile.")
		}
	}
	if len(advice) == 0 {
		advice = append(advice, "No blocking issue found. If the display is still blank, run Render Check and inspect Home Assistant theme/session settings.")
	}
	return advice
}

func (s *Server) browserRenderCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Index int    `json:"index"`
		URL   string `json:"url"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	target := strings.TrimSpace(body.URL)
	if target == "" {
		urls := s.cfg.Snapshot().Kiosk.PageURLs()
		if body.Index < 0 || body.Index >= len(urls) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "page index out of range"})
			return
		}
		target = urls[body.Index]
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only http and https URLs can be rendered"})
		return
	}
	status := s.browser.Status()
	command := status.Command
	if command == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "browser command is empty"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	var result renderSnapshotResult
	var err error
	if status.Running && target == status.URL && status.DevTools {
		result.PNG, err = s.browser.CaptureScreenshot(ctx)
		result.Analysis = analyzePNG(result.PNG)
	} else {
		result, err = renderPageSnapshot(ctx, command, target, 1024, 768)
	}
	if err != nil {
		result.Error = err.Error()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          err == nil && result.Analysis["visible"].(bool),
		"url":         target,
		"command":     command,
		"latency_ms":  result.LatencyMS,
		"screenshot":  "render.png",
		"analysis":    result.Analysis,
		"output_tail": result.OutputTail,
		"error":       result.Error,
	})
}

func (s *Server) browserSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	status := s.browser.Status()
	target := strings.TrimSpace(r.URL.Query().Get("url"))
	if target == "" {
		target = status.URL
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only http and https URLs can be rendered"})
		return
	}
	refresh := r.URL.Query().Get("refresh") == "1"
	s.snapshot.mu.Lock()
	defer s.snapshot.mu.Unlock()
	if !refresh && len(s.snapshot.png) > 0 && s.snapshot.url == target {
		writeSnapshot(w, s.snapshot.png, s.snapshot.time)
		return
	}
	if !refresh {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no cached snapshot; request refresh=1 explicitly"})
		return
	}
	if !status.Running || !status.DevTools {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "the running Chromium session is not connected to DevTools"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	pngData, err := s.browser.CaptureScreenshot(ctx)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	s.snapshot.png = append(s.snapshot.png[:0], pngData...)
	s.snapshot.url = target
	s.snapshot.time = time.Now()
	writeSnapshot(w, s.snapshot.png, s.snapshot.time)
}

func writeSnapshot(w http.ResponseWriter, pngData []byte, created time.Time) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=15")
	w.Header().Set("X-KioskMate-Snapshot-Time", created.UTC().Format(time.RFC3339))
	_, _ = w.Write(pngData)
}

type renderSnapshotResult struct {
	PNG        []byte
	Analysis   map[string]any
	OutputTail string
	Error      string
	LatencyMS  int64
}

func renderPageSnapshot(ctx context.Context, command string, target string, width int, height int) (renderSnapshotResult, error) {
	tmp, err := os.MkdirTemp("", "kioskmate-render-*")
	if err != nil {
		return renderSnapshotResult{}, err
	}
	defer os.RemoveAll(tmp)
	screenshot := filepath.Join(tmp, "render.png")
	profile := filepath.Join(tmp, "profile")
	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--hide-scrollbars",
		"--no-first-run",
		fmt.Sprintf("--window-size=%d,%d", width, height),
		"--user-data-dir=" + profile,
		"--screenshot=" + screenshot,
		target,
	}
	start := time.Now()
	out, err := exec.CommandContext(ctx, command, args...).CombinedOutput()
	analysis := analyzeScreenshot(screenshot)
	pngData, readErr := os.ReadFile(screenshot)
	result := renderSnapshotResult{
		PNG:        pngData,
		Analysis:   analysis,
		OutputTail: tailText(string(out), 2000),
		LatencyMS:  time.Since(start).Milliseconds(),
	}
	if err != nil {
		return result, err
	}
	if readErr != nil {
		return result, readErr
	}
	return result, nil
}

func analyzeScreenshot(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{"visible": false, "error": err.Error()}
	}
	return analyzePNG(data)
}

func analyzePNG(data []byte) map[string]any {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return map[string]any{"visible": false, "error": err.Error()}
	}
	bounds := img.Bounds()
	total := bounds.Dx() * bounds.Dy()
	if total <= 0 {
		return map[string]any{"visible": false, "error": "empty image"}
	}
	blank := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 4 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 4 {
			r, g, b, _ := img.At(x, y).RGBA()
			r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)
			if (r8 > 245 && g8 > 245 && b8 > 245) || (r8 < 8 && g8 < 8 && b8 < 8) {
				blank++
			}
		}
	}
	samplesX := (bounds.Dx() + 3) / 4
	samplesY := (bounds.Dy() + 3) / 4
	samples := max(1, samplesX*samplesY)
	blankRatio := float64(blank) / float64(samples)
	return map[string]any{
		"visible":     blankRatio < 0.985,
		"blank_ratio": blankRatio,
		"width":       bounds.Dx(),
		"height":      bounds.Dy(),
	}
}

func tailText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit > 0 && len(text) > limit {
		return text[len(text)-limit:]
	}
	return text
}

func pageCheckHint(target string, resp *http.Response) (string, string) {
	finalURL := ""
	if resp != nil && resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	lowerTarget := strings.ToLower(target + " " + finalURL)
	isHA := strings.Contains(lowerTarget, "8123") || strings.Contains(lowerTarget, "homeassistant") || strings.Contains(lowerTarget, "home-assistant")
	if resp.StatusCode == http.StatusForbidden {
		if isHA {
			return "home_assistant_forbidden", "Home Assistant returned 403. Remove the kiosk IP from ip_bans.yaml, restart Home Assistant if needed, then reset the KioskMate HA browser session."
		}
		return "forbidden", "The server returned 403 Forbidden. Check server ACLs, reverse proxy rules or IP bans."
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return "auth_required", "The page requires authentication. Open it on the kiosk display and sign in once, or reset the HA session if the login is broken."
	}
	if strings.Contains(strings.ToLower(finalURL), "/auth/") {
		return "auth_redirect", "The page redirects to authentication. This is normal before the kiosk browser has a valid Home Assistant session."
	}
	if resp.StatusCode >= 400 {
		return "http_error", "The page returned an HTTP error. Check the URL and backend service."
	}
	return "ok", "The kiosk host can reach this page."
}

func isHomeAssistantURL(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.Contains(lower, ":8123") || strings.Contains(lower, "homeassistant") || strings.Contains(lower, "home-assistant") || strings.Contains(lower, "/api/websocket")
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authenticated(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !s.validToken(r) && !sameOrigin(r) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "cross-origin request rejected"})
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
	s.pruneSessionsLocked(time.Now())
	session, ok := s.sessions[cookie.Value]
	persist := false
	if ok {
		now := time.Now()
		// Persist LastSeen at most every 5 minutes so active sessions keep a
		// fresh inactivity clock across restarts without rewriting disk on every request.
		if now.Sub(session.LastSeen) >= 5*time.Minute {
			persist = true
		}
		session.LastSeen = now
		s.sessions[cookie.Value] = session
	}
	s.mu.Unlock()
	if persist {
		s.persistSessions()
	}
	return ok
}

func (s *Server) validToken(r *http.Request) bool {
	token := r.Header.Get("X-KioskMate-Token")
	if token == "" {
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if token == "" || s.cfg.Snapshot().Admin.Token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.Snapshot().Admin.Token)) == 1
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
	s.pruneSessionsLocked(time.Now())
	s.sessions[id] = session
	for len(s.sessions) > 32 {
		var oldestID string
		var oldest time.Time
		for sessionID, item := range s.sessions {
			if oldestID == "" || item.LastSeen.Before(oldest) {
				oldestID = sessionID
				oldest = item.LastSeen
			}
		}
		delete(s.sessions, oldestID)
	}
	s.mu.Unlock()
	s.persistSessions()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    id,
		Path:     "/",
		MaxAge:   int((7 * 24 * time.Hour).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   requestIsHTTPS(r),
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
	s.attempts[key] = kept
	return len(kept) < 8
}

func (s *Server) recordFailedAttempt(r *http.Request) {
	s.mu.Lock()
	s.attempts[remoteIP(r)] = append(s.attempts[remoteIP(r)], time.Now())
	s.mu.Unlock()
}

func (s *Server) clearAttempts(r *http.Request) {
	s.mu.Lock()
	delete(s.attempts, remoteIP(r))
	s.mu.Unlock()
}

func (s *Server) pruneSessionsLocked(now time.Time) {
	for id, session := range s.sessions {
		if now.Sub(session.Created) > 7*24*time.Hour || now.Sub(session.LastSeen) > 24*time.Hour {
			delete(s.sessions, id)
		}
	}
}

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	const (
		memory      = 64 * 1024
		iterations  = 3
		parallelism = 2
		keyLength   = 32
	)
	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)
	return fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, iterations, parallelism, base64.RawURLEncoding.EncodeToString(salt), base64.RawURLEncoding.EncodeToString(hash)), nil
}

func VerifyPassword(password, encoded string) bool {
	if strings.HasPrefix(encoded, "argon2id$") {
		return verifyArgon2ID(password, encoded)
	}
	return verifyLegacyPassword(password, encoded)
}

func verifyArgon2ID(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" || parts[1] != "v=19" {
		return false
	}
	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[2], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil || memory < 8*1024 || memory > 256*1024 || iterations < 1 || iterations > 10 || parallelism < 1 || parallelism > 8 {
		return false
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(salt) < 16 {
		return false
	}
	want, err := base64.RawURLEncoding.DecodeString(parts[4])
	if err != nil || len(want) < 16 || len(want) > 64 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func verifyLegacyPassword(password, encoded string) bool {
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
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), payment=(), usb=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'; img-src 'self' blob: data:; script-src 'self'; style-src 'self'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func sameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && strings.EqualFold(parsed.Host, r.Host)
}

func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}
