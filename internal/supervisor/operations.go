package supervisor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

type Operation struct {
	ID       string     `json:"id"`
	Action   string     `json:"action"`
	State    string     `json:"state"`
	Started  time.Time  `json:"started"`
	Finished *time.Time `json:"finished,omitempty"`
	Error    string     `json:"error,omitempty"`
}

// The operation gate owns process changes; the state mutex only protects snapshots.
func (b *Browser) operate(ctx context.Context, action string, run func() error) error {
	b.operationOnce.Do(func() { b.operationGate = make(chan struct{}, 1) })
	select {
	case b.operationGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-b.operationGate }()
	if err := ctx.Err(); err != nil {
		return err
	}
	var id [16]byte
	_, _ = rand.Read(id[:])
	b.mu.Lock()
	b.operation = Operation{ID: hex.EncodeToString(id[:]), Action: action, State: "running", Started: time.Now()}
	b.mu.Unlock()
	err := run()
	now := time.Now()
	b.mu.Lock()
	b.operation.State = "succeeded"
	b.operation.Finished = &now
	if err != nil {
		b.operation.State = "failed"
		b.operation.Error = err.Error()
	}
	b.operationHistory = append(b.operationHistory, b.operation)
	if len(b.operationHistory) > 50 {
		b.operationHistory = b.operationHistory[len(b.operationHistory)-50:]
	}
	operation := b.operation
	journal := b.journal
	b.mu.Unlock()
	if journal != nil {
		status, message := operation.State, operation.State
		if err != nil {
			message = err.Error()
		}
		journal.RecordDuration("browser", operation.Action, status, message, now.Sub(operation.Started), map[string]string{"operation_id": operation.ID})
	}
	b.persistRuntimeState()
	return err
}

func (b *Browser) Operations() []Operation {
	b.mu.Lock()
	defer b.mu.Unlock()
	items := append([]Operation(nil), b.operationHistory...)
	if b.operation.State == "running" {
		items = append(items, b.operation)
	}
	return items
}

func (b *Browser) cancelRecoveryLocked() {
	b.recoveryEpoch++
	if b.recoveryCancel != nil {
		b.recoveryCancel()
		b.recoveryCancel = nil
	}
}

func (b *Browser) Start(ctx context.Context) error {
	return b.operate(ctx, "start", func() error {
		b.mu.Lock()
		b.manuallyStopped = false
		b.cancelRecoveryLocked()
		b.mu.Unlock()
		return b.start(ctx)
	})
}

func (b *Browser) Stop(ctx context.Context) error {
	b.mu.Lock()
	b.manuallyStopped = true
	b.cancelRecoveryLocked()
	b.mu.Unlock()
	return b.operate(ctx, "stop", func() error { return b.stop(ctx) })
}

func (b *Browser) Restart(ctx context.Context) error {
	return b.operate(ctx, "restart", func() error {
		b.mu.Lock()
		b.manuallyStopped = false
		b.cancelRecoveryLocked()
		b.mu.Unlock()
		return b.restart(ctx)
	})
}

func (b *Browser) Reload(ctx context.Context) error {
	return b.operate(ctx, "reload", func() error { return b.reload(ctx) })
}

func (b *Browser) SetActive(ctx context.Context, index int) error {
	return b.operate(ctx, "page", func() error { return b.setActive(ctx, index) })
}

func (b *Browser) ResetSession(ctx context.Context) error {
	return b.operate(ctx, "reset-session", func() error {
		b.mu.Lock()
		b.cancelRecoveryLocked()
		b.mu.Unlock()
		return b.resetSession(ctx)
	})
}
