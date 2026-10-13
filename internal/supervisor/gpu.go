package supervisor

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
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
	session, err := dialBrowserTarget(ctx, profile)
	if err != nil {
		return GPUInfo{}, err
	}
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
