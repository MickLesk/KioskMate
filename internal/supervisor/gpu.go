package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// GPUInfo is the subset of Chromium's chrome://gpu report that tells whether
// rendering is hardware accelerated on the kiosk.
type GPUInfo struct {
	Vendor          string            `json:"vendor,omitempty"`
	Renderer        string            `json:"renderer,omitempty"`
	Driver          string            `json:"driver,omitempty"`
	FeatureStatus   map[string]string `json:"feature_status"`
	SoftwareRaster  bool              `json:"software_raster"`
	SoftwareFeature []string          `json:"software_features,omitempty"`
}

// GPUInfo queries SystemInfo.getInfo on Chromium's browser target. The page
// target used for kiosk control does not expose the SystemInfo domain.
func (b *Browser) GPUInfo(ctx context.Context) (GPUInfo, error) {
	cfg := b.cfg.Snapshot()
	b.mu.Lock()
	running := b.cmd != nil && b.cmd.Process != nil
	profile := browserUserDataDir(cfg, b.active)
	b.mu.Unlock()
	if !running {
		return GPUInfo{}, errors.New("browser is not running")
	}
	if !supportsCDP(cfg.Kiosk.BrowserPreset) {
		return GPUInfo{}, errors.New("GPU report requires a Chromium browser preset")
	}
	return readGPUInfo(ctx, profile)
}

func readGPUInfo(ctx context.Context, profile string) (GPUInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
	if err != nil {
		return GPUInfo{}, err
	}
	lines := strings.Fields(string(data))
	if len(lines) < 2 {
		return GPUInfo{}, errors.New("DevToolsActivePort has no browser target")
	}
	port, err := strconv.Atoi(lines[0])
	if err != nil || port < 1 || port > 65535 || !strings.HasPrefix(lines[1], "/devtools/browser/") {
		return GPUInfo{}, errors.New("DevToolsActivePort is invalid")
	}
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://127.0.0.1:%d%s", port, lines[1]), &websocket.DialOptions{
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	})
	if err != nil {
		return GPUInfo{}, fmt.Errorf("connect to Chromium browser target: %w", err)
	}
	conn.SetReadLimit(8 << 20)
	session := newCDPSession(conn)
	defer session.close()
	var result systemInfoResult
	if err := session.command(ctx, "SystemInfo.getInfo", map[string]any{}, &result); err != nil {
		return GPUInfo{}, err
	}
	return result.gpuInfo(), nil
}

type systemInfoResult struct {
	GPU struct {
		Devices []struct {
			VendorString  string `json:"vendorString"`
			DeviceString  string `json:"deviceString"`
			DriverVendor  string `json:"driverVendor"`
			DriverVersion string `json:"driverVersion"`
		} `json:"devices"`
		FeatureStatus map[string]string `json:"featureStatus"`
	} `json:"gpu"`
}

func (r systemInfoResult) gpuInfo() GPUInfo {
	info := GPUInfo{FeatureStatus: r.GPU.FeatureStatus}
	if info.FeatureStatus == nil {
		info.FeatureStatus = map[string]string{}
	}
	if len(r.GPU.Devices) > 0 {
		device := r.GPU.Devices[0]
		info.Vendor, info.Renderer = device.VendorString, device.DeviceString
		info.Driver = strings.TrimSpace(device.DriverVendor + " " + device.DriverVersion)
	}
	for feature, status := range info.FeatureStatus {
		if strings.Contains(status, "software") || strings.HasPrefix(status, "disabled") || status == "unavailable_off" {
			info.SoftwareFeature = append(info.SoftwareFeature, feature)
		}
	}
	sort.Strings(info.SoftwareFeature)
	raster := info.FeatureStatus["rasterization"]
	info.SoftwareRaster = raster != "" && !strings.HasPrefix(raster, "enabled")
	return info
}
