//go:build !windows

package hardware

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Rediscover drops all memoized capability probes (sudo rights, ddcutil
// features, display tool, session type) and the status cache.
func (s *Service) Rediscover() {
	resetProbes()
	diskUsageMu.Lock()
	diskUsageAt = time.Time{}
	diskUsageMu.Unlock()
	s.InvalidateCache()
}

// refreshFastSystem returns a copy of system with the cheap /proc and /sys
// values updated. A copy is required because earlier Status results may still
// be read by other goroutines.
func refreshFastSystem(system map[string]any) map[string]any {
	next := make(map[string]any, len(system)+4)
	for key, value := range system {
		next[key] = value
	}
	next["uptime_minutes"] = uptimeMinutes()
	next["memory_usage_percent"] = memoryUsagePercent()
	next["processor_usage_percent"] = processorUsagePercent()
	next["load_average"] = loadAverage()
	next["processor_temperature_c"] = processorTemperature()
	next["battery_level"] = batteryLevel()
	next["illuminance_level"] = illuminanceLevel()
	next["touch_devices"] = touchDeviceNames()
	throttled := ""
	if power, ok := system["power"].(map[string]any); ok {
		if flags, ok := power["throttled"].(string); ok {
			throttled = "throttled=" + flags
		}
	}
	next["power"] = powerStatus(undervoltageAlarm(), throttled)
	return next
}

func touchDeviceNames() []string {
	file, err := os.Open("/proc/bus/input/devices")
	if err != nil {
		return nil
	}
	defer file.Close()
	return touchDevices(file, kernelWordBits())
}

// kernelWordBits is the kernel's long size. It can differ from this binary's
// (armhf user space on a 64-bit Raspberry Pi kernel).
func kernelWordBits() uint {
	arch := readFirst([]string{"/proc/sys/kernel/arch"}, runtime.GOARCH)
	if strings.Contains(arch, "64") {
		return 64
	}
	return 32
}

// undervoltageAlarm reads the Raspberry Pi firmware voltage sensor
// (raspberrypi-hwmon, named "rpi_volt"); "" when the board has none.
func undervoltageAlarm() string {
	dirs, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, dir := range dirs {
		if readFirst([]string{filepath.Join(dir, "name")}, "") == "rpi_volt" {
			return readFirst([]string{filepath.Join(dir, "in0_lcrit_alarm")}, "")
		}
	}
	return ""
}

// throttledFlags asks the firmware for the since-boot undervoltage and
// throttling bits; only in the slow status because it spawns vcgencmd.
func throttledFlags(ctx context.Context) string {
	if !commandExists("vcgencmd") {
		return ""
	}
	return runText(ctx, "vcgencmd", "get_throttled")
}

func collectStatus(ctx context.Context) Status {
	ensureDisplayEnv()
	support := detectSupport(ctx)
	return Status{
		Support: support,
		Session: map[string]string{
			"user":    userName(),
			"type":    sessionType(ctx),
			"desktop": sessionDesktop(),
		},
		Device: map[string]any{
			"model":         readFirst([]string{"/sys/firmware/devicetree/base/model", "/sys/class/dmi/id/product_name"}, "Generic"),
			"vendor":        vendor(),
			"serial_number": serialNumber(),
			"host_name":     hostName(),
			"machine_id":    readFirst([]string{"/etc/machine-id"}, ""),
		},
		Network: map[string]any{
			"addresses": networkAddresses(),
			"primary":   primaryAddress(),
		},
		System: map[string]any{
			"uptime_minutes":          uptimeMinutes(),
			"memory_size_gib":         memorySizeGiB(),
			"memory_usage_percent":    memoryUsagePercent(),
			"processor_usage_percent": processorUsagePercent(),
			"processor_count":         runtime.NumCPU(),
			"load_average":            loadAverage(),
			"processor_temperature_c": processorTemperature(),
			"disk_usage":              diskUsage(ctx),
			"battery_level":           batteryLevel(),
			"illuminance_level":       illuminanceLevel(),
			"package_upgrades":        packageUpgradeCount(ctx),
			"touch_devices":           touchDeviceNames(),
			"power":                   powerStatus(undervoltageAlarm(), throttledFlags(ctx)),
		},
		Runtime: runtimeStatus(),
		Display: map[string]any{
			"power":      displayStatus(ctx),
			"brightness": displayBrightness(ctx),
			"command":    displayCommand(ctx),
			"backlight":  backlightPath(),
		},
		Audio: map[string]any{
			"volume":     audioVolume(ctx, false),
			"microphone": audioVolume(ctx, true),
			"sink":       runText(ctx, "pactl", "get-default-sink"),
			"source":     runText(ctx, "pactl", "get-default-source"),
		},
	}
}

func (s *Service) SetDisplay(ctx context.Context, power string) error {
	power = strings.ToUpper(strings.TrimSpace(power))
	if power != "ON" && power != "OFF" {
		return errors.New("display power must be ON or OFF")
	}
	ensureDisplayEnv()
	var err error
	switch displayCommand(ctx) {
	case "ddcutil":
		value := "4"
		if power == "ON" {
			value = "1"
		}
		err = wrapDdcutilSudoError(run(ctx, "sudo", "ddcutil", "setvcp", "0xD6", value))
	case "wlopm":
		err = run(ctx, "wlopm", "--"+strings.ToLower(power), "*")
		if err == nil {
			if status, ok := lastFieldUpper(firstLine(runText(ctx, "wlopm"))).(string); ok && status != "" && status != power {
				err = fmt.Errorf("wlopm reported %s after requesting %s", status, power)
			}
		}
	case "kscreen-doctor":
		err = run(ctx, "kscreen-doctor", "--dpms", strings.ToLower(power))
	case "wlr-randr":
		output := wlrRandrOutputName(ctx)
		if output == "" {
			err = errors.New("wlr-randr output not found")
			break
		}
		flag := "--off"
		if power == "ON" {
			flag = "--on"
		}
		err = run(ctx, "wlr-randr", "--output", output, flag)
	case "xset":
		err = run(ctx, "xset", "dpms", "force", strings.ToLower(power))
	default:
		hint := "display power control unsupported"
		if sessionType(ctx) == "wayland" || os.Getenv("WAYLAND_DISPLAY") != "" {
			hint = "display power control unsupported: install wlopm for Wayland display power"
		}
		err = errors.New(hint)
	}
	if err == nil {
		s.InvalidateCache()
	}
	return err
}

func (s *Service) SetBrightness(ctx context.Context, percent int) error {
	if percent < 1 || percent > 100 {
		return errors.New("brightness must be between 1 and 100")
	}
	if ddcSupports(ctx, "10") {
		err := wrapDdcutilSudoError(run(ctx, "sudo", "ddcutil", "setvcp", "0x10", strconv.Itoa(percent)))
		if err == nil {
			s.InvalidateCache()
		}
		return err
	}
	dir := backlightPath()
	if dir == "" || !sudoRights(ctx) {
		return errors.New("brightness control unsupported")
	}
	max := readInt(filepath.Join(dir, "max_brightness"))
	if max <= 0 {
		return errors.New("brightness max unavailable")
	}
	value := max * percent / 100
	if value < 1 {
		value = 1
	}
	cmd := exec.CommandContext(ctx, "sudo", "tee", filepath.Join(dir, "brightness"))
	cmd.Stdin = strings.NewReader(strconv.Itoa(value))
	var stderr bytes.Buffer
	cmd.Stdout = nil
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return errors.New(strings.TrimSpace(stderr.String()))
	}
	s.InvalidateCache()
	return nil
}

