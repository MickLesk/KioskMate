//go:build windows

package hardware

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// The Windows implementation reports only what it can actually read or
// control: monitor brightness through WMI (WmiMonitorBrightnessMethods,
// usually internal laptop or tablet panels), device data, memory, uptime,
// disk usage and battery level. Display power, audio, the on-screen keyboard,
// package upgrades, CPU load and temperature are reported as unsupported.

const powershellTimeout = 20 * time.Second

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetTickCount64       = kernel32.NewProc("GetTickCount64")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func (s *Service) Rediscover() {
	resetProbes()
	s.InvalidateCache()
}

func collectStatus(ctx context.Context) Status {
	probe, probeErr := runWindowsProbe(ctx)
	support := Support{
		DisplayBrightness: probeErr == nil && probe.brightnessSupported(),
		BatteryLevel:      probeErr == nil && probe.Battery != nil,
	}
	var brightness, battery any
	if support.DisplayBrightness {
		brightness = *probe.Brightness
	}
	if support.BatteryLevel {
		battery = *probe.Battery
	}
	brightnessCommand := ""
	if support.DisplayBrightness {
		brightnessCommand = "wmi"
	}
	system := map[string]any{
		"uptime_minutes":          uptimeMinutes(),
		"memory_size_gib":         memorySizeGiB(),
		"memory_usage_percent":    memoryUsagePercent(),
		"processor_usage_percent": nil,
		"processor_count":         runtime.NumCPU(),
		"processor_temperature_c": nil,
		"disk_usage":              diskUsage(),
		"battery_level":           battery,
		"illuminance_level":       nil,
		"package_upgrades":        nil,
	}
	return Status{
		Support: support,
		Session: map[string]string{
			"user":    userName(),
			"type":    "windows",
			"desktop": "windows",
		},
		Device: map[string]any{
			"model":         firstNonEmpty(probe.Model, "Generic"),
			"vendor":        firstNonEmpty(probe.Manufacturer, "Generic"),
			"serial_number": firstNonEmpty(probe.Serial, machineGUIDSuffix()),
			"host_name":     hostName(),
			"machine_id":    machineGUID(),
		},
		Network: map[string]any{
			"addresses": networkAddresses(),
			"primary":   primaryAddress(),
		},
		System:  system,
		Runtime: runtimeStatus(),
		Display: map[string]any{
			"power":      nil,
			"brightness": brightness,
			"command":    brightnessCommand,
			"backlight":  "",
		},
		Audio: map[string]any{
			"volume":     nil,
			"microphone": nil,
			"sink":       "",
			"source":     "",
		},
	}
}

func refreshFastSystem(system map[string]any) map[string]any {
	next := make(map[string]any, len(system)+2)
	for key, value := range system {
		next[key] = value
	}
	next["uptime_minutes"] = uptimeMinutes()
	next["memory_usage_percent"] = memoryUsagePercent()
	return next
}

func (s *Service) SetDisplay(ctx context.Context, power string) error {
	power = strings.ToUpper(strings.TrimSpace(power))
	if power != "ON" && power != "OFF" {
		return errors.New("display power must be ON or OFF")
	}
	return errors.New("display power control is unsupported on Windows")
}

func (s *Service) SetBrightness(ctx context.Context, percent int) error {
	if percent < 1 || percent > 100 {
		return errors.New("brightness must be between 1 and 100")
	}
	if _, err := runPowerShell(ctx, windowsBrightnessScript(percent)); err != nil {
		return errors.New("WMI brightness control failed: " + err.Error())
	}
	s.InvalidateCache()
	return nil
}

func (s *Service) SetAudioVolume(ctx context.Context, percent int, microphone bool) error {
	if percent < 0 || percent > 100 {
		return errors.New("volume must be between 0 and 100")
	}
	return errors.New("audio volume control is unsupported on Windows")
}

func (s *Service) SetKeyboard(ctx context.Context, power string) error {
	power = strings.ToUpper(strings.TrimSpace(power))
	if power != "ON" && power != "OFF" {
		return errors.New("keyboard power must be ON or OFF")
	}
	return errors.New("on-screen keyboard control is unsupported on Windows")
}

func runWindowsProbe(ctx context.Context) (windowsProbe, error) {
	output, err := runPowerShell(ctx, windowsProbeScript)
	if err != nil {
		return windowsProbe{}, err
	}
	return parseWindowsProbe(output)
}

// runPowerShell runs a script with Windows PowerShell 5.1, which ships with
// Windows 10 and 11, without opening a console window.
func runPowerShell(ctx context.Context, script string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, powershellTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "powershell.exe", powershellArgs(script)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			return "", err
		}
		return "", errors.New(firstLine(text))
	}
	return text, nil
}

func firstLine(value string) string {
	if idx := strings.IndexAny(value, "\r\n"); idx >= 0 {
		return value[:idx]
	}
	return value
}

func readMemoryStatus() (memoryStatusEx, bool) {
	status := memoryStatusEx{}
	status.Length = uint32(unsafe.Sizeof(status))
	if err := procGlobalMemoryStatusEx.Find(); err != nil {
		return status, false
	}
	ok, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	return status, ok != 0 && status.TotalPhys > 0
}

func memorySizeGiB() any {
	status, ok := readMemoryStatus()
	if !ok {
		return nil
	}
	return float64(status.TotalPhys) / 1024 / 1024 / 1024
}

func memoryUsagePercent() any {
	status, ok := readMemoryStatus()
	if !ok {
		return nil
	}
	return float64(status.TotalPhys-status.AvailPhys) / float64(status.TotalPhys) * 100
}

func uptimeMinutes() any {
	if err := procGetTickCount64.Find(); err != nil {
		return nil
	}
	ms, _, _ := procGetTickCount64.Call()
	return float64(ms) / 1000 / 60
}

// diskUsage reports the system drive, the Windows counterpart of "/".
func diskUsage() any {
	drive := firstNonEmpty(os.Getenv("SystemDrive"), "C:") + `\`
	name, err := windows.UTF16PtrFromString(drive)
	if err != nil {
		return nil
	}
	var available, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(name, &available, &total, &free); err != nil || total == 0 {
		return nil
	}
	used := float64(total - free)
	return map[string]any{
		"total_gb":     round1(float64(total) / 1024 / 1024 / 1024),
		"used_gb":      round1(used / 1024 / 1024 / 1024),
		"available_gb": round1(float64(available) / 1024 / 1024 / 1024),
		"percent":      round1(used / float64(total) * 100),
	}
}

func machineGUID() string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return ""
	}
	defer key.Close()
	value, _, err := key.GetStringValue("MachineGuid")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func machineGUIDSuffix() string {
	id := strings.ReplaceAll(machineGUID(), "-", "")
	if len(id) > 6 {
		return id[len(id)-6:]
	}
	return id
}
