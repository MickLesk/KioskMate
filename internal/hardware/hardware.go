package hardware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Service struct{}

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
			"processor_temperature_c": processorTemperature(),
			"disk_usage":              diskUsage(ctx),
			"battery_level":           batteryLevel(),
			"illuminance_level":       illuminanceLevel(),
			"package_upgrades":        packageUpgradeCount(ctx),
		},
		Runtime: map[string]any{
			"go":         runtime.Version(),
			"goos":       runtime.GOOS,
			"goarch":     runtime.GOARCH,
			"goroutines": runtime.NumGoroutine(),
		},
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
	switch displayCommand(ctx) {
	case "ddcutil":
		value := "4"
		if power == "ON" {
			value = "1"
		}
		return run(ctx, "sudo", "ddcutil", "setvcp", "0xD6", value)
	case "wlopm":
		return run(ctx, "wlopm", "--"+strings.ToLower(power), "*")
	case "kscreen-doctor":
		return run(ctx, "kscreen-doctor", "--dpms", strings.ToLower(power))
	case "xset":
		return run(ctx, "xset", "dpms", "force", strings.ToLower(power))
	default:
		return errors.New("display power control unsupported")
	}
}

func (s *Service) SetBrightness(ctx context.Context, percent int) error {
	if percent < 1 || percent > 100 {
		return errors.New("brightness must be between 1 and 100")
	}
	if commandExists("ddcutil") && sudoRights(ctx) && strings.Contains(runText(ctx, "sudo", "ddcutil", "capabilities"), "Feature: 10") {
		return run(ctx, "sudo", "ddcutil", "setvcp", "0x10", strconv.Itoa(percent))
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
	return run(ctx, "pactl", setVol, target, strconv.Itoa(percent)+"%")
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
		DisplayBrightness:    (backlightPath() != "" && sudoRights(ctx)) || (commandExists("ddcutil") && sudoRights(ctx) && strings.Contains(runText(ctx, "sudo", "ddcutil", "capabilities"), "Feature: 10")),
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
	session := sessionType(ctx)
	desktop := sessionDesktop()
	if commandExists("ddcutil") && sudoRights(ctx) && strings.Contains(runText(ctx, "sudo", "ddcutil", "capabilities"), "Feature: D6") {
		return "ddcutil"
	}
	if session == "wayland" {
		if commandExists("wlopm") && (strings.Contains(desktop, "labwc") || strings.Contains(desktop, "wayfire") || desktop == "unknown") {
			if runText(ctx, "wlopm") != "" {
				return "wlopm"
			}
		}
		if commandExists("kscreen-doctor") && runText(ctx, "kscreen-doctor", "--dpms", "show") != "" {
			return "kscreen-doctor"
		}
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
	if commandExists("ddcutil") && sudoRights(ctx) {
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

func packageUpgradeCount(ctx context.Context) any {
	if !commandExists("apt") {
		return nil
	}
	out := runText(ctx, "apt", "list", "--upgradable")
	if out == "" {
		return 0
	}
	count := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "/") {
			count++
		}
	}
	return count
}

func diskUsage(ctx context.Context) any {
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

func sessionType(ctx context.Context) string {
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

func processorUsagePercent() any {
	load := readFirst([]string{"/proc/loadavg"}, "")
	fields := strings.Fields(load)
	if len(fields) == 0 {
		return nil
	}
	value, _ := strconv.ParseFloat(fields[0], 64)
	return value / float64(runtime.NumCPU()) * 100
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

func sudoRights(ctx context.Context) bool {
	return run(ctx, "sudo", "-n", "true") == nil
}

func processRuns(ctx context.Context, name string) bool {
	return run(ctx, "pidof", name) == nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func runText(ctx context.Context, command string, args ...string) string {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, command, args...).CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(string(out), "\x00", ""))
}

func run(ctx context.Context, command string, args ...string) error {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, command, args...).CombinedOutput()
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func boolInt(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func round1(value float64) float64 {
	return float64(int(value*10+0.5)) / 10
}

func MarshalStatus(status Status) []byte {
	data, _ := json.Marshal(status)
	return data
}