func (s *Service) SetAudioVolume(ctx context.Context, percent int, microphone bool) error {
	if percent < 0 || percent > 100 {
		return errors.New("volume must be between 0 and 100")
	}
	target := "@DEFAULT_SINK@"
	setMute := "set-sink-mute"
	setVol := "set-sink-volume"
	if microphone {
		target = "@DEFAULT_SOURCE@"
		setMute = "set-source-mute"
		setVol = "set-source-volume"
	}
	if err := run(ctx, "pactl", setMute, target, boolInt(percent == 0)); err != nil {
		return err
	}
	if err := run(ctx, "pactl", setVol, target, strconv.Itoa(percent)+"%"); err != nil {
		return err
	}
	s.InvalidateCache()
	return nil
}

func (s *Service) SetKeyboard(ctx context.Context, power string) error {
	power = strings.ToUpper(strings.TrimSpace(power))
	if power != "ON" && power != "OFF" {
		return errors.New("keyboard power must be ON or OFF")
	}
	visible := "false"
	if power == "ON" {
		visible = "true"
	}
	return run(ctx, "dbus-send", "--print-reply", "--type=method_call", "--dest=sm.puri.OSK0", "/sm/puri/OSK0", "sm.puri.OSK0.SetVisible", "boolean:"+visible)
}

