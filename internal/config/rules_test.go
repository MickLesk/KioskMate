package config

import (
	"strings"
	"testing"
)

func TestValidateRules(t *testing.T) {
	base := func() KioskConfig {
		return KioskConfig{
			Pages: []KioskPage{{PageID: "cam"}},
			Rules: []WorkflowRule{{
				ID:      "doorbell",
				Trigger: RuleTrigger{Type: "webhook", Token: "0123456789abcdef"},
				Actions: []RuleAction{{Type: "show_page", PageID: "cam", DurationSeconds: 30}},
			}},
		}
	}
	if err := validateRules(base()); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*KioskConfig){
		"id must be":        func(k *KioskConfig) { k.Rules[0].ID = "a/b" },
		"used twice":        func(k *KioskConfig) { k.Rules = append(k.Rules, k.Rules[0]) },
		"at least 16":       func(k *KioskConfig) { k.Rules[0].Trigger.Token = "short" },
		"without wildcards": func(k *KioskConfig) { k.Rules[0].Trigger = RuleTrigger{Type: "mqtt", Topic: "home/#"} },
		"HH:MM":             func(k *KioskConfig) { k.Rules[0].Trigger = RuleTrigger{Type: "time", At: "25:00"} },
		"unknown page":      func(k *KioskConfig) { k.Rules[0].Actions[0].PageID = "nope" },
		"start and end":     func(k *KioskConfig) { k.Rules[0].Conditions.Start = "08:00" },
		"between 1 and 100": func(k *KioskConfig) { k.Rules[0].Actions[0] = RuleAction{Type: "brightness", Value: "0"} },
		"at least one":      func(k *KioskConfig) { k.Rules[0].Actions = nil },
		"idle trigger":      func(k *KioskConfig) { k.Rules[0].Trigger = RuleTrigger{Type: "idle", IdleSeconds: 5} },
		"http(s) URL":       func(k *KioskConfig) { k.Rules[0].Actions[0] = RuleAction{Type: "sound", Value: "file:///etc/passwd"} },
		"CSS selector":      func(k *KioskConfig) { k.Rules[0].Actions[0] = RuleAction{Type: "click"} },
	}
	for want, mutate := range cases {
		kiosk := base()
		mutate(&kiosk)
		if err := validateRules(kiosk); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v", want, err)
		}
	}
}
