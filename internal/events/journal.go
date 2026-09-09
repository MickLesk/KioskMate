package events

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultItems = 500
	maxLineBytes = 16 * 1024
	maxFileBytes = 2 << 20
	maxTextBytes = 512
)

// Event is a bounded, privacy-safe record of a user-visible runtime event.
// Callers should pass origins and identifiers, never credentials or URLs with
// query parameters.
type Event struct {
	ID        string            `json:"id"`
	At        time.Time         `json:"at"`
	Component string            `json:"component"`
	Action    string            `json:"action,omitempty"`
	Status    string            `json:"status"`
	Message   string            `json:"message,omitempty"`
	Duration  int64             `json:"duration_ms,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
}

type Journal struct {
	mu    sync.Mutex
	path  string
	limit int
	items []Event
	seq   atomic.Uint64
}

func Open(path string, limit int) (*Journal, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("event journal path is empty")
	}
	if limit <= 0 {
		limit = defaultItems
	}
	j := &Journal{path: path, limit: limit}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return j, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	_ = os.Chmod(path, 0o600)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), maxLineBytes)
	for scanner.Scan() {
		var event Event
		if json.Unmarshal(scanner.Bytes(), &event) == nil && event.Component != "" {
			j.items = append(j.items, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	j.trimLocked()
	return j, nil
}

func (j *Journal) Path() string {
	if j == nil {
		return ""
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.path
}

func (j *Journal) Record(component, action, status, message string, details map[string]string) Event {
	event := Event{
		ID:        j.nextID(),
		At:        time.Now().UTC(),
		Component: clean(component),
		Action:    clean(action),
		Status:    clean(status),
		Message:   clean(message),
		Details:   cleanDetails(details),
	}
	if event.Component == "" {
		event.Component = "runtime"
	}
	if event.Status == "" {
		event.Status = "info"
	}
	return j.record(event)
}

func (j *Journal) RecordDuration(component, action, status, message string, duration time.Duration, details map[string]string) Event {
	if j == nil {
		return Event{}
	}
	event := Event{
		ID:        j.nextID(),
		At:        time.Now().UTC(),
		Component: clean(component),
		Action:    clean(action),
		Status:    clean(status),
		Message:   clean(message),
		Duration:  duration.Milliseconds(),
		Details:   cleanDetails(details),
	}
	if event.Component == "" {
		event.Component = "runtime"
	}
	if event.Status == "" {
		event.Status = "info"
	}
	return j.record(event)
}

func (j *Journal) nextID() string {
	if j == nil {
		return ""
	}
	return "event-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.FormatUint(j.seq.Add(1), 10)
}

func (j *Journal) record(event Event) Event {
	if j == nil {
		return Event{}
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.items = append(j.items, event)
	j.trimLocked()
	j.appendLocked(event)
	return event
}

func (j *Journal) Recent(limit int) []Event {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if limit <= 0 || limit > len(j.items) {
		limit = len(j.items)
	}
	start := len(j.items) - limit
	result := append([]Event(nil), j.items[start:]...)
	for index := range result {
		result[index].Details = cloneDetails(result[index].Details)
	}
	return result
}

func (j *Journal) trimLocked() {
	if len(j.items) > j.limit {
		j.items = append([]Event(nil), j.items[len(j.items)-j.limit:]...)
	}
}

func (j *Journal) appendLocked(event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	file, err := os.OpenFile(j.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	_, _ = file.Write(append(data, '\n'))
	_ = file.Close()
	if info, err := os.Stat(j.path); err == nil && info.Size() > maxFileBytes {
		_ = os.Remove(j.path + ".1")
		_ = os.Rename(j.path, j.path+".1")
	}
}

func clean(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > maxTextBytes {
		value = value[:maxTextBytes]
	}
	return value
}

func cleanDetails(details map[string]string) map[string]string {
	if len(details) == 0 {
		return nil
	}
	result := make(map[string]string, len(details))
	for key, value := range details {
		key = clean(key)
		lowerKey := strings.ToLower(key)
		if key != "" && !strings.Contains(lowerKey, "password") && !strings.Contains(lowerKey, "token") && !strings.Contains(lowerKey, "secret") && !strings.Contains(lowerKey, "private_key") {
			result[key] = clean(value)
		}
	}
	return result
}

func cloneDetails(details map[string]string) map[string]string {
	if len(details) == 0 {
		return nil
	}
	return cleanDetails(details)
}