func detectSupport(ctx context.Context) Support {
	return Support{
		SudoRights:           sudoRights(ctx),
		DisplayStatus:        displayCommand(ctx) != "",
		DisplayBrightness:    (backlightPath() != "" && sudoRights(ctx)) || (ddcSupports(ctx, "10")),
		AudioVolume:          commandExists("pactl") && runText(ctx, "pactl", "get-default-sink") != "",
		MicrophoneVolume:     commandExists("pactl") && runText(ctx, "pactl", "get-default-source") != "",
		KeyboardVisibility:   processRuns(ctx, "squeekboard") && commandExists("dbus-send"),
		BatteryLevel:         batteryPath() != "",
		IlluminanceLevel:     illuminancePath() != "",
		PackageUpgrades:      commandExists("apt"),
		ProcessorTemperature: processorTemperature() != nil,
	}
}

func displayCommand(ctx context.Context) string {
	ensureDisplayEnv()
	return memoProbe("display-command", displayCmdTTL, func() string { return detectDisplayCommand(ctx) })
}

func detectDisplayCommand(ctx context.Context) string {
	session := sessionType(ctx)
	desktop := sessionDesktop()
	if ddcSupports(ctx, "D6") {
		return "ddcutil"
	}
	if session == "wayland" || os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland" {
		if commandExists("wlopm") {
			if runText(ctx, "wlopm") != "" || strings.Contains(desktop, "labwc") || strings.Contains(desktop, "wayfire") || desktop == "unknown" {
				return "wlopm"
			}
		}
		if commandExists("kscreen-doctor") && runText(ctx, "kscreen-doctor", "--dpms", "show") != "" {
			return "kscreen-doctor"
		}
		if commandExists("wlr-randr") && wlrRandrOutputName(ctx) != "" {
			return "wlr-randr"
		}
		// xset over Xwayland often reports success without blanking the real Wayland output.
		return ""
	}
	if commandExists("xset") && runText(ctx, "xset", "-q") != "" {
		return "xset"
	}
	return ""
}

func displayStatus(ctx context.Context) any {
	switch displayCommand(ctx) {
	case "ddcutil":
		out := runText(ctx, "sudo", "ddcutil", "getvcp", "0xD6", "--brief")
		match := regexp.MustCompile(`VCP D6 \S+ x0?([14])`).FindStringSubmatch(out)
		if len(match) == 2 {
			if match[1] == "1" {
				return "ON"
			}
			return "OFF"
		}
	case "wlopm":
		return lastFieldUpper(firstLine(runText(ctx, "wlopm")))
	case "kscreen-doctor":
		return lastFieldUpper(firstLine(runText(ctx, "kscreen-doctor", "--dpms", "show")))
	case "wlr-randr":
		if enabled, ok := wlrRandrEnabled(ctx); ok {
			if enabled {
				return "ON"
			}
			return "OFF"
		}
	case "xset":
		out := runText(ctx, "xset", "-q")
		if strings.Contains(out, "Monitor is On") {
			return "ON"
		}
		if strings.Contains(out, "Monitor is Off") {
			return "OFF"
		}
	}
	return nil
}

func displayBrightness(ctx context.Context) any {
	if ddcSupports(ctx, "10") {
		out := runText(ctx, "sudo", "ddcutil", "getvcp", "0x10", "--brief")
		match := regexp.MustCompile(`VCP 10 C (\d+) (\d+)`).FindStringSubmatch(out)
		if len(match) == 3 {
			current, _ := strconv.Atoi(match[1])
			maximum, _ := strconv.Atoi(match[2])
			if maximum > 0 {
				return current * 100 / maximum
			}
		}
	}
	dir := backlightPath()
	if dir == "" {
		return nil
	}
	current := readInt(filepath.Join(dir, "brightness"))
	maximum := readInt(filepath.Join(dir, "max_brightness"))
	if maximum <= 0 {
		return nil
	}
	return current * 100 / maximum
}

