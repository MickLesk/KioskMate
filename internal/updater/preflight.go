package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type PreflightCheck struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type PreflightReport struct {
	OK             bool             `json:"ok"`
	TargetVersion  string           `json:"target_version,omitempty"`
	RequiredBytes  int64            `json:"required_bytes,omitempty"`
	AvailableBytes int64            `json:"available_bytes,omitempty"`
	PrivilegeMode  string           `json:"privilege_mode,omitempty"`
	PrivilegeScope []string         `json:"privilege_scope"`
	Checks         []PreflightCheck `json:"checks"`
}

func (s *Service) Preflight(ctx context.Context, mode, password string) PreflightReport {
	requestedMode := strings.TrimSpace(mode)
	if requestedMode == "" {
		requestedMode = "sudo"
	}
	report := PreflightReport{
		OK:             true,
		PrivilegeMode:  requestedMode,
		PrivilegeScope: []string{"apt-get install local verified package", "package maintainer scripts", "systemctl --user daemon-reload and service restart"},
	}
	add := func(id string, ok bool, message string) {
		report.Checks = append(report.Checks, PreflightCheck{ID: id, OK: ok, Message: message})
		if !ok {
			report.OK = false
		}
	}
	info, err := s.Check(ctx)
	if err != nil {
		add("release", false, err.Error())
		return report
	}
	report.TargetVersion = info.LatestVersion
	add("release", info.UpdateAvailable, func() string {
		if info.UpdateAvailable {
			return "release " + info.LatestVersion + " is available"
		}
		return "KioskMate is already up to date"
	}())
	add("platform", runtime.GOOS == "linux", runtime.GOOS+"/"+runtime.GOARCH)
	add("asset", info.Asset.URL != "", func() string {
		if info.Asset.URL == "" {
			return "no matching Debian package was found"
		}
		return info.Asset.Name
	}())
	for _, command := range []string{"apt-get", "dpkg-deb", "systemctl"} {
		_, lookupErr := exec.LookPath(command)
		add("command_"+strings.ReplaceAll(command, "-", "_"), lookupErr == nil, func() string {
			if lookupErr != nil {
				return command + " is not installed"
			}
			return command + " is available"
		}())
	}
	report.RequiredBytes = requiredDiskBytes(info.Asset.Size)
	available, diskErr := freeDiskBytes(os.TempDir())
	report.AvailableBytes = available
	add("disk", diskErr == nil && available >= report.RequiredBytes, func() string {
		if diskErr != nil {
			return diskErr.Error()
		}
		return fmt.Sprintf("%d MiB available, %d MiB required", available/(1<<20), report.RequiredBytes/(1<<20))
	}())
	privilege, privilegeErr := s.preparePrivilege(ctx, mode, password)
	if privilege.Mode != "" {
		report.PrivilegeMode = privilege.Mode
	}
	add("privilege", privilegeErr == nil, func() string {
		if privilegeErr != nil {
			return privilegeErr.Error()
		}
		return "administrator authentication succeeded"
	}())
	return report
}

func (s *Service) validateEnvironment(asset Asset) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("updates require Linux, current platform is %s", runtime.GOOS)
	}
	for _, command := range []string{"apt-get", "dpkg-deb", "systemctl"} {
		if _, err := exec.LookPath(command); err != nil {
			return fmt.Errorf("required command %s is unavailable", command)
		}
	}
	available, err := freeDiskBytes(os.TempDir())
	if err != nil {
		return fmt.Errorf("check temporary disk space: %w", err)
	}
	required := requiredDiskBytes(asset.Size)
	if available < required {
		return fmt.Errorf("insufficient temporary disk space: %d MiB available, %d MiB required", available/(1<<20), required/(1<<20))
	}
	return nil
}

func requiredDiskBytes(packageSize int64) int64 {
	const reserve = int64(64 << 20)
	if packageSize <= 0 {
		packageSize = 16 << 20
	}
	return packageSize*3 + reserve
}

func validateDebPackage(ctx context.Context, file, expectedVersion string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("Debian package validation requires Linux")
	}
	field := func(name string) (string, error) {
		output, err := exec.CommandContext(ctx, "dpkg-deb", "-f", file, name).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("read Debian %s: %s", name, strings.TrimSpace(string(output)))
		}
		return strings.TrimSpace(string(output)), nil
	}
	name, err := field("Package")
	if err != nil {
		return err
	}
	if name != "kioskmate" {
		return fmt.Errorf("unexpected Debian package name %q", name)
	}
	architecture, err := field("Architecture")
	if err != nil {
		return err
	}
	expected := map[string]string{"arm64": "arm64", "amd64": "amd64"}[runtime.GOARCH]
	version, err := field("Version")
	if err != nil {
		return err
	}
	if err := validateDebMetadata(name, version, architecture, expectedVersion, expected); err != nil {
		return err
	}
	contents, err := exec.CommandContext(ctx, "dpkg-deb", "--contents", file).CombinedOutput()
	if err != nil {
		return fmt.Errorf("read Debian package contents: %s", strings.TrimSpace(string(contents)))
	}
	listing := string(contents)
	if !containsExecutableDebPath(listing, "./usr/bin/kioskmate") {
		return errors.New("Debian package is missing executable /usr/bin/kioskmate")
	}
	if !containsDebPath(listing, "./usr/lib/systemd/user/kioskmate.service") {
		return errors.New("Debian package is missing kioskmate.service")
	}
	return nil
}

func validateDebMetadata(name, version, architecture, expectedVersion, expectedArchitecture string) error {
	if name != "kioskmate" {
		return fmt.Errorf("unexpected Debian package name %q", name)
	}
	if expectedArchitecture == "" || architecture != expectedArchitecture {
		return fmt.Errorf("Debian package architecture %q does not match %q", architecture, expectedArchitecture)
	}
	if strings.TrimPrefix(version, "v") != strings.TrimPrefix(expectedVersion, "v") {
		return fmt.Errorf("Debian package version %q does not match release %q", version, expectedVersion)
	}
	return nil
}

func containsDebPath(listing, expected string) bool {
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.TrimPrefix(fields[len(fields)-1], "/") == strings.TrimPrefix(expected, "/") {
			return true
		}
	}
	return false
}

func containsExecutableDebPath(listing, expected string) bool {
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.TrimPrefix(fields[len(fields)-1], "/") != strings.TrimPrefix(expected, "/") {
			continue
		}
		return len(fields[0]) >= 4 && fields[0][3] == 'x'
	}
	return false
}
