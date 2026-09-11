package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

type cdpSession struct {
	conn    *websocket.Conn
	next    atomic.Int64
	mu      sync.Mutex
	writeMu sync.Mutex
	pending map[int64]chan cdpMessage
	bodyIDs map[string]struct{}
	events  chan cdpMessage
	done    chan struct{}
	cancel  context.CancelFunc
	err     error
	target  devToolsTarget
}

func newCDPSession(conn *websocket.Conn) *cdpSession {
	ctx, cancel := context.WithCancel(context.Background())
	s := &cdpSession{
		conn: conn, pending: make(map[int64]chan cdpMessage), bodyIDs: make(map[string]struct{}),
		events: make(chan cdpMessage, 256), done: make(chan struct{}), cancel: cancel,
	}
	go s.read(ctx)
	return s
}

func (s *cdpSession) close() { s.cancel(); _ = s.conn.CloseNow() }

func (s *cdpSession) read(ctx context.Context) {
	defer close(s.done)
	for {
		_, data, err := s.conn.Read(ctx)
		if err != nil {
			s.mu.Lock()
			s.err = err
			s.mu.Unlock()
			return
		}
		var message cdpMessage
		if json.Unmarshal(data, &message) != nil {
			continue
		}
		if message.ID != 0 {
			s.mu.Lock()
			response := s.pending[message.ID]
			s.mu.Unlock()
			if response != nil {
				select {
				case response <- message:
				default:
				}
			}
			continue
		}
		if !s.interestingEvent(message) {
			continue
		}
		// Fail visibly instead of silently discarding authentication events under load.
		select {
		case s.events <- message:
		default:
			s.mu.Lock()
			s.err = errors.New("DevTools event queue overflow")
			s.mu.Unlock()
			_ = s.conn.CloseNow()
			return
		}
	}
}

func (s *cdpSession) interestingEvent(event cdpMessage) bool {
	switch event.Method {
	case "Page.loadEventFired", "Page.lifecycleEvent":
		return true
	case "Network.webSocketCreated":
		return true
	case "Network.loadingFinished":
		var params struct {
			ID string `json:"requestId"`
		}
		if json.Unmarshal(event.Params, &params) != nil || params.ID == "" {
			return false
		}
		s.mu.Lock()
		_, ok := s.bodyIDs[params.ID]
		if ok {
			delete(s.bodyIDs, params.ID)
		}
		s.mu.Unlock()
		return ok
	case "Runtime.consoleAPICalled":
		_, ok := parseThemeConsoleEvent(event.Params)
		return ok
	case "Network.webSocketFrameReceived":
		var frame struct {
			Response struct {
				Payload string `json:"payloadData"`
			} `json:"response"`
		}
		return json.Unmarshal(event.Params, &frame) == nil && homeAssistantAuthFailureFrame(frame.Response.Payload)
	case "Network.responseReceived":
		var response struct {
			ID       string `json:"requestId"`
			Response struct {
				Status int    `json:"status"`
				URL    string `json:"url"`
			} `json:"response"`
		}
		if json.Unmarshal(event.Params, &response) != nil || (response.Response.Status != 400 && response.Response.Status != 401 && response.Response.Status != 403) {
			return false
		}
		if response.Response.Status == 400 && isTokenEndpoint(response.Response.URL) && response.ID != "" {
			s.mu.Lock()
			if len(s.bodyIDs) < 128 {
				s.bodyIDs[response.ID] = struct{}{}
			}
			s.mu.Unlock()
		}
		return true
	default:
		return false
	}
}

func isTokenEndpoint(raw string) bool {
	path := strings.SplitN(raw, "?", 2)[0]
	return strings.HasSuffix(path, "/auth/token") || strings.HasSuffix(path, "/auth/token/")
}

func (s *cdpSession) command(ctx context.Context, method string, params any, result any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	id := s.next.Add(1)
	response := make(chan cdpMessage, 1)
	s.mu.Lock()
	s.pending[id] = response
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.pending, id); s.mu.Unlock() }()
	payload, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	err = s.conn.Write(ctx, websocket.MessageText, payload)
	s.writeMu.Unlock()
	if err != nil {
		return err
	}
	select {
	case message := <-response:
		if message.Error != nil {
			return fmt.Errorf("DevTools %s failed: %s", method, message.Error.Message)
		}
		if result != nil && len(message.Result) != 0 {
			return json.Unmarshal(message.Result, result)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("DevTools %s: %w", method, ctx.Err())
	case <-s.done:
		s.mu.Lock()
		err := s.err
		s.mu.Unlock()
		if err == nil {
			err = errors.New("connection closed")
		}
		return fmt.Errorf("DevTools disconnected: %w", err)
	}
}