func audioVolume(ctx context.Context, microphone bool) any {
	if !commandExists("pactl") {
		return nil
	}
	target := "@DEFAULT_SINK@"
	getMute := "get-sink-mute"
	getVol := "get-sink-volume"
	if microphone {
		target = "@DEFAULT_SOURCE@"
		getMute = "get-source-mute"
		getVol = "get-source-volume"
	}
	mute := runText(ctx, "pactl", getMute, target)
	volume := runText(ctx, "pactl", getVol, target)
	match := regexp.MustCompile(`/\s*(\d+)%`).FindStringSubmatch(volume)
	if len(match) != 2 {
		return nil
	}
	value, _ := strconv.Atoi(match[1])
	if strings.Contains(mute, "yes") {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

var (
	packageUpgradeMu     sync.Mutex
	packageUpgradeCached any
	packageUpgradeAt     time.Time
)

func packageUpgradeCount(ctx context.Context) any {
	if !commandExists("apt") {
		return nil
	}
	packageUpgradeMu.Lock()
	defer packageUpgradeMu.Unlock()
	if !packageUpgradeAt.IsZero() && time.Since(packageUpgradeAt) < 10*time.Minute {
		return packageUpgradeCached
	}
	out := runText(ctx, "apt", "list", "--upgradable")
	if out == "" {
		packageUpgradeCached = 0
		packageUpgradeAt = time.Now()
		return 0
	}
	count := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "/") {
			count++
		}
	}
	packageUpgradeCached = count
	packageUpgradeAt = time.Now()
	return count
}

// Disk usage changes slowly; df runs at most every diskUsageTTL.
const diskUsageTTL = 10 * time.Minute

var (
	diskUsageMu     sync.Mutex
	diskUsageCached any
	diskUsageAt     time.Time
)

func diskUsage(ctx context.Context) any {
	diskUsageMu.Lock()
	defer diskUsageMu.Unlock()
	if !diskUsageAt.IsZero() && time.Since(diskUsageAt) < diskUsageTTL {
		return diskUsageCached
	}
	diskUsageCached = readDiskUsage(ctx)
	diskUsageAt = time.Now()
	return diskUsageCached
}

func readDiskUsage(ctx context.Context) any {
	out := runText(ctx, "df", "-B1", "/")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return nil
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return nil
	}
	total, _ := strconv.ParseFloat(fields[1], 64)
	used, _ := strconv.ParseFloat(fields[2], 64)
	avail, _ := strconv.ParseFloat(fields[3], 64)
	percent := 0.0
	if total > 0 {
		percent = used / total * 100
	}
	return map[string]any{
		"total_gb":     round1(total / 1024 / 1024 / 1024),
		"used_gb":      round1(used / 1024 / 1024 / 1024),
		"available_gb": round1(avail / 1024 / 1024 / 1024),
		"percent":      round1(percent),
	}
}

func sessionType(ctx context.Context) string {
	return memoProbe("session-type", sessionTTL, func() string { return detectSessionType(ctx) })
}

func detectSessionType(ctx context.Context) string {
	if !commandExists("loginctl") {
		return os.Getenv("XDG_SESSION_TYPE")
	}
	display := runText(ctx, "loginctl", "show-user", userName(), "-p", "Display", "--value")
	if display == "" {
		return os.Getenv("XDG_SESSION_TYPE")
	}
	return firstNonEmpty(runText(ctx, "loginctl", "show-session", display, "-p", "Type", "--value"), os.Getenv("XDG_SESSION_TYPE"), "unknown")
}

func sessionDesktop() string {
	return strings.ToLower(firstNonEmpty(os.Getenv("XDG_CURRENT_DESKTOP"), os.Getenv("XDG_DESKTOP_SESSION"), os.Getenv("DESKTOP_SESSION"), "unknown"))
}

func vendor() string {
	v := readFirst([]string{"/sys/class/dmi/id/board_vendor"}, "")
	if v != "" {
		return v
	}
	if strings.Contains(readFirst([]string{"/sys/firmware/devicetree/base/model"}, ""), "Raspberry Pi") {
		return "Raspberry Pi Ltd"
	}
	return "Generic"
}

