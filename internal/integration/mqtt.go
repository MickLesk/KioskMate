package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/MickLesk/KioskMate/internal/actions"
	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/hardware"
	"github.com/MickLesk/KioskMate/internal/mqttclient"
	"github.com/MickLesk/KioskMate/internal/supervisor"
	"github.com/MickLesk/KioskMate/internal/updater"
)

type Browser interface {
	Start(context.Context) error
	Stop(context.Context) error
	Restart(context.Context) error
	Reload(context.Context) error
	Next(context.Context) error
	Previous(context.Context) error
	SetActive(context.Context, int) error
	ResetSession(context.Context) error
	Status() supervisor.Status
}

type MQTTService struct {
	cfg      *config.Config
	browser  Browser
	hardware *hardware.Service
	updater  *updater.Service
	actions  *actions.Service
	logger   *slog.Logger
	version  string
	client   *mqttclient.Client
	command  *mqttclient.Client
	cache    map[string]string
	cleaned  bool
}

type discoveryItem struct {
	Topic string
	Data  map[string]any
}

func NewMQTTService(cfg *config.Config, browser Browser, hw *hardware.Service, updates *updater.Service, actionService *actions.Service, version string, logger *slog.Logger) *MQTTService {
	return &MQTTService{cfg: cfg, browser: browser, hardware: hw, updater: updates, actions: actionService, version: version, logger: logger, cache: map[string]string{}}
}

func (s *MQTTService) PublishNow() error {
	if !s.cfg.MQTT.Enabled {
		return errors.New("mqtt is disabled")
	}
	if s.cfg.MQTT.URL == "" {
		return errors.New("mqtt url is empty")
	}
	return s.publishAll()
}

