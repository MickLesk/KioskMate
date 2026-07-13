package systemtime

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Status struct {
	CurrentTime  time.Time `json:"current_time"`
	Timezone     string    `json:"timezone"`
	NTPEnabled   bool      `json:"ntp_enabled"`
	Synchronized bool      `json:"synchronized"`
	Service      string    `json:"service"`
	Server       string    `json:"server"`
	Supported    bool      `json:"supported"`
	CanConfigure bool      `json:"can_configure"`
	Error        string    `json:"error,omitempty"`
}

func Read(ctx context.Context, configuredServer string) Status {
	status := Status{CurrentTime: time.Now(), Server: configuredServer}
	if runtime.GOOS != "linux" {
		status.Timezone = status.CurrentTime.Location().String()
		status.Error = "system time control is only available on Linux"
		return status
	}
	path, err := exec.LookPath("timedatectl")
	if err != nil {
		status.Timezone = status.CurrentTime.Location().String()
		status.Error = "timedatectl is unavailable"
		return status
	}
	status.Supported = true
	status.CanConfigure = true
	status.Service = "systemd-timesyncd"
	command := exec.CommandContext(ctx, path, "show", "--property=Timezone", "--property=NTP", "--property=NTPSynchronized")
	output, err := command.Output()
	if err != nil {
		status.Error = err.Error()
		return status
	}
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "Timezone":
			status.Timezone = value
		case "NTP":
			status.NTPEnabled = value == "yes"
		case "NTPSynchronized":
			status.Synchronized = value == "yes"
		}
	}
	return status
}

func Zones() []string {
	file, err := os.Open("/usr/share/zoneinfo/zone.tab")
	if err != nil {
		return []string{"UTC", "Europe/Berlin"}
	}
	defer file.Close()
	zones := []string{"UTC"}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			zones = append(zones, fields[2])
		}
	}
	sort.Strings(zones)
	return zones
}
