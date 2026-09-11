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
	MemoryMetric      string     `json:"memory_metric"`
}

type TelemetryHistory struct {
	Summary TelemetrySummary  `json:"summary"`
	Samples []TelemetrySample `json:"samples"`
}

type SoakCheck struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Current any    `json:"current,omitempty"`
	Target  any    `json:"target,omitempty"`
}

type SoakReport struct {
	GeneratedAt       time.Time        `json:"generated_at"`
	StartedAt         time.Time        `json:"started_at"`
	DurationSeconds   int64            `json:"duration_seconds"`
	RequiredSeconds   int64            `json:"required_seconds"`
	Status            string           `json:"status"`
	Passed            bool             `json:"passed"`
	Starts            int              `json:"browser_starts"`
	Restarts          int              `json:"browser_restarts"`
	ExitCounts        map[string]int   `json:"exit_reason_counts"`
	DuplicateObserved bool             `json:"duplicate_browser_observed"`
	Summary           TelemetrySummary `json:"summary"`
	AuthGuard         AuthGuardStatus  `json:"auth_guard"`
	Recovery          RecoveryStatus   `json:"recovery"`
	Checks            []SoakCheck      `json:"checks"`
}

type ManualOverride struct {
	Active  bool       `json:"active"`
	Page    int        `json:"page"`
	Source  string     `json:"source,omitempty"`
	Started *time.Time `json:"started,omitempty"`
	Until   *time.Time `json:"until,omitempty"`
}

type runtimeState struct {
	RecoveryRuns       []time.Time       `json:"recovery_runs,omitempty"`
	StartCount         int               `json:"start_count"`
	RestartCount       int               `json:"restart_count"`
	Recovery           RecoveryStatus    `json:"recovery"`
	Override           ManualOverride    `json:"override"`
	Telemetry          []TelemetrySample `json:"telemetry"`
	Operations         []Operation       `json:"operations,omitempty"`
	LastExit           ExitStatus        `json:"last_exit,omitempty"`
	ExitCounts         map[string]int    `json:"exit_reason_counts,omitempty"`
	TelemetryStarted   time.Time         `json:"telemetry_started_at,omitempty"`
	TelemetryStarts    int               `json:"telemetry_start_baseline,omitempty"`
	TelemetryRestarts  int               `json:"telemetry_restart_baseline,omitempty"`
	TelemetryExits     map[string]int    `json:"telemetry_exit_baseline,omitempty"`
	TelemetryDuplicate bool              `json:"telemetry_duplicate_observed,omitempty"`
}

