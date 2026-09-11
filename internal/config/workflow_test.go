package config

import "testing"

func TestWorkflowIssuesDetectConflicts(t *testing.T) {
	kiosk := KioskConfig{
		Pages: []KioskPage{
			{PageID: "one", Name: "One", URL: "https://one.test", DisplayMode: "mqtt", Trigger: KioskPageTrigger{Topic: "screen/show", Payload: "ON"}},
			{PageID: "two", Name: "Two", URL: "https://two.test", DisplayMode: "mqtt", Trigger: KioskPageTrigger{Topic: "screen/show", Payload: "ON"}},
		},
		TimeRules: []TimeRule{
			{Name: "Morning", Start: "08:00", End: "10:00", Days: []string{"mon"}},
			{Name: "Meeting", Start: "09:30", End: "11:00", Days: []string{"mon"}},
		},
	}
	issues := WorkflowIssues(kiosk)
	want := map[string]bool{"duplicate_mqtt_trigger": false, "overlapping_schedule": false}
	for _, issue := range issues {
		if _, ok := want[issue.Code]; ok {
			want[issue.Code] = true
		}
	}
	for code, found := range want {
		if !found {
			t.Fatalf("missing %s in %#v", code, issues)
		}
	}
}

func TestWorkflowIssuesHandlesOvernightRules(t *testing.T) {
	kiosk := KioskConfig{TimeRules: []TimeRule{
		{Name: "Night", Start: "22:00", End: "02:00", Days: []string{"sun"}},
		{Name: "Early", Start: "01:00", End: "03:00", Days: []string{"mon"}},
	}}
	issues := WorkflowIssues(kiosk)
	if len(issues) != 1 || issues[0].Code != "overlapping_schedule" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestWorkflowIssuesAcceptsIndependentRules(t *testing.T) {
	kiosk := KioskConfig{TimeRules: []TimeRule{
		{Name: "Morning", Start: "08:00", End: "09:00", Days: []string{"mon"}},
		{Name: "Evening", Start: "18:00", End: "19:00", Days: []string{"mon"}},
	}}
	if issues := WorkflowIssues(kiosk); len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}
