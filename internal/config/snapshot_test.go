package config

import "testing"

func TestSnapshotOwnsNestedCollections(t *testing.T) {
	brightness := 40
	cfg := &Config{Kiosk: KioskConfig{
		URLs: []string{"https://ha.example"}, ExtraArgs: []string{"--kiosk"},
		Pages:    []KioskPage{{Name: "Home", Schedule: KioskPageSchedule{Days: []string{"mon"}}, DisplayOptions: KioskDisplayOptions{Brightness: &brightness}}},
		Rotation: []RotationItem{{Page: 0}}, TimeRules: []TimeRule{{Days: []string{"tue"}}},
	}}
	copy := cfg.Snapshot()
	copy.Kiosk.URLs[0] = "changed"
	copy.Kiosk.ExtraArgs[0] = "changed"
	copy.Kiosk.Pages[0].Name = "changed"
	copy.Kiosk.Pages[0].Schedule.Days[0] = "sun"
	*copy.Kiosk.Pages[0].DisplayOptions.Brightness = 1
	copy.Kiosk.Rotation[0].Page = 99
	copy.Kiosk.TimeRules[0].Days[0] = "sun"
	if cfg.Kiosk.URLs[0] != "https://ha.example" || cfg.Kiosk.ExtraArgs[0] != "--kiosk" || cfg.Kiosk.Pages[0].Name != "Home" || cfg.Kiosk.Pages[0].Schedule.Days[0] != "mon" || brightness != 40 || cfg.Kiosk.Rotation[0].Page != 0 || cfg.Kiosk.TimeRules[0].Days[0] != "tue" {
		t.Fatal("snapshot mutation changed the published configuration")
	}
}