func serialNumber() string {
	value := readFirst([]string{"/sys/firmware/devicetree/base/serial-number"}, "")
	if value != "" {
		return value
	}
	machine := readFirst([]string{"/etc/machine-id"}, "123456")
	if len(machine) > 6 {
		return machine[len(machine)-6:]
	}
	return machine
}

func uptimeMinutes() any {
	value := readFirst([]string{"/proc/uptime"}, "")
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return nil
	}
	seconds, _ := strconv.ParseFloat(fields[0], 64)
	return seconds / 60
}

func memorySizeGiB() any {
	total := meminfo("MemTotal")
	if total == 0 {
		return nil
	}
	return float64(total) / 1024 / 1024
}

func memoryUsagePercent() any {
	total := meminfo("MemTotal")
	available := meminfo("MemAvailable")
	if total == 0 {
		return nil
	}
	return float64(total-available) / float64(total) * 100
}

// processorUsagePercent is the one-minute load average per core. It exceeds
// 100 % when processes wait, including on swap I/O, so the Admin UI shows the
// load average next to the core count instead.
func processorUsagePercent() any {
	load, ok := loadAverage().(float64)
	if !ok {
		return nil
	}
	return load / float64(runtime.NumCPU()) * 100
}

func loadAverage() any {
	fields := strings.Fields(readFirst([]string{"/proc/loadavg"}, ""))
	if len(fields) == 0 {
		return nil
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return nil
	}
	return value
}

func processorTemperature() any {
	types := []string{"cpu-thermal", "x86_pkg_temp", "k10temp", "TCPU", "cpu", "acpitz"}
	entries, _ := os.ReadDir("/sys/class/thermal")
	for _, wanted := range types {
		for _, entry := range entries {
			dir := filepath.Join("/sys/class/thermal", entry.Name())
			if readFirst([]string{filepath.Join(dir, "type")}, "") != wanted {
				continue
			}
			raw := readInt(filepath.Join(dir, "temp"))
			if raw > 0 {
				return float64(raw) / 1000
			}
		}
	}
	return nil
}

func batteryPath() string {
	entries, _ := os.ReadDir("/sys/class/power_supply")
	for _, entry := range entries {
		dir := filepath.Join("/sys/class/power_supply", entry.Name())
		if _, err := os.Stat(filepath.Join(dir, "capacity")); err == nil {
			return dir
		}
	}
	return ""
}

func batteryLevel() any {
	if dir := batteryPath(); dir != "" {
		return readInt(filepath.Join(dir, "capacity"))
	}
	return nil
}

func illuminancePath() string {
	entries, _ := os.ReadDir("/sys/bus/iio/devices")
	for _, entry := range entries {
		dir := filepath.Join("/sys/bus/iio/devices", entry.Name())
		if readFirst([]string{filepath.Join(dir, "name")}, "") == "als" {
			if _, err := os.Stat(filepath.Join(dir, "in_illuminance_raw")); err == nil {
				return dir
			}
		}
	}
	return ""
}

func illuminanceLevel() any {
	if dir := illuminancePath(); dir != "" {
		return readInt(filepath.Join(dir, "in_illuminance_raw"))
	}
	return nil
}

func backlightPath() string {
	entries, _ := os.ReadDir("/sys/class/backlight")
	for _, entry := range entries {
		dir := filepath.Join("/sys/class/backlight", entry.Name())
		if _, err := os.Stat(filepath.Join(dir, "brightness")); err == nil {
			return dir
		}
	}
	return ""
}

func meminfo(key string) uint64 {
	data, _ := os.ReadFile("/proc/meminfo")
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimSuffix(fields[0], ":") == key {
			value, _ := strconv.ParseUint(fields[1], 10, 64)
			return value
		}
	}
	return 0
}

// wrapDdcutilSudoError replaces cryptic sudo/ddcutil failures (missing
// passwordless sudo, denied I2C access under sudo, etc.) with an actionable
// message instead of surfacing raw sudo/ddcutil stderr to the Admin UI.
func wrapDdcutilSudoError(err error) error {
	if err == nil {
		return nil
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "sudo") || strings.Contains(lower, "password is required") || strings.Contains(lower, "permission denied") {
		return fmt.Errorf("sudo required for ddcutil: %s", err.Error())
	}
	return err
}