func (s *MQTTService) Run(ctx context.Context) {
	var commandCancel context.CancelFunc
	activeKey := ""
	defer func() {
		if commandCancel != nil {
			commandCancel()
		}
		s.closeClients()
	}()
	for {
		if !s.cfg.MQTT.Enabled || s.cfg.MQTT.URL == "" {
			if activeKey != "" {
				s.closeClients()
				if commandCancel != nil {
					commandCancel()
					commandCancel = nil
				}
				activeKey = ""
				s.logger.Info("mqtt disabled")
			}
			if !sleepContext(ctx, 5*time.Second) {
				return
			}
			continue
		}
		key := s.connectionKey()
		if key != activeKey {
			s.closeClients()
			if commandCancel != nil {
				commandCancel()
			}
			commandCtx, cancel := context.WithCancel(ctx)
			commandCancel = cancel
			activeKey = key
			s.cache = map[string]string{}
			go s.commands(commandCtx)
			s.logger.Info("mqtt connection configured", "root", s.root(), "discovery", s.cfg.MQTT.Discovery, "version", s.cfg.MQTT.Version)
		}
		if err := s.publishAll(); err != nil {
			s.logger.Warn("mqtt publish failed", "error", err)
			if s.client != nil {
				_ = s.client.Close()
				s.client = nil
			}
		}
		if !sleepContext(ctx, mqttInterval(s.cfg.MQTT.Interval)) {
			if s.client != nil {
				_ = s.client.Publish(s.root()+"/availability", []byte("offline"), s.retained(true))
			}
			return
		}
	}
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func mqttInterval(interval time.Duration) time.Duration {
	if interval <= 0 {
		return 30 * time.Second
	}
	if interval < 5*time.Second {
		return 5 * time.Second
	}
	return interval
}

func (s *MQTTService) connectionKey() string {
	return strings.Join([]string{
		s.cfg.MQTT.URL,
		s.cfg.MQTT.Username,
		s.cfg.MQTT.Password,
		s.cfg.MQTT.BaseTopic,
		s.cfg.MQTT.Node,
		s.cfg.MQTT.ClientID,
		s.cfg.MQTT.Discovery,
		s.cfg.MQTT.Version,
		s.cfg.MQTT.KeepAlive.String(),
		fmt.Sprint(s.cfg.MQTT.ForceDisableRetain),
	}, "\x00")
}

func (s *MQTTService) closeClients() {
	if s.client != nil {
		_ = s.client.Publish(s.root()+"/availability", []byte("offline"), s.retained(true))
		_ = s.client.Close()
		s.client = nil
	}
	if s.command != nil {
		_ = s.command.Close()
		s.command = nil
	}
}

func (s *MQTTService) commands(ctx context.Context) {
	client := &mqttclient.Client{
		URL:       s.cfg.MQTT.URL,
		ClientID:  firstNonEmpty(s.cfg.MQTT.ClientID, s.cfg.MQTT.Node) + "_cmd",
		Username:  s.cfg.MQTT.Username,
		Password:  s.cfg.MQTT.Password,
		Version:   s.cfg.MQTT.Version,
		KeepAlive: s.cfg.MQTT.KeepAlive,
	}
	s.command = client
	topics := []string{
		s.root() + "/command",
		s.root() + "/start/execute",
		s.root() + "/stop/execute",
		s.root() + "/restart/execute",
		s.root() + "/refresh/execute",
		s.root() + "/next/execute",
		s.root() + "/previous/execute",
		s.root() + "/reset_session/execute",
		s.root() + "/shutdown/execute",
		s.root() + "/reboot/execute",
		s.root() + "/restart_service/execute",
		s.root() + "/apt_update/execute",
		s.root() + "/apt_upgrade/execute",
		s.root() + "/update/install",
		s.root() + "/display/power/set",
		s.root() + "/display/brightness/set",
		s.root() + "/volume/set",
		s.root() + "/microphone/set",
		s.root() + "/keyboard/set",
		s.root() + "/page_number/set",
		s.root() + "/page_name/set",
		s.root() + "/page_url/set",
		s.root() + "/scheduler_enabled/set",
		s.root() + "/scheduler_mode/set",
		s.root() + "/scheduler_tick/set",
		s.root() + "/performance_profile/set",
		s.root() + "/gpu_mode/set",
		s.root() + "/reduce_motion/set",
		s.root() + "/watchdog_enabled/set",
	}
	for {
		err := client.Subscribe(topics, func(topic string, payload []byte) {
			s.handleCommand(ctx, topic, strings.TrimSpace(string(payload)))
		})
		if ctx.Err() != nil {
			_ = client.Close()
			return
		}
		s.logger.Warn("mqtt command subscription failed", "error", err)
		if !sleepContext(ctx, 5*time.Second) {
			_ = client.Close()
			return
		}
	}
}

func (s *MQTTService) handleCommand(ctx context.Context, topic string, command string) {
	actionCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var err error
	switch {
	case strings.HasSuffix(topic, "/start/execute"):
		err = s.browser.Start(actionCtx)
	case strings.HasSuffix(topic, "/stop/execute"):
		err = s.browser.Stop(actionCtx)
	case strings.HasSuffix(topic, "/restart/execute"):
		err = s.browser.Restart(actionCtx)
	case strings.HasSuffix(topic, "/refresh/execute"):
		err = s.browser.Reload(actionCtx)
	case strings.HasSuffix(topic, "/next/execute"):
		err = s.browser.Next(actionCtx)
	case strings.HasSuffix(topic, "/previous/execute"):
		err = s.browser.Previous(actionCtx)
	case strings.HasSuffix(topic, "/reset_session/execute"):
		err = s.browser.ResetSession(actionCtx)
	case strings.HasSuffix(topic, "/shutdown/execute"):
		_, err = s.actions.Start(actionCtx, "shutdown")
	case strings.HasSuffix(topic, "/reboot/execute"):
		_, err = s.actions.Start(actionCtx, "reboot")
	case strings.HasSuffix(topic, "/restart_service/execute"):
		_, err = s.actions.Start(actionCtx, "restart-service")
	case strings.HasSuffix(topic, "/apt_update/execute"):
		_, err = s.actions.Start(actionCtx, "apt-update")
	case strings.HasSuffix(topic, "/apt_upgrade/execute"):
		_, err = s.actions.Start(actionCtx, "apt-upgrade")
	case strings.HasSuffix(topic, "/update/install"):
		if s.updater == nil {
			err = fmt.Errorf("updater unavailable")
			break
		}
		s.updater.Install(context.Background())
	case strings.HasSuffix(topic, "/display/power/set"):
		err = s.hardware.SetDisplay(actionCtx, command)
	case strings.HasSuffix(topic, "/display/brightness/set"):
		err = s.hardware.SetBrightness(actionCtx, atoi(command))
	case strings.HasSuffix(topic, "/volume/set"):
		err = s.hardware.SetAudioVolume(actionCtx, atoi(command), false)
	case strings.HasSuffix(topic, "/microphone/set"):
		err = s.hardware.SetAudioVolume(actionCtx, atoi(command), true)
	case strings.HasSuffix(topic, "/keyboard/set"):
		err = s.hardware.SetKeyboard(actionCtx, command)
	case strings.HasSuffix(topic, "/page_number/set"):
		err = s.browser.SetActive(actionCtx, atoi(command)-1)
	case strings.HasSuffix(topic, "/page_name/set"):
		err = s.setPageByName(actionCtx, command)
	case strings.HasSuffix(topic, "/page_url/set"):
		if strings.HasPrefix(command, "http://") || strings.HasPrefix(command, "https://") {
			s.cfg.Kiosk.URLs = []string{command}
			s.cfg.Kiosk.Pages = []config.KioskPage{{Name: "MQTT Page", URL: command}}
			if err = config.Save(s.cfg); err == nil {
				err = s.browser.SetActive(actionCtx, 0)
			}
		}
	case strings.HasSuffix(topic, "/scheduler_enabled/set"):
		s.cfg.Kiosk.Scheduler.Enabled = boolCommand(command)
		if s.cfg.Kiosk.Scheduler.Mode == "" {
			s.cfg.Kiosk.Scheduler.Mode = "rotation"
		}
		err = config.Save(s.cfg)
	case strings.HasSuffix(topic, "/scheduler_mode/set"):
		mode := strings.ToLower(command)
		if !validSchedulerMode(mode) {
			err = fmt.Errorf("unsupported scheduler mode %q", command)
			break
		}
		s.cfg.Kiosk.Scheduler.Mode = mode
		err = config.Save(s.cfg)
	case strings.HasSuffix(topic, "/scheduler_tick/set"):
		seconds := atoi(command)
		if seconds < 5 {
			seconds = 5
		}
		s.cfg.Kiosk.Scheduler.TickInterval = time.Duration(seconds) * time.Second
		err = config.Save(s.cfg)
	case strings.HasSuffix(topic, "/performance_profile/set"):
		profile := strings.ToLower(strings.TrimSpace(command))
		if !validPerformanceProfile(profile) {
			err = fmt.Errorf("unsupported performance profile %q", command)
			break
		}
		s.cfg.Performance.Profile = profile
		err = config.Save(s.cfg)
	case strings.HasSuffix(topic, "/gpu_mode/set"):
		mode := strings.ToLower(strings.TrimSpace(command))
		if mode != "auto" && mode != "software" {
			err = fmt.Errorf("unsupported gpu mode %q", command)
			break
		}
		s.cfg.Performance.GPUMode = mode
		err = config.Save(s.cfg)
	case strings.HasSuffix(topic, "/reduce_motion/set"):
		s.cfg.Performance.ReduceMotion = boolCommand(command)
		err = config.Save(s.cfg)
	case strings.HasSuffix(topic, "/watchdog_enabled/set"):
		s.cfg.Watchdog.Enabled = boolCommand(command)
		err = config.Save(s.cfg)
	default:
		switch strings.ToLower(command) {
		case "start":
			err = s.browser.Start(actionCtx)
		case "stop":
			err = s.browser.Stop(actionCtx)
		case "restart", "reload":
			err = s.browser.Restart(actionCtx)
		case "refresh":
			err = s.browser.Reload(actionCtx)
		case "next":
			err = s.browser.Next(actionCtx)
		case "previous", "prev":
			err = s.browser.Previous(actionCtx)
		case "reboot", "shutdown", "apt-update", "apt-upgrade", "restart-service":
			_, err = s.actions.Start(actionCtx, strings.ReplaceAll(strings.ToLower(command), "_", "-"))
		default:
			s.logger.Warn("unknown mqtt command", "command", command)
			return
		}
	}
	if err != nil {
		s.logger.Warn("mqtt command failed", "command", command, "error", err)
	}
	s.publishCommandResult(topic, command, err)
}

func (s *MQTTService) publishAll() error {
	client := s.mqtt()
	if err := s.publishDiscovery(client); err != nil {
		return err
	}
	status := s.browser.Status()
	hw := s.hardware.Status(context.Background())
	root := s.root()
	schedulerReason := status.Scheduler.Reason
	if schedulerReason == "" {
		if s.cfg.Kiosk.Scheduler.Enabled {
			schedulerReason = "waiting"
		} else {
			schedulerReason = "disabled"
		}
	}
	schedulerMode := status.Scheduler.Mode
	if schedulerMode == "" {
		schedulerMode = s.cfg.Kiosk.Scheduler.Mode
	}
	if err := s.publishState(client, "browser", boolState(status.Running), true); err != nil {
		return err
	}
	state := map[string]any{
		"running":      status.Running,
		"pid":          status.PID,
		"rss_mb":       status.Stats.RSSMB,
		"cpu_percent":  status.Stats.CPUPercent,
		"pids":         status.Stats.PIDs,
		"page":         status.Active + 1,
		"page_name":    status.PageName,
		"page_url":     status.URL,
		"scheduler":    status.Scheduler,
		"version":      s.version,
		"mqtt_version": s.cfg.MQTT.Version,
		"updated":      time.Now().UTC().Format(time.RFC3339),
	}
	payload, _ := json.Marshal(state)
	if err := client.Publish(root+"/state", payload, false); err != nil {
		return err
	}
	_ = client.Publish(root+"/availability", []byte("online"), s.retained(true))
	_ = s.publishState(client, "rss", fmt.Sprintf("%d", status.Stats.RSSMB), false)
	_ = s.publishState(client, "cpu", fmt.Sprintf("%.1f", status.Stats.CPUPercent), false)
	_ = s.publishState(client, "version", s.version, true)
	_ = s.publishState(client, "update/installed_version", s.version, true)
	_ = s.publishState(client, "update/latest_version", s.version, true)
	_ = s.publishState(client, "update/title", "KioskMate "+s.version, true)
	_ = s.publishState(client, "update/release_url", "", true)
	_ = s.publishState(client, "mqtt_version", s.cfg.MQTT.Version, true)
	_ = s.publishState(client, "browser_pid", fmt.Sprintf("%d", status.PID), false)
	_ = s.publishState(client, "browser_command", status.Command, true)
	_ = s.publishState(client, "browser_last_error", firstString(status.LastError, "none"), false)
	_ = s.publishState(client, "page_count", fmt.Sprintf("%d", s.cfg.Kiosk.PageCount()), true)
	_ = s.publishState(client, "page_number", fmt.Sprintf("%d", status.Active+1), true)
	_ = s.publishState(client, "page_name", status.PageName, true)
	_ = s.publishState(client, "page_url", status.URL, true)
	_ = s.publishState(client, "scheduler_enabled", boolState(s.cfg.Kiosk.Scheduler.Enabled), true)
	_ = s.publishState(client, "scheduler_state", schedulerReason, true)
	_ = s.publishState(client, "scheduler_mode", schedulerMode, true)
	_ = s.publishState(client, "scheduler_tick", fmt.Sprintf("%d", int(s.cfg.Kiosk.Scheduler.TickInterval/time.Second)), true)
	_ = s.publishState(client, "scheduler_active_rule", firstString(status.Scheduler.ActiveRule, "none"), true)
	if status.Scheduler.NextSwitch != nil {
		_ = s.publishState(client, "scheduler_next_switch", status.Scheduler.NextSwitch.Format(time.RFC3339), true)
	} else {
		_ = s.publishState(client, "scheduler_next_switch", "none", true)
	}
	_ = s.publishState(client, "performance_profile", s.cfg.Performance.Profile, true)
	_ = s.publishState(client, "gpu_mode", s.cfg.Performance.GPUMode, true)
	_ = s.publishState(client, "reduce_motion", boolState(s.cfg.Performance.ReduceMotion), true)
	_ = s.publishState(client, "watchdog_enabled", boolState(s.cfg.Watchdog.Enabled), true)
	_ = s.publishState(client, "heartbeat", time.Now().Format("2006-01-02T15:04:05"), false)
	_ = s.publishSystemStates(client, hw)
	_ = client.Publish(root+"/hardware/state", hardware.MarshalStatus(hw), false)
	return nil
}

func (s *MQTTService) publishDiscovery(client *mqttclient.Client) error {
	status := s.hardware.Status(context.Background())
	device := map[string]any{
		"identifiers":   []string{"kioskmate", s.cfg.MQTT.Node},
		"name":          "KioskMate",
		"manufacturer":  "MickLesk",
		"model":         firstString(status.Device["model"], "KioskMate Supervisor"),
		"serial_number": status.Device["serial_number"],
		"sw_version":    s.version,
		"hw_version":    firstString(status.Device["vendor"], ""),
	}
	items := []discoveryItem{
		{
			Topic: s.discoveryTopic("binary_sensor", "browser"),
			Data: map[string]any{
				"name":        "Browser Running",
				"unique_id":   s.cfg.MQTT.Node + "_browser_running",
				"state_topic": s.root() + "/browser/state",
				"payload_on":  "ON",
				"payload_off": "OFF",
				"device":      device,
			},
		},
		{
			Topic: s.discoveryTopic("button", "start"),
			Data: map[string]any{
				"name":          "Start Browser",
				"unique_id":     s.cfg.MQTT.Node + "_start_browser",
				"command_topic": s.root() + "/start/execute",
				"icon":          "mdi:play",
				"device":        device,
			},
		},
		{
			Topic: s.discoveryTopic("button", "stop"),
			Data: map[string]any{
				"name":          "Stop Browser",
				"unique_id":     s.cfg.MQTT.Node + "_stop_browser",
				"command_topic": s.root() + "/stop/execute",
				"icon":          "mdi:stop",
				"device":        device,
			},
		},
		{
			Topic: s.discoveryTopic("button", "refresh"),
			Data: map[string]any{
				"name":          "Refresh",
				"unique_id":     s.cfg.MQTT.Node + "_refresh",
				"command_topic": s.root() + "/refresh/execute",
				"icon":          "mdi:web-refresh",
				"device":        device,
			},
		},
		{
			Topic: s.discoveryTopic("button", "previous"),
			Data: map[string]any{
				"name":          "Previous Page",
				"unique_id":     s.cfg.MQTT.Node + "_previous_page",
				"command_topic": s.root() + "/previous/execute",
				"icon":          "mdi:arrow-left",
				"device":        device,
			},
		},
		{
			Topic: s.discoveryTopic("button", "next"),
			Data: map[string]any{
				"name":          "Next Page",
				"unique_id":     s.cfg.MQTT.Node + "_next_page",
				"command_topic": s.root() + "/next/execute",
				"icon":          "mdi:arrow-right",
				"device":        device,
			},
		},
		{
			Topic: s.discoveryTopic("button", "reset_session"),
			Data: map[string]any{
				"name":          "Reset HA Session",
				"unique_id":     s.cfg.MQTT.Node + "_reset_ha_session",
				"command_topic": s.root() + "/reset_session/execute",
				"icon":          "mdi:cookie-refresh",
				"device":        device,
			},
		},
		{
			Topic: s.discoveryTopic("button", "reboot"),
			Data: map[string]any{
				"name":          "Reboot",
				"unique_id":     s.cfg.MQTT.Node + "_reboot",
				"command_topic": s.root() + "/reboot/execute",
				"icon":          "mdi:restart",
				"device":        device,
			},
		},
		{
			Topic: s.discoveryTopic("button", "shutdown"),
			Data: map[string]any{
				"name":          "Shutdown",
				"unique_id":     s.cfg.MQTT.Node + "_shutdown",
				"command_topic": s.root() + "/shutdown/execute",
				"icon":          "mdi:power",
				"device":        device,
			},
		},
		{
			Topic: s.discoveryTopic("button", "restart_service"),
			Data: map[string]any{
				"name":            "Restart Service",
				"unique_id":       s.cfg.MQTT.Node + "_restart_service",
				"command_topic":   s.root() + "/restart_service/execute",
				"icon":            "mdi:restart-alert",
				"entity_category": "config",
				"device":          device,
			},
		},
		{
			Topic: s.discoveryTopic("update", "app"),
			Data: map[string]any{
				"name":                    "KioskMate Update",
				"unique_id":               s.cfg.MQTT.Node + "_app_update",
				"title":                   "KioskMate",
				"installed_version_topic": s.root() + "/update/installed_version/state",
				"latest_version_topic":    s.root() + "/update/latest_version/state",
				"release_url_topic":       s.root() + "/update/release_url/state",
				"command_topic":           s.root() + "/update/install",
				"payload_install":         "INSTALL",
				"entity_category":         "config",
				"device":                  device,
			},
		},
		{
			Topic: s.discoveryTopic("button", "apt_update"),
			Data: map[string]any{
				"name":            "Apt Update",
				"unique_id":       s.cfg.MQTT.Node + "_apt_update",
				"command_topic":   s.root() + "/apt_update/execute",
				"icon":            "mdi:package-variant",
				"entity_category": "config",
				"device":          device,
			},
		},
		{
			Topic: s.discoveryTopic("button", "apt_upgrade"),
			Data: map[string]any{
				"name":            "Apt Upgrade",
				"unique_id":       s.cfg.MQTT.Node + "_apt_upgrade",
				"command_topic":   s.root() + "/apt_upgrade/execute",
				"icon":            "mdi:package-up",
				"entity_category": "config",
				"device":          device,
			},
		},
		{
			Topic: s.discoveryTopic("sensor", "cpu"),
			Data: map[string]any{
				"name":                "Browser CPU",
				"unique_id":           s.cfg.MQTT.Node + "_browser_cpu",
				"state_topic":         s.root() + "/cpu/state",
				"unit_of_measurement": "%",
				"state_class":         "measurement",
				"device":              device,
			},
		},
		s.diagnosticSensor(device, "browser_pid", "Browser PID", "mdi:identifier", ""),
		s.diagnosticSensor(device, "browser_command", "Browser Command", "mdi:console", ""),
		s.diagnosticSensor(device, "browser_last_error", "Browser Last Error", "mdi:alert-circle", ""),
		s.diagnosticSensor(device, "page_count", "Page Count", "mdi:counter", ""),
		s.sensor(device, "model", "Model", "mdi:raspberry-pi", ""),
		s.sensor(device, "serial_number", "Serial Number", "mdi:hexadecimal", ""),
		s.sensor(device, "host_name", "Host Name", "mdi:console-network", ""),
		s.sensor(device, "network_address", "Network Address", "mdi:ip-network", ""),
		s.sensor(device, "up_time", "Up Time", "mdi:timeline-clock", "min"),
		s.sensor(device, "memory_size", "Memory Size", "mdi:memory", "GiB"),
		s.sensor(device, "memory_usage", "Memory Usage", "mdi:memory-arrow-down", "%"),
		s.sensor(device, "processor_usage", "Processor Usage", "mdi:cpu-64-bit", "%"),
		s.sensor(device, "processor_temperature", "Processor Temperature", "mdi:radiator", "°C"),
		s.sensor(device, "battery_level", "Battery Level", "mdi:battery-medium", "%"),
		s.sensor(device, "illuminance_level", "Illuminance Level", "mdi:brightness-5", "lx"),
		s.sensor(device, "package_upgrades", "Package Upgrades", "mdi:package-down", ""),
		s.sensor(device, "disk_usage", "Disk Usage", "mdi:harddisk", "%"),
		s.sensor(device, "heartbeat", "Heartbeat", "mdi:heart-flash", ""),
		s.sensor(device, "mqtt_version", "MQTT Version", "mdi:protocol", ""),
		s.sensor(device, "page_name", "Page Name", "mdi:card-text", ""),
		s.sensor(device, "scheduler_state", "Scheduler State", "mdi:calendar-clock", ""),
		s.sensor(device, "scheduler_mode", "Scheduler Mode", "mdi:timeline-clock", ""),
		s.sensor(device, "scheduler_tick", "Scheduler Tick", "mdi:timer-cog", "s"),
		s.sensor(device, "scheduler_active_rule", "Scheduler Active Rule", "mdi:calendar-check", ""),
		s.sensor(device, "scheduler_next_switch", "Scheduler Next Switch", "mdi:clock-end", ""),
		s.sensor(device, "performance_profile", "Performance Profile", "mdi:speedometer", ""),
		s.sensor(device, "gpu_mode", "GPU Mode", "mdi:chip", ""),
		s.sensor(device, "last_command", "Last Command", "mdi:console-line", ""),
		s.sensor(device, "last_command_status", "Last Command Status", "mdi:list-status", ""),
		s.sensor(device, "last_command_error", "Last Command Error", "mdi:alert", ""),
		s.diagnosticSensor(device, "last_command_json", "Last Command JSON", "mdi:code-json", ""),
		{
			Topic: s.discoveryTopic("light", "display"),
			Data: map[string]any{
				"name":                     "Display",
				"unique_id":                s.cfg.MQTT.Node + "_display",
				"command_topic":            s.root() + "/display/power/set",
				"state_topic":              s.root() + "/display/power/state",
				"brightness_command_topic": s.root() + "/display/brightness/set",
				"brightness_state_topic":   s.root() + "/display/brightness/state",
				"brightness_scale":         100,
				"payload_on":               "ON",
				"payload_off":              "OFF",
				"supported_color_modes":    []string{"brightness"},
				"device":                   device,
			},
		},
		{
			Topic: s.discoveryTopic("number", "volume"),
			Data: map[string]any{
				"name":          "Volume",
				"unique_id":     s.cfg.MQTT.Node + "_volume",
				"command_topic": s.root() + "/volume/set",
				"state_topic":   s.root() + "/volume/state",
				"min":           0,
				"max":           100,
				"step":          1,
				"icon":          "mdi:volume-high",
				"device":        device,
			},
		},
		{
			Topic: s.discoveryTopic("number", "microphone"),
			Data: map[string]any{
				"name":          "Microphone",
				"unique_id":     s.cfg.MQTT.Node + "_microphone",
				"command_topic": s.root() + "/microphone/set",
				"state_topic":   s.root() + "/microphone/state",
				"min":           0,
				"max":           100,
				"step":          1,
				"icon":          "mdi:microphone",
				"device":        device,
			},
		},
		{
			Topic: s.discoveryTopic("switch", "keyboard"),
			Data: map[string]any{
				"name":          "Keyboard",
				"unique_id":     s.cfg.MQTT.Node + "_keyboard",
				"command_topic": s.root() + "/keyboard/set",
				"state_topic":   s.root() + "/keyboard/state",
				"payload_on":    "ON",
				"payload_off":   "OFF",
				"icon":          "mdi:keyboard-close-outline",
				"device":        device,
			},
		},
		{
			Topic: s.discoveryTopic("number", "page_number"),
			Data: map[string]any{
				"name":          "Page Number",
				"unique_id":     s.cfg.MQTT.Node + "_page_number",
				"command_topic": s.root() + "/page_number/set",
				"state_topic":   s.root() + "/page_number/state",
				"min":           1,
				"max":           max(1, s.cfg.Kiosk.PageCount()),
				"step":          1,
				"icon":          "mdi:book-open-page-variant",
				"device":        device,
			},
		},
		{
			Topic: s.discoveryTopic("select", "page_name"),
			Data: map[string]any{
				"name":          "Page Name",
				"unique_id":     s.cfg.MQTT.Node + "_page_name_select",
				"command_topic": s.root() + "/page_name/set",
				"state_topic":   s.root() + "/page_name/state",
				"options":       s.pageNames(),
				"icon":          "mdi:view-dashboard",
				"device":        device,
			},
		},
		{
			Topic: s.discoveryTopic("text", "page_url"),
			Data: map[string]any{
				"name":          "Page Url",
				"unique_id":     s.cfg.MQTT.Node + "_page_url",
				"command_topic": s.root() + "/page_url/set",
				"state_topic":   s.root() + "/page_url/state",
				"pattern":       "https?://.*",
				"icon":          "mdi:web",
				"device":        device,
			},
		},
		{
			Topic: s.discoveryTopic("switch", "reduce_motion"),
			Data: map[string]any{
				"name":            "Reduce Motion",
				"unique_id":       s.cfg.MQTT.Node + "_reduce_motion",
				"command_topic":   s.root() + "/reduce_motion/set",
				"state_topic":     s.root() + "/reduce_motion/state",
				"payload_on":      "ON",
				"payload_off":     "OFF",
				"icon":            "mdi:motion-pause-outline",
				"entity_category": "config",
				"device":          device,
			},
		},
		{
			Topic: s.discoveryTopic("switch", "watchdog_enabled"),
			Data: map[string]any{
				"name":            "Browser Watchdog",
				"unique_id":       s.cfg.MQTT.Node + "_watchdog_enabled",
				"command_topic":   s.root() + "/watchdog_enabled/set",
				"state_topic":     s.root() + "/watchdog_enabled/state",
				"payload_on":      "ON",
				"payload_off":     "OFF",
				"icon":            "mdi:shield-refresh",
				"entity_category": "config",
				"device":          device,
			},
		},
		{
			Topic: s.discoveryTopic("switch", "scheduler_enabled"),
			Data: map[string]any{
				"name":            "Kiosk Scheduler",
				"unique_id":       s.cfg.MQTT.Node + "_scheduler_enabled",
				"command_topic":   s.root() + "/scheduler_enabled/set",
				"state_topic":     s.root() + "/scheduler_enabled/state",
				"payload_on":      "ON",
				"payload_off":     "OFF",
				"icon":            "mdi:calendar-clock",
				"entity_category": "config",
				"device":          device,
			},
		},
		{
			Topic: s.discoveryTopic("select", "scheduler_mode"),
			Data: map[string]any{
				"name":            "Scheduler Mode",
				"unique_id":       s.cfg.MQTT.Node + "_scheduler_mode_select",
				"command_topic":   s.root() + "/scheduler_mode/set",
				"state_topic":     s.root() + "/scheduler_mode/state",
				"options":         []string{"rotation", "time", "hybrid"},
				"icon":            "mdi:timeline-clock",
				"entity_category": "config",
				"device":          device,
			},
		},
		{
			Topic: s.discoveryTopic("number", "scheduler_tick"),
			Data: map[string]any{
				"name":                "Scheduler Tick",
				"unique_id":           s.cfg.MQTT.Node + "_scheduler_tick_number",
				"command_topic":       s.root() + "/scheduler_tick/set",
				"state_topic":         s.root() + "/scheduler_tick/state",
				"min":                 5,
				"max":                 3600,
				"step":                5,
				"unit_of_measurement": "s",
				"icon":                "mdi:timer-cog",
				"entity_category":     "config",
				"device":              device,
			},
		},
		{
			Topic: s.discoveryTopic("select", "performance_profile"),
			Data: map[string]any{
				"name":            "Performance Profile",
				"unique_id":       s.cfg.MQTT.Node + "_performance_profile_select",
				"command_topic":   s.root() + "/performance_profile/set",
				"state_topic":     s.root() + "/performance_profile/state",
				"options":         []string{"quality", "balanced", "raspberry", "minimal"},
				"icon":            "mdi:speedometer",
				"entity_category": "config",
				"device":          device,
			},
		},
		{
			Topic: s.discoveryTopic("select", "gpu_mode"),
			Data: map[string]any{
				"name":            "GPU Mode",
				"unique_id":       s.cfg.MQTT.Node + "_gpu_mode_select",
				"command_topic":   s.root() + "/gpu_mode/set",
				"state_topic":     s.root() + "/gpu_mode/state",
				"options":         []string{"auto", "software"},
				"icon":            "mdi:chip",
				"entity_category": "config",
				"device":          device,
			},
		},
		{
			Topic: s.discoveryTopic("sensor", "rss"),
			Data: map[string]any{
				"name":                "Browser RSS",
				"unique_id":           s.cfg.MQTT.Node + "_browser_rss",
				"state_topic":         s.root() + "/rss/state",
				"unit_of_measurement": "MB",
				"state_class":         "measurement",
				"device":              device,
			},
		},
		{
			Topic: s.discoveryTopic("sensor", "version"),
			Data: map[string]any{
				"name":        "Version",
				"unique_id":   s.cfg.MQTT.Node + "_version",
				"state_topic": s.root() + "/version/state",
				"icon":        "mdi:tag",
				"device":      device,
			},
		},
	}
	if !s.cleaned {
		_ = s.cleanupLegacyDiscovery(client, items)
		s.cleaned = true
	}
	for _, item := range items {
		s.decorateDiscovery(item.Data)
		payload, _ := json.Marshal(item.Data)
		if err := client.Publish(item.Topic, payload, s.retained(true)); err != nil {
			return err
		}
	}
	return nil
}

func (s *MQTTService) sensor(device map[string]any, id string, name string, icon string, unit string) discoveryItem {
	data := map[string]any{
		"name":        name,
		"unique_id":   s.cfg.MQTT.Node + "_" + id,
		"state_topic": s.root() + "/" + id + "/state",
		"icon":        icon,
		"device":      device,
	}
	if unit != "" {
		data["unit_of_measurement"] = unit
		data["state_class"] = "measurement"
	}
	return discoveryItem{Topic: s.discoveryTopic("sensor", id), Data: data}
}

func (s *MQTTService) diagnosticSensor(device map[string]any, id string, name string, icon string, unit string) discoveryItem {
	item := s.sensor(device, id, name, icon, unit)
	item.Data["entity_category"] = "diagnostic"
	if unit == "" {
		delete(item.Data, "state_class")
	}
	return item
}

func (s *MQTTService) cleanupLegacyDiscovery(client *mqttclient.Client, current []discoveryItem) error {
	status := s.hardware.Status(context.Background())
	var nodes []string
	if node := legacyRpiNode(status.Device["serial_number"]); node != "" {
		nodes = append(nodes, node)
	}
	entries := legacyDiscoveryEntries()
	for _, item := range current {
		component, object, ok := parseDiscoveryTopic(item.Topic)
		if ok {
			entries = append(entries, [2]string{component, object})
		}
	}
	seen := map[string]bool{}
	for _, node := range nodes {
		if node == "" || node == s.cfg.MQTT.Node {
			continue
		}
		for _, entry := range entries {
			topic := strings.Trim(s.cfg.MQTT.Discovery, "/") + "/" + entry[0] + "/" + node + "/" + entry[1] + "/config"
			if seen[topic] {
				continue
			}
			seen[topic] = true
			if err := client.Publish(topic, []byte{}, s.retained(true)); err != nil {
				return err
			}
		}
	}
	return nil
}

func legacyDiscoveryEntries() [][2]string {
	return [][2]string{
		{"update", "app"},
		{"button", "shutdown"},
		{"button", "reboot"},
		{"button", "refresh"},
		{"button", "apt_update"},
		{"button", "apt_upgrade"},
		{"sensor", "apt_status"},
		{"select", "kiosk"},
		{"select", "theme"},
		{"light", "display"},
		{"number", "volume"},
		{"number", "microphone"},
		{"switch", "keyboard"},
		{"number", "page_number"},
		{"number", "page_zoom"},
		{"text", "page_url"},
		{"sensor", "model"},
		{"sensor", "serial_number"},
		{"sensor", "network_address"},
		{"sensor", "host_name"},
		{"sensor", "up_time"},
		{"sensor", "memory_size"},
		{"sensor", "memory_usage"},
		{"sensor", "processor_usage"},
		{"sensor", "processor_temperature"},
		{"sensor", "battery_level"},
		{"sensor", "illuminance_level"},
		{"sensor", "package_upgrades"},
		{"sensor", "disk_usage"},
		{"sensor", "last_active"},
		{"image", "screenshot"},
		{"sensor", "heartbeat"},
		{"sensor", "errors"},
		{"sensor", "version"},
	}
}

func legacyRpiNode(serial any) string {
	raw := fmt.Sprint(serial)
	if raw == "" || raw == "<nil>" {
		return ""
	}
	if len(raw) > 6 {
		raw = raw[len(raw)-6:]
	}
	var b strings.Builder
	for _, r := range strings.ToUpper(raw) {
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return "rpi_" + b.String()
}

func parseDiscoveryTopic(topic string) (string, string, bool) {
	parts := strings.Split(strings.Trim(topic, "/"), "/")
	if len(parts) < 5 || parts[len(parts)-1] != "config" {
		return "", "", false
	}
	return parts[len(parts)-4], parts[len(parts)-2], true
}

func (s *MQTTService) decorateDiscovery(data map[string]any) {
	data["availability_topic"] = s.root() + "/availability"
	data["payload_available"] = "online"
	data["payload_not_available"] = "offline"
	data["origin"] = map[string]any{
		"name":       "KioskMate",
		"sw_version": s.version,
	}
}

func (s *MQTTService) publishSystemStates(client *mqttclient.Client, hw hardware.Status) error {
	values := map[string]any{
		"model":                 hw.Device["model"],
		"serial_number":         hw.Device["serial_number"],
		"host_name":             hw.Device["host_name"],
		"network_address":       hw.Network["primary"],
		"up_time":               hw.System["uptime_minutes"],
		"memory_size":           hw.System["memory_size_gib"],
		"memory_usage":          hw.System["memory_usage_percent"],
		"processor_usage":       hw.System["processor_usage_percent"],
		"processor_temperature": hw.System["processor_temperature_c"],
		"battery_level":         hw.System["battery_level"],
		"illuminance_level":     hw.System["illuminance_level"],
		"package_upgrades":      hw.System["package_upgrades"],
		"disk_usage":            nested(hw.System["disk_usage"], "percent"),
		"display/power":         hw.Display["power"],
		"display/brightness":    hw.Display["brightness"],
		"volume":                hw.Audio["volume"],
		"microphone":            hw.Audio["microphone"],
		"keyboard":              nil,
	}
	for key, value := range values {
		if value == nil || reflect.ValueOf(value).Kind() == reflect.Invalid {
			continue
		}
		if err := s.publishState(client, key, stateString(value), false); err != nil {
			return err
		}
	}
	return nil
}

func stateString(value any) string {
	switch v := value.(type) {
	case float64:
		return trimFloat(v)
	case float32:
		return trimFloat(float64(v))
	case int, int64, int32, uint, uint64, uint32:
		return fmt.Sprint(v)
	case string:
		return v
	case bool:
		return boolState(v)
	default:
		return fmt.Sprint(v)
	}
}

func trimFloat(value float64) string {
	text := fmt.Sprintf("%.1f", value)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	return text
}

func (s *MQTTService) publishCommandResult(topic string, command string, err error) {
	client := s.mqtt()
	status := "ok"
	errText := ""
	if err != nil {
		status = "error"
		errText = err.Error()
	}
	result := map[string]any{
		"topic":   topic,
		"command": command,
		"status":  status,
		"error":   errText,
		"time":    time.Now().UTC().Format(time.RFC3339),
	}
	payload, _ := json.Marshal(result)
	_ = client.Publish(s.root()+"/last_command_json/state", payload, false)
	_ = s.publishState(client, "last_command_status", status, false)
	_ = s.publishState(client, "last_command_error", firstString(errText, "none"), false)
	_ = s.publishState(client, "last_command", strings.TrimSpace(command), false)
}

func (s *MQTTService) publishState(client *mqttclient.Client, path string, value string, retained bool) error {
	if s.cache[path] == value {
		return nil
	}
	s.cache[path] = value
	return client.Publish(s.root()+"/"+path+"/state", []byte(value), s.retained(retained))
}

func (s *MQTTService) mqtt() *mqttclient.Client {
	if s.client == nil {
		s.client = &mqttclient.Client{
			URL:       s.cfg.MQTT.URL,
			ClientID:  firstNonEmpty(s.cfg.MQTT.ClientID, s.cfg.MQTT.Node),
			Username:  s.cfg.MQTT.Username,
			Password:  s.cfg.MQTT.Password,
			Version:   s.cfg.MQTT.Version,
			KeepAlive: s.cfg.MQTT.KeepAlive,
		}
	}
	return s.client
}

func (s *MQTTService) root() string {
	base := strings.Trim(firstNonEmpty(s.cfg.MQTT.BaseTopic, "kioskmate"), "/")
	node := strings.Trim(firstNonEmpty(s.cfg.MQTT.Node, "kioskmate"), "/")
	return base + "/" + node
}

func (s *MQTTService) retained(retained bool) bool {
	return retained && !s.cfg.MQTT.ForceDisableRetain
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (s *MQTTService) pageNames() []string {
	names := make([]string, 0, s.cfg.Kiosk.PageCount())
	for index, url := range s.cfg.Kiosk.PageURLs() {
		name := s.cfg.Kiosk.PageName(index)
		if name == "" {
			name = url
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return []string{"Home Assistant"}
	}
	return names
}

func (s *MQTTService) setPageByName(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("page name required")
	}
	for index, candidate := range s.pageNames() {
		if strings.EqualFold(candidate, name) {
			return s.browser.SetActive(ctx, index)
		}
	}
	return fmt.Errorf("page %q not found", name)
}

func (s *MQTTService) discoveryTopic(component, object string) string {
	return strings.Trim(s.cfg.MQTT.Discovery, "/") + "/" + component + "/" + s.cfg.MQTT.Node + "/" + object + "/config"
}

func boolState(value bool) string {
	if value {
		return "ON"
	}
	return "OFF"
}

func boolCommand(command string) bool {
	switch strings.ToUpper(strings.TrimSpace(command)) {
	case "ON", "TRUE", "1", "YES", "ENABLE", "ENABLED":
		return true
	default:
		return false
	}
}

func validSchedulerMode(mode string) bool {
	switch mode {
	case "rotation", "time", "hybrid":
		return true
	default:
		return false
	}
}

func validPerformanceProfile(profile string) bool {
	switch profile {
	case "quality", "balanced", "raspberry", "minimal":
		return true
	default:
		return false
	}
}

func atoi(value string) int {
	i, _ := strconv.Atoi(strings.TrimSpace(value))
	return i
}

func firstString(value any, fallback string) string {
	if text, ok := value.(string); ok && text != "" {
		return text
	}
	return fallback
}

func nested(value any, key string) any {
	if item, ok := value.(map[string]any); ok {
		return item[key]
	}
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
