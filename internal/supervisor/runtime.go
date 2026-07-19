package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/system"
)

const (
	telemetryRetention = 24 * time.Hour
	telemetryInterval  = time.Minute
	stableRuntime      = 10 * time.Minute
)

type RecoveryStatus struct {
	State        string     `json:"state"`
	Stage        string     `json:"stage,omitempty"`
	Reason       string     `json:"reason,omitempty"`
	Attempts     int        `json:"attempts"`
	LastAction   string     `json:"last_action,omitempty"`
	LastResult   string     `json:"last_result,omitempty"`
	LastAt       *time.Time `json:"last_at,omitempty"`
	BackoffUntil *time.Time `json:"backoff_until,omitempty"`
}

type TelemetrySample struct {
	At           time.Time `json:"at"`
	CPUPercent   float64   `json:"cpu_percent"`
	RSSMB        uint64    `json:"rss_mb"`
	ProcessCount int       `json:"process_count"`
}

type TelemetrySummary struct {
	Samples           int        `json:"samples"`
	WindowStart       *time.Time `json:"window_start,omitempty"`
	WindowEnd         *time.Time `json:"window_end,omitempty"`
	CPUAverage        float64    `json:"cpu_average"`
	CPUMaximum        float64    `json:"cpu_maximum"`
	RSSAverageMB      float64    `json:"rss_average_mb"`
	RSSMaximumMB      uint64     `json:"rss_maximum_mb"`
	ProcessMaximum    int        `json:"process_maximum"`
	BrowserStartCount int        `json:"browser_start_count"`
	RestartCount      int        `json:"restart_count"`
}

type TelemetryHistory struct {
	Summary TelemetrySummary  `json:"summary"`
	Samples []TelemetrySample `json:"samples"`
}

type ManualOverride struct {
	Active  bool       `json:"active"`
	Page    int        `json:"page"`
	Source  string     `json:"source,omitempty"`
	Started *time.Time `json:"started,omitempty"`
	Until   *time.Time `json:"until,omitempty"`
}

type runtimeState struct {
	StartCount   int               `json:"start_count"`
	RestartCount int               `json:"restart_count"`
	Recovery     RecoveryStatus    `json:"recovery"`
	Override     ManualOverride    `json:"override"`
	Telemetry    []TelemetrySample `json:"telemetry"`
}

func (b *Browser) Recover(ctx context.Context, reason string) error {
	return b.recover(ctx, reason, false)
}

func (b *Browser) setStartFailureLocked(err error) {
	now := time.Now()
	b.recovery.State = "failed"
	b.recovery.Stage = "start"
	b.recovery.Reason = "browser start failed"
	b.recovery.LastAction = "start"
	b.recovery.LastResult = err.Error()
	b.recovery.LastAt = &now
}

func (b *Browser) recover(ctx context.Context, reason string, forceRestart bool) error {
	now := time.Now()
	b.mu.Lock()
	if b.authGuard.Tripped {
		err := fmt.Errorf("authentication guard is active: %s", b.authGuard.Reason)
		b.recovery = RecoveryStatus{State: "auth_blocked", Stage: "authentication", Reason: reason, LastResult: err.Error(), LastAt: &now}
		b.mu.Unlock()
		b.persistRuntimeState()
		return err
	}
	if b.recovery.BackoffUntil != nil && now.Before(*b.recovery.BackoffUntil) {
		until := *b.recovery.BackoffUntil
		b.mu.Unlock()
		return fmt.Errorf("automatic recovery is in backoff until %s", until.Format(time.RFC3339))
	}
	running := b.cmd != nil && b.cmd.Process != nil
	b.recovery.State = "recovering"
	b.recovery.Stage = "reload"
	b.recovery.Reason = strings.TrimSpace(reason)
	b.recovery.LastAction = "reload"
	b.recovery.LastAt = &now
	b.mu.Unlock()

	if running && !forceRestart {
		if err := b.reloadDevTools(ctx); err == nil {
			b.finishRecovery("reload", "page reloaded", false)
			return nil
		}
	}
	b.mu.Lock()
	b.recovery.Stage = "restart"
	b.recovery.LastAction = "restart"
	b.mu.Unlock()
	var err error
	if running {
		err = b.Restart(ctx)
	} else {
		err = b.Start(ctx)
	}
	if err != nil {
		b.finishRecovery("restart", err.Error(), true)
		return err
	}
	b.finishRecovery("restart", "browser running", false)
	return nil
}

