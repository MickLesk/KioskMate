package config

import "testing"

func TestKioskConflicts(t *testing.T) {
	kiosk := KioskConfig{
		Pages: []KioskPage{{Name: "Doorbell page", URL: "https://a", DisplayMode: "mqtt", Trigger: KioskPageTrigger{Topic: "home/bell"}}},
		TimeRules: []TimeRule{
			{Name: "Morning", Page: 0, Start: "06:00", End: "09:00", Days: []string{"mon"}},
			{Name: "Night", Page: 1, Start: "22:00", End: "07:00"},
			{Name: "Weekend", Page: 2, Start: "08:00", End: "10:00", Days: []string{"sat"}},
			{Name: "Same page", Page: 0, Start: "07:00", End: "08:00"},
		},
		Rules: []WorkflowRule{
			{ID: "a", Name: "Bell A", Trigger: RuleTrigger{Type: "mqtt", Topic: "home/bell", Payload: "ring"}},
			{ID: "b", Name: "Bell B", Trigger: RuleTrigger{Type: "mqtt", Topic: "home/bell"}},
			{ID: "c", Name: "Other", Trigger: RuleTrigger{Type: "mqtt", Topic: "home/other"}},
			{ID: "d", Name: "Off", Disabled: true, Trigger: RuleTrigger{Type: "mqtt", Topic: "home/other"}},
		},
	}
	got := map[string]int{}
	for _, conflict := range kiosk.Conflicts() {
		got[conflict.Kind]++
	}
	// Only Morning and Night overlap (06:00-07:00 on Monday); end times are
	// exclusive and two rules for the same page do not conflict.
	if got["time_rules_overlap"] != 1 {
		t.Errorf("time rule overlaps = %d, want 1: %#v", got["time_rules_overlap"], kiosk.Conflicts())
	}
	if got["rules_same_trigger"] != 1 {
		t.Errorf("same trigger = %d, want 1", got["rules_same_trigger"])
	}
	if got["rule_page_topic"] != 2 {
		t.Errorf("rule/page topic = %d, want 2", got["rule_page_topic"])
	}
}
