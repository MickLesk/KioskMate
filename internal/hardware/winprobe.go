package hardware

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// The PowerShell scripts and their parsing for the Windows implementation
// carry no build tag so they are unit-tested on Linux as well.

// windowsProbeScript reads device data and the WMI brightness state in one
// PowerShell start. Every query may fail on its own (no battery, an external
// monitor without WMI brightness) and then yields null.
const windowsProbeScript = `$ErrorActionPreference = 'SilentlyContinue'
$cs = Get-CimInstance -ClassName Win32_ComputerSystem
$bios = Get-CimInstance -ClassName Win32_BIOS
$bright = @(Get-CimInstance -Namespace root/wmi -ClassName WmiMonitorBrightness | Where-Object { $_.Active }) | Select-Object -First 1
$methods = @(Get-CimInstance -Namespace root/wmi -ClassName WmiMonitorBrightnessMethods).Count
$battery = @(Get-CimInstance -ClassName Win32_Battery) | Select-Object -First 1
[pscustomobject]@{
  manufacturer = [string]$cs.Manufacturer
  model = [string]$cs.Model
  serial = [string]$bios.SerialNumber
  brightness = if ($bright) { [int]$bright.CurrentBrightness } else { $null }
  brightness_methods = [int]$methods
  battery = if ($battery) { [int]$battery.EstimatedChargeRemaining } else { $null }
} | ConvertTo-Json -Compress`

type windowsProbe struct {
	Manufacturer      string `json:"manufacturer"`
	Model             string `json:"model"`
	Serial            string `json:"serial"`
	Brightness        *int   `json:"brightness"`
	BrightnessMethods int    `json:"brightness_methods"`
	Battery           *int   `json:"battery"`
}

func parseWindowsProbe(output string) (windowsProbe, error) {
	var probe windowsProbe
	output = strings.TrimSpace(strings.TrimPrefix(output, "\ufeff"))
	if output == "" {
		return probe, errors.New("empty PowerShell output")
	}
	if err := json.Unmarshal([]byte(output), &probe); err != nil {
		return windowsProbe{}, err
	}
	probe.Manufacturer = strings.TrimSpace(probe.Manufacturer)
	probe.Model = strings.TrimSpace(probe.Model)
	probe.Serial = strings.TrimSpace(probe.Serial)
	if probe.Brightness != nil && (*probe.Brightness < 0 || *probe.Brightness > 100) {
		probe.Brightness = nil
	}
	if probe.Battery != nil && (*probe.Battery < 0 || *probe.Battery > 100) {
		probe.Battery = nil
	}
	return probe, nil
}

// brightnessSupported reports whether WMI exposes a brightness control. This
// is usually the internal panel of a laptop or tablet; external monitors are
// normally not controllable through WMI.
func (p windowsProbe) brightnessSupported() bool {
	return p.BrightnessMethods > 0 && p.Brightness != nil
}

// windowsBrightnessScript sets the brightness of every WMI-controllable
// monitor. The percent value is validated by the caller and formatted as an
// integer, so nothing user-supplied reaches the script as text.
func windowsBrightnessScript(percent int) string {
	return `$ErrorActionPreference = 'Stop'
$monitors = @(Get-CimInstance -Namespace root/wmi -ClassName WmiMonitorBrightnessMethods)
if ($monitors.Count -eq 0) { throw 'no monitor exposes WMI brightness control' }
foreach ($monitor in $monitors) {
  Invoke-CimMethod -InputObject $monitor -MethodName WmiSetBrightness -Arguments @{ Timeout = [uint32]0; Brightness = [byte]` + strconv.Itoa(percent) + ` } | Out-Null
}`
}

// powershellArgs builds a non-interactive Windows PowerShell invocation that
// ignores user profiles and the local script execution policy.
func powershellArgs(script string) []string {
	return []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script}
}