func (b *Browser) finishRecovery(action, result string, failed bool) {
	now := time.Now()
	b.mu.Lock()
	b.recovery.LastAction = action
	b.recovery.LastResult = result
	b.recovery.LastAt = &now
	if failed {
		b.recovery.State = "failed"
		b.recovery.Attempts++
		until := now.Add(recoveryDelay(b.recovery.Attempts))
		b.recovery.BackoffUntil = &until
	} else {
		b.recovery.State = "healthy"
		b.recovery.Stage = "running"
		b.recovery.BackoffUntil = nil
	}
	b.mu.Unlock()
	b.persistRuntimeState()
}

func (b *Browser) prepareUnexpectedExitRecoveryLocked(runtime time.Duration, lastError string) {
	now := time.Now()
	if runtime >= stableRuntime {
		b.recovery.Attempts = 0
	}
	b.recovery.Attempts++
	until := now.Add(recoveryDelay(b.recovery.Attempts))
	b.recovery.State = "backoff"
	b.recovery.Stage = "unexpected_exit"
	b.recovery.Reason = firstRuntimeString(lastError, "browser exited unexpectedly")
	b.recovery.LastAction = "automatic restart"
	b.recovery.LastResult = fmt.Sprintf("restart scheduled after %s", until.Sub(now).Round(time.Second))
	b.recovery.LastAt = &now
	b.recovery.BackoffUntil = &until
}

func (b *Browser) scheduleUnexpectedExitRecovery() {
	b.mu.Lock()
	until := b.recovery.BackoffUntil
	b.mu.Unlock()
	if until == nil {
		return
	}
	delay := time.Until(*until)
	if delay < 0 {
		delay = 0
	}
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		<-timer.C
		time.Sleep(100 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := b.recover(ctx, "unexpected browser exit", true); err != nil {
			b.logger.Warn("automatic browser recovery failed", "error", err)
		}
	}()
}

func recoveryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 5 * time.Second * time.Duration(1<<minRuntimeInt(attempt-1, 7))
	if delay > 10*time.Minute {
		return 10 * time.Minute
	}
	return delay
}

func (b *Browser) SetOverride(ctx context.Context, page int, duration time.Duration, source string) error {
	if duration <= 0 {
		duration = time.Hour
	}
	if duration > 24*time.Hour {
		duration = 24 * time.Hour
	}
	if err := b.SetActive(ctx, page); err != nil {
		return err
	}
	now := time.Now()
	until := now.Add(duration)
	b.mu.Lock()
	b.override = ManualOverride{Active: true, Page: page, Source: firstRuntimeString(strings.TrimSpace(source), "admin"), Started: &now, Until: &until}
	b.scheduler = SchedulerStatus{Enabled: true, Mode: "override", Reason: "manual override", NextSwitch: &until}
	b.mu.Unlock()
	b.persistRuntimeState()
	return nil
}

func (b *Browser) ClearOverride() error {
	b.mu.Lock()
	b.override = ManualOverride{}
	b.rotationUntil = time.Time{}
	b.mu.Unlock()
	b.persistRuntimeState()

	target, status := b.schedulerTarget(time.Now())
	b.mu.Lock()
	b.scheduler = status
	b.mu.Unlock()
	if target >= 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := b.SetActive(ctx, target); err != nil {
			b.logger.Warn("clear override immediate switch failed", "error", err)
		}
	}
	return nil
}

func (b *Browser) Telemetry() TelemetryHistory {
	b.mu.Lock()
	defer b.mu.Unlock()
	samples := append([]TelemetrySample(nil), b.telemetry...)
	return TelemetryHistory{Summary: b.telemetrySummaryLocked(), Samples: samples}
}

func (b *Browser) ResetTelemetry() error {
	b.mu.Lock()
	b.telemetry = nil
	b.mu.Unlock()
	b.persistRuntimeState()
	return nil
}

