package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/MickLesk/KioskMate/internal/events"
	"github.com/MickLesk/KioskMate/internal/integration"
	"github.com/MickLesk/KioskMate/internal/updater"
)

// streamInterval is how often /api/stream pushes status while an Admin page
// is open. Nothing is sampled for the stream when no client is connected.
var streamInterval = 5 * time.Second

// stream serves Server-Sent Events: a "status" event with the same payload as
// /api/status (hardware from its cache) and an "events" event with journal
// entries recorded since the previous push. The Admin UI uses it instead of
// re-fetching every endpoint, and stops it while the tab is hidden.
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	controller := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	// Tell EventSource to wait before reconnecting after a server restart.
	if _, err := fmt.Fprint(w, "retry: 5000\n\n"); err != nil {
		return
	}
	lastEvent := ""
	send := func() bool {
		if !s.authenticated(r) {
			_, _ = fmt.Fprint(w, "event: auth\ndata: {}\n\n")
			_ = controller.Flush()
			return false
		}
		if err := writeSSE(w, "status", s.streamStatus(r.Context())); err != nil {
			return false
		}
		if s.journal != nil {
			fresh, newest := eventsSince(s.journal.Recent(50), lastEvent)
			if newest != "" {
				lastEvent = newest
			}
			if len(fresh) > 0 {
				if err := writeSSE(w, "events", fresh); err != nil {
					return false
				}
			}
		}
		return controller.Flush() == nil
	}
	if !send() {
		return
	}
	ticker := time.NewTicker(streamInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !send() {
				return
			}
		}
	}
}

func (s *Server) streamStatus(ctx context.Context) map[string]any {
	cfg := s.cfg.Snapshot()
	mqttStatus := integration.MQTTConnectionStatus{State: "unavailable"}
	if s.mqtt != nil {
		mqttStatus = s.mqtt.ConnectionStatus()
	}
	updateStatus := updater.ReleaseInfo{CurrentVersion: s.version}
	if s.updater != nil {
		updateStatus = s.updater.Status()
	}
	status := map[string]any{
		"mqtt":   mqttStatus,
		"update": updateStatus,
		"config": statusConfig(cfg),
	}
	if s.browser != nil {
		status["browser"] = s.browser.Status()
	}
	if s.hardware != nil {
		hwCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		status["hardware"] = s.hardware.Status(hwCtx)
		cancel()
	}
	return status
}

// eventsSince returns the events recorded after the event with id last (all of
// them on the first call or when last fell out of the window), oldest first as
// Journal.Recent returns them, plus the newest id.
func eventsSince(recent []events.Event, last string) ([]events.Event, string) {
	if len(recent) == 0 {
		return nil, ""
	}
	newest := recent[len(recent)-1].ID
	for index, event := range recent {
		if event.ID == last {
			return recent[index+1:], newest
		}
	}
	return recent, newest
}

func writeSSE(w http.ResponseWriter, event string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	return err
}