func processRuns(ctx context.Context, name string) bool {
	return run(ctx, "pidof", name) == nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func ensureDisplayEnv() {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		if uid := os.Getuid(); uid >= 0 {
			runtimeDir = filepath.Join("/run/user", strconv.Itoa(uid))
			if info, err := os.Stat(runtimeDir); err == nil && info.IsDir() {
				_ = os.Setenv("XDG_RUNTIME_DIR", runtimeDir)
			}
		}
	}
	if runtimeDir == "" {
		return
	}
	current := os.Getenv("WAYLAND_DISPLAY")
	if current != "" {
		if _, err := os.Stat(filepath.Join(runtimeDir, current)); err == nil {
			return
		}
	}
	matches, _ := filepath.Glob(filepath.Join(runtimeDir, "wayland-*"))
	for _, match := range matches {
		base := filepath.Base(match)
		if strings.HasSuffix(base, ".lock") {
			continue
		}
		if info, err := os.Stat(match); err == nil && !info.IsDir() {
			_ = os.Setenv("WAYLAND_DISPLAY", base)
			if os.Getenv("XDG_SESSION_TYPE") == "" {
				_ = os.Setenv("XDG_SESSION_TYPE", "wayland")
			}
			return
		}
	}
}

// wlrRandrOutputName returns the first output name reported by `wlr-randr`
// (unindented lines like "HDMI-A-1 ..."), or "" if wlr-randr is unavailable or
// reports no outputs.
func wlrRandrOutputName(ctx context.Context) string {
	for _, line := range strings.Split(runText(ctx, "wlr-randr"), "\n") {
		if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		if fields := strings.Fields(line); len(fields) > 0 {
			return fields[0]
		}
	}
	return ""
}

// wlrRandrEnabled reports whether the first wlr-randr output is currently
// enabled. ok is false when the state could not be determined.
func wlrRandrEnabled(ctx context.Context) (enabled bool, ok bool) {
	out := runText(ctx, "wlr-randr")
	inOutputBlock := false
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if inOutputBlock {
				break
			}
			inOutputBlock = true
			continue
		}
		if !inOutputBlock {
			continue
		}
		if strings.HasPrefix(trimmed, "Enabled:") {
			return strings.Contains(strings.ToLower(trimmed), "yes"), true
		}
	}
	return false, false
}

func commandEnv() []string {
	ensureDisplayEnv()
	return os.Environ()
}

func runText(ctx context.Context, command string, args ...string) string {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, command, args...)
	cmd.Env = commandEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(string(out), "\x00", ""))
}

func run(ctx context.Context, command string, args ...string) error {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, command, args...)
	cmd.Env = commandEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

func readFirst(paths []string, fallback string) string {
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			value := strings.TrimSpace(strings.ReplaceAll(string(data), "\x00", ""))
			if value != "" {
				return value
			}
		}
	}
	return fallback
}

func readInt(path string) int {
	value, _ := strconv.Atoi(readFirst([]string{path}, "0"))
	return value
}

func firstLine(value string) string {
	if idx := strings.IndexByte(value, '\n'); idx >= 0 {
		return value[:idx]
	}
	return value
}

func lastFieldUpper(value string) any {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return nil
	}
	return strings.ToUpper(fields[len(fields)-1])
}

func boolInt(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func sudoRights(ctx context.Context) bool {
	return memoProbe("sudo", sudoProbeTTL, func() string {
		if run(ctx, "sudo", "-n", "true") == nil {
			return "yes"
		}
		return "no"
	}) == "yes"
}

// ddcCapabilities returns the cached `ddcutil capabilities` output, or "" when
// ddcutil or passwordless sudo is unavailable.
func ddcCapabilities(ctx context.Context) string {
	if !commandExists("ddcutil") || !sudoRights(ctx) {
		return ""
	}
	return memoProbe("ddcutil-capabilities", probeTTL, func() string {
		return runText(ctx, "sudo", "ddcutil", "capabilities")
	})
}

func ddcSupports(ctx context.Context, feature string) bool {
	return strings.Contains(ddcCapabilities(ctx), "Feature: "+feature)
}
