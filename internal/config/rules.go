package config

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// WorkflowRule reacts to an event with a list of actions on top of the
// page playlist (rotation and time rules): "when the doorbell rings, show the
// camera page for 30 seconds, then return to the playlist".
type WorkflowRule struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Disabled        bool           `json:"disabled,omitempty"`
	Trigger         RuleTrigger    `json:"trigger"`
	Conditions      RuleConditions `json:"conditions,omitempty"`
	Actions         []RuleAction   `json:"actions"`
	CooldownSeconds int            `json:"cooldown_seconds,omitempty"`
}

// RuleTrigger types:
//   - "mqtt": Topic, optional Payload (empty matches any payload)
//   - "time": At (HH:MM) on Days (empty = every day)
//   - "webhook": POST /api/rules/<id>/trigger?token=<Token>
//   - "touch": any touch, click, key or wheel input on the kiosk page
//   - "idle": no input for IdleSeconds (fires once, re-armed by input)
type RuleTrigger struct {
	Type        string   `json:"type"`
	Topic       string   `json:"topic,omitempty"`
	Payload     string   `json:"payload,omitempty"`
	At          string   `json:"at,omitempty"`
	Days        []string `json:"days,omitempty"`
	Token       string   `json:"token,omitempty"`
	IdleSeconds int      `json:"idle_seconds,omitempty"`
}

// RuleConditions must all hold when the trigger fires.
type RuleConditions struct {
	Days         []string `json:"days,omitempty"`
	Start        string   `json:"start,omitempty"`
	End          string   `json:"end,omitempty"`
	ActivePageID string   `json:"active_page_id,omitempty"`
}

// RuleAction types:
//   - "show_page": PageID, DurationSeconds (0 = 1 hour) or UntilResume, then
//     back to the playlist
//   - "resume_playlist": end a page shown by a rule
//   - "display": Value ON or OFF; with DurationSeconds the opposite state
//     follows after that time ("motion: display on for 2 minutes")
//   - "brightness": Value 1-100
//   - "reload": reload the current page
//   - "overlay": Value text shown as a banner for DurationSeconds (default 10)
//   - "script": Value JavaScript evaluated in the current page
//   - "click": Value CSS selector of the element to click
//   - "scroll": Value pixels (e.g. 400 or -400) or a CSS selector to scroll to
//   - "sound": Value audio URL, or empty for a short chime
type RuleAction struct {
	Type            string `json:"type"`
	PageID          string `json:"page_id,omitempty"`
	DurationSeconds int    `json:"duration_seconds,omitempty"`
	UntilResume     bool   `json:"until_resume,omitempty"`
	Value           string `json:"value,omitempty"`
}

var ruleTriggerTypes = []string{"mqtt", "time", "webhook", "touch", "idle"}
var ruleActionTypes = []string{"show_page", "resume_playlist", "display", "brightness", "reload", "overlay", "script", "click", "scroll", "sound"}

// EnabledPageIndex maps a stable page ID to the index the browser uses (the
// position among enabled pages), or -1.
func (k KioskConfig) EnabledPageIndex(pageID string) int {
	pageID = strings.TrimSpace(pageID)
	if pageID == "" {
		return -1
	}
	for index, page := range k.enabledPages() {
		if page.PageID == pageID {
			return index
		}
	}
	return -1
}

func cloneRules(rules []WorkflowRule) []WorkflowRule {
	out := slices.Clone(rules)
	for i := range out {
		out[i].Trigger.Days = slices.Clone(rules[i].Trigger.Days)
		out[i].Conditions.Days = slices.Clone(rules[i].Conditions.Days)
		out[i].Actions = slices.Clone(rules[i].Actions)
	}
	return out
}

