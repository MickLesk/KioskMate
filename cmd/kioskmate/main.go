package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/MickLesk/KioskMate/internal/actions"
	"github.com/MickLesk/KioskMate/internal/admin"
	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/hardware"
	"github.com/MickLesk/KioskMate/internal/integration"
	"github.com/MickLesk/KioskMate/internal/supervisor"
	"github.com/MickLesk/KioskMate/internal/updater"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "", "Path to KioskMate config file")
	printVersion := flag.Bool("version", false, "Print version and exit")
	adminInfo := flag.Bool("admin-info", false, "Print admin diagnostics and exit")
	doctor := flag.Bool("doctor", false, "Print system diagnostics and exit")
	repair := flag.Bool("repair", false, "Repair config and user service defaults")
	adminReset := flag.Bool("admin-reset", false, "Remove admin password hash and return to setup mode")
	adminPassword := flag.Bool("admin-password", false, "Set admin password from KIOSKMATE_ADMIN_PASSWORD")
	flag.Parse()

	if *printVersion {
		fmt.Println(version)
		return
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}
	if *adminInfo || *doctor || *repair || *adminReset || *adminPassword {
		if err := handleCommand(*adminInfo, *doctor, *repair, *adminReset, *adminPassword, cfg, version); err != nil {
			logger.Error("command failed", "error", err)
			os.Exit(1)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	browser := supervisor.NewBrowser(cfg, logger.With("component", "browser"))
	updateService := updater.New(cfg, version)
	actionService := actions.New(cfg)
	hardwareService := hardware.New()
	mqttService := integration.NewMQTTService(cfg, browser, hardwareService, updateService, actionService, version, logger.With("component", "mqtt"))
	server := admin.NewServer(cfg, browser, updateService, actionService, hardwareService, version, logger.With("component", "admin"))

	if err := browser.Start(ctx); err != nil {
		logger.Warn("initial browser start failed", "error", err)
	}

	errc := make(chan error, 1)
	go func() {
		errc <- server.ListenAndServe(ctx)
	}()
	go browser.RunScheduler(ctx)
	go mqttService.Run(ctx)

	logger.Info("kioskmate core started", "version", version, "config", cfg.Path, "admin", cfg.Admin.Addr())
	for _, url := range adminURLs(cfg.Admin.Bind, cfg.Admin.Port) {
		logger.Info("admin ui", "url", url)
	}
	if cfg.Admin.PasswordHash == "" {
		logger.Info("admin setup token", "token", cfg.Admin.Token)
	}

	select {
	case <-ctx.Done():
		logger.Info("shutdown requested")
	case err := <-errc:
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("admin server stopped", "error", err)
			os.Exit(1)
		}
	}

	if err := browser.Stop(context.Background()); err != nil {
		logger.Warn("browser stop failed", "error", err)
	}
}

func adminURLs(bind string, port int) []string {
	if bind != "" && bind != "0.0.0.0" && bind != "::" {
		return []string{fmt.Sprintf("http://%s:%d", bind, port)}
	}
	urls := []string{fmt.Sprintf("http://127.0.0.1:%d", port)}
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
			if ip4 := ip.To4(); ip4 != nil && !ip4.IsLoopback() {
				urls = append(urls, fmt.Sprintf("http://%s:%d", ip4.String(), port))
			}
		}
	}
	return urls
}

func handleCommand(adminInfo, doctor, repair, adminReset, adminPassword bool, cfg *config.Config, version string) error {
	if repair {
		if err := config.Save(cfg); err != nil {
			return err
		}
	}
	if adminReset {
		cfg.Admin.PasswordHash = ""
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Println("Admin password removed. Restart KioskMate and use the setup token.")
	}
	if adminPassword {
		password := os.Getenv("KIOSKMATE_ADMIN_PASSWORD")
		if len(password) < 8 {
			return fmt.Errorf("KIOSKMATE_ADMIN_PASSWORD must contain at least 8 characters")
		}
		hash, err := admin.HashPassword(password)
		if err != nil {
			return err
		}
		cfg.Admin.PasswordHash = hash
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Println("Admin password updated.")
	}
	if adminInfo {
		fmt.Println("KioskMate admin info")
		fmt.Println("Version:", version)
		fmt.Println("Config file:", cfg.Path)
		fmt.Println("Config backup:", cfg.Path+".bak")
		fmt.Println("Admin address:", cfg.Admin.Addr())
		fmt.Println("Admin password configured:", cfg.Admin.PasswordHash != "")
		if cfg.Admin.PasswordHash == "" {
			fmt.Println("Setup token:", cfg.Admin.Token)
		}
		for _, url := range adminURLs(cfg.Admin.Bind, cfg.Admin.Port) {
			fmt.Println("Admin UI:", url)
		}
		fmt.Println("User service:", "/usr/lib/systemd/user/"+cfg.Update.Service)
	}
	if doctor {
		fmt.Println("KioskMate doctor")
		fmt.Println("Version:", version)
		fmt.Println("Config:", cfg.Path)
		fmt.Println("Config exists:", fileExists(cfg.Path))
		fmt.Println("Service:", cfg.Update.Service)
		fmt.Println("Service active:", commandOK("systemctl", "--user", "is-active", "--quiet", cfg.Update.Service))
		fmt.Println("Chromium available:", browserAvailable())
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		status := hardware.New().Status(ctx)
		data, _ := json.MarshalIndent(status, "", "  ")
		fmt.Println(string(data))
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func commandOK(command string, args ...string) bool {
	return exec.Command(command, args...).Run() == nil
}

func browserAvailable() bool {
	for _, name := range []string{"chromium-browser", "chromium", "google-chrome-stable", "google-chrome", "microsoft-edge"} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}