func (b *Browser) recordTelemetryLocked(stats system.ProcessTreeStats) bool {
	now := time.Now()
	if len(b.telemetry) > 0 && now.Sub(b.telemetry[len(b.telemetry)-1].At) < telemetryInterval {
		return false
	}
	cutoff := now.Add(-telemetryRetention)
	kept := b.telemetry[:0]
	for _, sample := range b.telemetry {
		if sample.At.After(cutoff) {
			kept = append(kept, sample)
		}
	}
	b.telemetry = append(kept, TelemetrySample{At: now, CPUPercent: stats.CPUPercent, RSSMB: stats.RSSMB, ProcessCount: len(stats.PIDs)})
	return true
}

func (b *Browser) telemetrySummaryLocked() TelemetrySummary {
	summary := TelemetrySummary{Samples: len(b.telemetry), BrowserStartCount: b.startCount, RestartCount: b.restartCount}
	if len(b.telemetry) == 0 {
		return summary
	}
	start, end := b.telemetry[0].At, b.telemetry[len(b.telemetry)-1].At
	summary.WindowStart, summary.WindowEnd = &start, &end
	var cpuTotal float64
	var rssTotal uint64
	for _, sample := range b.telemetry {
		cpuTotal += sample.CPUPercent
		rssTotal += sample.RSSMB
		if sample.CPUPercent > summary.CPUMaximum {
			summary.CPUMaximum = sample.CPUPercent
		}
		if sample.RSSMB > summary.RSSMaximumMB {
			summary.RSSMaximumMB = sample.RSSMB
		}
		if sample.ProcessCount > summary.ProcessMaximum {
			summary.ProcessMaximum = sample.ProcessCount
		}
	}
	summary.CPUAverage = cpuTotal / float64(len(b.telemetry))
	summary.RSSAverageMB = float64(rssTotal) / float64(len(b.telemetry))
	return summary
}

func (b *Browser) runtimeStatePath() string {
	return filepath.Join(config.ConfigDir(b.cfg.Path), "runtime-state.json")
}

func (b *Browser) persistRuntimeState() {
	if b.cfg.Path == "" {
		return
	}
	b.persistMu.Lock()
	defer b.persistMu.Unlock()
	b.mu.Lock()
	state := runtimeState{StartCount: b.startCount, RestartCount: b.restartCount, Recovery: b.recovery, Override: b.override, Telemetry: append([]TelemetrySample(nil), b.telemetry...)}
	b.mu.Unlock()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(b.runtimeStatePath()), 0o700)
	temporary := b.runtimeStatePath() + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err == nil {
		if err := os.Rename(temporary, b.runtimeStatePath()); err != nil {
			_ = os.Remove(b.runtimeStatePath())
			_ = os.Rename(temporary, b.runtimeStatePath())
		}
	}
}

func (b *Browser) loadRuntimeState() {
	if b.cfg.Path == "" {
		return
	}
	data, err := os.ReadFile(b.runtimeStatePath())
	if err != nil {
		return
	}
	var state runtimeState
	if json.Unmarshal(data, &state) != nil {
		return
	}
	b.startCount, b.restartCount, b.recovery, b.override = state.StartCount, state.RestartCount, state.Recovery, state.Override
	cutoff := time.Now().Add(-telemetryRetention)
	for _, sample := range state.Telemetry {
		if sample.At.After(cutoff) {
			b.telemetry = append(b.telemetry, sample)
		}
	}
	if b.override.Until != nil && time.Now().After(*b.override.Until) {
		b.override = ManualOverride{}
	}
}

func classifyAuthGuard(reason string) (string, string) {
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(lower, "403") || strings.Contains(lower, "forbidden") || strings.Contains(lower, "ip ban"):
		return "ip_ban", "Remove the kiosk IP from Home Assistant ip_bans.yaml, restart Home Assistant, then reset the KioskMate HA session."
	case strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "token"):
		return "credentials", "Reset the KioskMate HA session and sign in again with valid Home Assistant credentials."
	default:
		return "authentication", "Reset the KioskMate HA session and sign in to Home Assistant again."
	}
}

func localKioskIP() string {
	conn, err := net.DialTimeout("udp", "1.1.1.1:53", time.Second)
	if err != nil {
		return ""
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return ""
}

func firstRuntimeString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func minRuntimeInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