func validateRules(kiosk KioskConfig) error {
	pageIDs := map[string]bool{}
	for _, page := range kiosk.Pages {
		pageIDs[page.PageID] = true
	}
	seen := map[string]bool{}
	for index, rule := range kiosk.Rules {
		label := strings.TrimSpace(rule.Name)
		if label == "" {
			label = "rule " + strconv.Itoa(index+1)
		}
		fail := func(format string, args ...any) error {
			return fmt.Errorf("workflow %s: %s", label, fmt.Sprintf(format, args...))
		}
		if !validRuleID(rule.ID) {
			return fail("id must be 1-64 letters, digits, '-' or '_'")
		}
		if seen[rule.ID] {
			return fail("id %q is used twice", rule.ID)
		}
		seen[rule.ID] = true
		switch rule.Trigger.Type {
		case "mqtt":
			if strings.TrimSpace(rule.Trigger.Topic) == "" || strings.ContainsAny(rule.Trigger.Topic, "+#") {
				return fail("MQTT trigger needs a topic without wildcards")
			}
		case "time":
			if !validClock(rule.Trigger.At) {
				return fail("time trigger needs HH:MM")
			}
		case "webhook":
			if len(strings.TrimSpace(rule.Trigger.Token)) < 16 {
				return fail("webhook trigger needs a token of at least 16 characters")
			}
		case "touch":
		case "idle":
			if rule.Trigger.IdleSeconds < 10 || rule.Trigger.IdleSeconds > 86400 {
				return fail("idle trigger needs between 10 and 86400 seconds")
			}
		default:
			return fail("trigger must be one of %s", strings.Join(ruleTriggerTypes, ", "))
		}
		if (rule.Conditions.Start == "") != (rule.Conditions.End == "") {
			return fail("condition time window needs start and end")
		}
		if rule.Conditions.Start != "" && (!validClock(rule.Conditions.Start) || !validClock(rule.Conditions.End)) {
			return fail("condition times must use HH:MM")
		}
		if id := rule.Conditions.ActivePageID; id != "" && !pageIDs[id] {
			return fail("condition refers to an unknown page")
		}
		if rule.CooldownSeconds < 0 || rule.CooldownSeconds > 86400 {
			return fail("cooldown must be between 0 and 86400 seconds")
		}
		if len(rule.Actions) == 0 {
			return fail("at least one action is required")
		}
		for _, action := range rule.Actions {
			if err := validateRuleAction(action, pageIDs); err != nil {
				return fail("%v", err)
			}
		}
	}
	return nil
}

func validRuleID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func validateRuleAction(action RuleAction, pageIDs map[string]bool) error {
	if action.DurationSeconds < 0 || action.DurationSeconds > 86400 {
		return errors.New("action duration must be between 0 and 86400 seconds")
	}
	switch action.Type {
	case "show_page":
		if !pageIDs[action.PageID] {
			return errors.New("show_page refers to an unknown page")
		}
	case "display":
		if value := strings.ToUpper(strings.TrimSpace(action.Value)); value != "ON" && value != "OFF" {
			return errors.New("display action needs ON or OFF")
		}
	case "brightness":
		value, err := strconv.Atoi(strings.TrimSpace(action.Value))
		if err != nil || value < 1 || value > 100 {
			return errors.New("brightness action needs a value between 1 and 100")
		}
	case "overlay":
		if strings.TrimSpace(action.Value) == "" || len(action.Value) > 500 {
			return errors.New("overlay action needs text up to 500 characters")
		}
	case "script":
		if strings.TrimSpace(action.Value) == "" || len(action.Value) > 8000 {
			return errors.New("script action needs JavaScript up to 8000 characters")
		}
	case "click":
		if strings.TrimSpace(action.Value) == "" || len(action.Value) > 500 {
			return errors.New("click action needs a CSS selector")
		}
	case "scroll":
		if strings.TrimSpace(action.Value) == "" || len(action.Value) > 500 {
			return errors.New("scroll action needs pixels or a CSS selector")
		}
	case "sound":
		if value := strings.TrimSpace(action.Value); value != "" && !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
			return errors.New("sound action needs an http(s) URL or nothing for the default chime")
		}
	case "resume_playlist", "reload":
	default:
		return fmt.Errorf("action must be one of %s", strings.Join(ruleActionTypes, ", "))
	}
	return nil
}
