package main

import (
	"context"
	"flag"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/MickLesk/KioskMate/internal/actions"
	"github.com/MickLesk/KioskMate/internal/admin"
	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/hardware"
	"github.com/MickLesk/KioskMate/internal/supervisor"
	"github.com/MickLesk/KioskMate/internal/updater"
)

type fixtureBrowser struct {
	status supervisor.Status
}

func (b *fixtureBrowser) Start(context.Context) error {
	b.status.Running, b.status.Ready, b.status.State = true, true, "running"
	return nil
}
func (b *fixtureBrowser) Stop(context.Context) error {
	b.status.Running, b.status.Ready, b.status.State = false, false, "stopped"
	return nil
}
func (b *fixtureBrowser) Restart(ctx context.Context) error    { return b.Start(ctx) }
func (b *fixtureBrowser) Next(context.Context) error           { return nil }
func (b *fixtureBrowser) Previous(context.Context) error       { return nil }
func (b *fixtureBrowser) ResetSession(context.Context) error   { return nil }
func (b *fixtureBrowser) Reload(context.Context) error         { return nil }
func (b *fixtureBrowser) SetActive(context.Context, int) error { return nil }
func (b *fixtureBrowser) TripAuthGuard(string)                 {}
func (b *fixtureBrowser) NoteDisplayPower(string)              {}
func (b *fixtureBrowser) Status() supervisor.Status            { return b.status }
func (b *fixtureBrowser) CaptureScreenshot(context.Context) ([]byte, error) {
	file, err := os.CreateTemp("", "kioskmate-e2e-*.png")
	if err != nil {
		return nil, err
	}
	name := file.Name()
	defer os.Remove(name)
	defer file.Close()
	canvas := image.NewRGBA(image.Rect(0, 0, 16, 9))
	for y := 0; y < 9; y++ {
		for x := 0; x < 16; x++ {
			canvas.Set(x, y, color.RGBA{R: 13, G: 15, B: 16, A: 255})
		}
	}
	if err := png.Encode(file, canvas); err != nil {
		return nil, err
	}
	return os.ReadFile(name)
}

func main() {
	port := flag.Int("port", 33339, "Admin fixture port")
	flag.Parse()
	dir, err := os.MkdirTemp("", "kioskmate-admin-e2e-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	cfg, err := config.Load(filepath.Join(dir, "config.json"))
	if err != nil {
		panic(err)
	}
	hash, err := admin.HashPassword("KioskMate-E2E")
	if err != nil {
		panic(err)
	}
	if err := cfg.Mutate(func(next *config.Config) error {
		next.Admin.Bind = "127.0.0.1"
		next.Admin.Port = *port
		next.Admin.PasswordHash = hash
		next.MQTT.Enabled = false
		next.Kiosk.Pages = []config.KioskPage{{PageID: "home", Name: "Home", URL: "https://demo.home-assistant.io", DurationSeconds: 60}}
		next.Kiosk.URLs = []string{"https://demo.home-assistant.io"}
		return nil
	}); err != nil {
		panic(err)
	}
	browser := &fixtureBrowser{status: supervisor.Status{
		State: "running", Running: true, Ready: true, Command: "chromium", Active: 0,
		PageName: "Home", URL: "https://demo.home-assistant.io",
		Display:    supervisor.DisplaySessionStatus{Ready: true, Type: "fixture"},
		Navigation: supervisor.NavigationStatus{State: "ready", Responsive: true, DocumentState: "complete"},
	}}
	actionService := actions.New(cfg)
	updateService := updater.New(cfg, "0.9.0-e2e", actionService)
	hardwareService := hardware.New()
	server := admin.NewServer(cfg, browser, nil, updateService, actionService, hardwareService, "0.9.0-e2e", slog.Default())
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := server.ListenAndServe(ctx); err != nil && ctx.Err() == nil {
		panic(err)
	}
}