func (b *Browser) Recover(ctx context.Context, reason string) error {
	return b.operate(ctx, "recover", func() error {
		b.mu.Lock()
		b.manuallyStopped = false
		b.cancelRecoveryLocked()
		b.mu.Unlock()
		return b.recover(ctx, reason, false)
	})
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
	if b.manuallyStopped {
		b.mu.Unlock()
		return fmt.Errorf("browser was stopped manually")
	}
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
	cutoff := now.Add(-time.Hour)
	runs := b.recoveryRuns[:0]
	for _, at := range b.recoveryRuns {
		if at.After(cutoff) {
			runs = append(runs, at)
		}
	}
	b.recoveryRuns = runs
	if len(runs) >= 6 {
		until := runs[0].Add(time.Hour)
		b.recovery.State, b.recovery.Stage, b.recovery.BackoffUntil = "backoff", "restart_budget", &until
		b.mu.Unlock()
		b.persistRuntimeState()
		return fmt.Errorf("automatic recovery budget exhausted until %s", until.Format(time.RFC3339))
	}
	b.recoveryRuns = append(b.recoveryRuns, now)
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
		err = b.restart(ctx, normalizeExitReason(reason))
	} else {
		err = b.start(ctx)
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
	if until == nil || b.manuallyStopped || b.authGuard.Tripped {
		b.mu.Unlock()
		return
	}
	b.cancelRecoveryLocked()
	epoch := b.recoveryEpoch
	ctx, cancel := context.WithCancel(context.Background())
	b.recoveryCancel = cancel
	b.mu.Unlock()
	if until == nil {
		return
	}
	delay := time.Until(*until)
	if delay < 0 {
		delay = 0
	}
	go func() {
		defer cancel()
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return
		}
		runCtx, finish := context.WithTimeout(ctx, 30*time.Second)
		defer finish()
		err := b.operate(runCtx, "recover", func() error {
			b.mu.Lock()
			stale := b.recoveryEpoch != epoch || b.manuallyStopped
			b.mu.Unlock()
			if stale {
				return context.Canceled
			}
			return b.recover(runCtx, "unexpected browser exit", true)
		})
		if err != nil && ctx.Err() == nil {
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
	b.telemetryStarted = time.Now()
	b.telemetryStarts = b.startCount
	b.telemetryRestarts = b.restartCount
	b.telemetryExits = cloneIntMap(b.exitReasonCounts)
	b.telemetryDuplicate = false
	b.mu.Unlock()
	b.persistRuntimeState()
	return nil
}

func (b *Browser) SoakReport() SoakReport {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.soakReportLocked(time.Now())
}

func (b *Browser) soakReportLocked(now time.Time) SoakReport {
	const required = 24 * time.Hour
	started := b.telemetryStarted
	if started.IsZero() {
		started = now
	}
	duration := now.Sub(started)
	if duration < 0 {
		duration = 0
	}
	starts := maxRuntimeInt(0, b.startCount-b.telemetryStarts)
	restarts := maxRuntimeInt(0, b.restartCount-b.telemetryRestarts)
	exits := counterDelta(b.exitReasonCounts, b.telemetryExits)
	duplicate := b.telemetryDuplicate || len(b.duplicateRoots) > 0
	summary := b.telemetrySummaryLocked()
	expectedSamples := int(duration / telemetryInterval)
	if expectedSamples < 1 {
		expectedSamples = 1
	}
	minimumSamples := expectedSamples * 8 / 10
	if minimumSamples < 1 {
		minimumSamples = 1
	}
	checks := []SoakCheck{
		{ID: "duration", OK: duration >= required, Current: int64(duration.Seconds()), Target: int64(required.Seconds())},
		{ID: "samples", OK: len(b.telemetry) >= minimumSamples, Current: len(b.telemetry), Target: minimumSamples},
		{ID: "duplicates", OK: !duplicate, Current: duplicate, Target: false},
		{ID: "restarts", OK: restarts <= 1, Current: restarts, Target: 1},
		{ID: "auth_guard", OK: !b.authGuard.Tripped, Current: b.authGuard.Tripped, Target: false},
		{ID: "recovery_budget", OK: b.recovery.Stage != "restart_budget", Current: b.recovery.Stage, Target: "available"},
	}
	passed := true
	for _, check := range checks {
		passed = passed && check.OK
	}
	status := "collecting"
	if duration >= required {
		if passed {
			status = "passed"
		} else {
			status = "failed"
		}
	}
	return SoakReport{
		GeneratedAt: now, StartedAt: started, DurationSeconds: int64(duration.Seconds()), RequiredSeconds: int64(required.Seconds()),
		Status: status, Passed: passed, Starts: starts, Restarts: restarts, ExitCounts: exits,
		DuplicateObserved: duplicate, Summary: summary, AuthGuard: b.authGuard, Recovery: b.recovery, Checks: checks,
	}
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
	summary := TelemetrySummary{Samples: len(b.telemetry), BrowserStartCount: b.startCount, RestartCount: b.restartCount, MemoryMetric: "pss_or_rss_fallback"}
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
	state := runtimeState{
		RecoveryRuns: append([]time.Time(nil), b.recoveryRuns...), StartCount: b.startCount, RestartCount: b.restartCount,
		Recovery: b.recovery, Override: b.override, Telemetry: append([]TelemetrySample(nil), b.telemetry...),
		Operations: append([]Operation(nil), b.operationHistory...),
		LastExit:   b.lastExitDetails, ExitCounts: cloneIntMap(b.exitReasonCounts),
		TelemetryStarted: b.telemetryStarted, TelemetryStarts: b.telemetryStarts, TelemetryRestarts: b.telemetryRestarts,
		TelemetryExits: cloneIntMap(b.telemetryExits), TelemetryDuplicate: b.telemetryDuplicate,
	}
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
	b.lastExitDetails, b.exitReasonCounts = state.LastExit, cloneIntMap(state.ExitCounts)
	if b.exitReasonCounts == nil {
		b.exitReasonCounts = map[string]int{}
	}
	if state.LastExit.At != nil {
		b.lastExit = *state.LastExit.At
	}
	b.recoveryRuns = state.RecoveryRuns
	b.telemetryStarted, b.telemetryStarts, b.telemetryRestarts = state.TelemetryStarted, state.TelemetryStarts, state.TelemetryRestarts
	b.telemetryExits, b.telemetryDuplicate = cloneIntMap(state.TelemetryExits), state.TelemetryDuplicate
	b.operationHistory = state.Operations
	if len(b.operationHistory) > 50 {
		b.operationHistory = b.operationHistory[len(b.operationHistory)-50:]
	}
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
		return "access_denied", "Check Home Assistant and proxy access rules. If HA confirms an IP ban, remove only this kiosk's entry on the HA host and restart HA. Preserve the browser session first."
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

func maxRuntimeInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func counterDelta(current, baseline map[string]int) map[string]int {
	delta := map[string]int{}
	for key, value := range current {
		if difference := value - baseline[key]; difference > 0 {
			delta[key] = difference
		}
	}
	return delta
}
