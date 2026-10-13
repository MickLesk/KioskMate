package hardware

// This file holds the platform-independent part of the hardware service. The
// probes and controls live in hardware.go (Linux tools) and
// hardware_windows.go.

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Service struct {
	mu       sync.Mutex
	cached   Status
	cachedAt time.Time
	fastAt   time.Time
}

// Status is split into a slow part (display, audio, disk, network, session),
// which shells out to external tools and is refreshed every slowStatusTTL or
// after an own control command, and a fast part read from /proc and /sys.
const (
	slowStatusTTL = 2 * time.Minute
	fastStatusTTL = 5 * time.Second
)

type Status struct {
	Support Support           `json:"support"`
	Session map[string]string `json:"session"`
	Device  map[string]any    `json:"device"`
	Network map[string]any    `json:"network"`
	System  map[string]any    `json:"system"`
	Runtime map[string]any    `json:"runtime"`
	Display map[string]any    `json:"display"`
	Audio   map[string]any    `json:"audio"`
}

type Support struct {
	SudoRights           bool `json:"sudo_rights"`
	DisplayStatus        bool `json:"display_status"`
	DisplayBrightness    bool `json:"display_brightness"`
	AudioVolume          bool `json:"audio_volume"`
	MicrophoneVolume     bool `json:"microphone_volume"`
	KeyboardVisibility   bool `json:"keyboard_visibility"`
	BatteryLevel         bool `json:"battery_level"`
	IlluminanceLevel     bool `json:"illuminance_level"`
	PackageUpgrades      bool `json:"package_upgrades"`
	ProcessorTemperature bool `json:"processor_temperature"`
}

func New() *Service {
	return &Service{}
}

func (s *Service) Status(ctx context.Context) Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.cachedAt.IsZero() || now.Sub(s.cachedAt) >= slowStatusTTL {
		s.cached = collectStatus(ctx)
		s.cachedAt = now
		s.fastAt = now
	} else if now.Sub(s.fastAt) >= fastStatusTTL {
		s.cached.System = refreshFastSystem(s.cached.System)
		s.cached.Runtime = runtimeStatus()
		s.fastAt = now
	}
	return s.cached
}

// InvalidateCache forces the next Status call to re-read display, audio and
// system values, e.g. after a control command changed them.
func (s *Service) InvalidateCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cachedAt = time.Time{}
}

func runtimeStatus() map[string]any {
	return map[string]any{
		"go":         runtime.Version(),
		"goos":       runtime.GOOS,
		"goarch":     runtime.GOARCH,
		"goroutines": runtime.NumGoroutine(),
	}
}

func networkAddresses() map[string]map[string][]string {
	out := map[string]map[string][]string{}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			family := "IPv6"
			value := ip.String()
			if ip4 := ip.To4(); ip4 != nil {
				family = "IPv4"
				value = ip4.String()
			}
			name := strings.ToUpper(iface.Name[:1]) + iface.Name[1:]
			if out[name] == nil {
				out[name] = map[string][]string{}
			}
			out[name][family] = append(out[name][family], value)
		}
	}
	return out
}

func primaryAddress() string {
	for _, families := range networkAddresses() {
		if values := families["IPv4"]; len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func userName() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return os.Getenv("USERNAME")
}

func hostName() string {
	name, _ := os.Hostname()
	return name
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func round1(value float64) float64 {
	return float64(int(value*10+0.5)) / 10
}

func MarshalStatus(status Status) []byte {
	data, _ := json.Marshal(status)
	return data
}
