package config

import (
	"slices"
	"strconv"
	"strings"
)

// Conflict is a saved configuration that works but probably does not do what
// was meant: two schedules claiming the same time, or two rules reacting to
// the same event. The Admin UI shows them as warnings after saving.
type Conflict struct {
	// Kind is "time_rules_overlap", "rules_same_trigger" or "rule_page_topic".
	Kind   string `json:"kind"`
	First  string `json:"first"`
	Second string `json:"second"`
}

// Conflicts lists overlapping time rules and workflow rules that share a
// trigger. Priority is fixed (manual override > workflow > schedule >
// rotation), so these are warnings, not errors.
func (k KioskConfig) Conflicts() []Conflict {
	var out []Conflict
	rules := make([]TimeRule, 0, len(k.TimeRules))
	for _, rule := range k.TimeRules {
		if !rule.Disabled {
			rules = append(rules, rule)
		}
	}
	for i := range rules {
		for j := i + 1; j < len(rules); j++ {
			if rules[i].Page != rules[j].Page && timeRulesOverlap(rules[i], rules[j]) {
				out = append(out, Conflict{Kind: "time_rules_overlap", First: timeRuleLabel(rules[i], i), Second: timeRuleLabel(rules[j], j)})
			}
		}
	}
	var workflows []WorkflowRule
	for _, rule := range k.Rules {
		if !rule.Disabled {
			workflows = append(workflows, rule)
		}
	}
	for i := range workflows {
		for j := i + 1; j < len(workflows); j++ {
			if sameTrigger(workflows[i].Trigger, workflows[j].Trigger) {
				out = append(out, Conflict{Kind: "rules_same_trigger", First: ruleLabel(workflows[i]), Second: ruleLabel(workflows[j])})
			}
		}
		if workflows[i].Trigger.Type != "mqtt" {
			continue
		}
		for _, page := range k.Pages {
			if !page.Disabled && page.DisplayMode == "mqtt" && strings.TrimSpace(page.Trigger.Topic) == strings.TrimSpace(workflows[i].Trigger.Topic) {
				out = append(out, Conflict{Kind: "rule_page_topic", First: ruleLabel(workflows[i]), Second: firstNonEmpty(page.Name, page.URL)})
			}
		}
	}
	return out
}

func sameTrigger(a, b RuleTrigger) bool {
	if a.Type != b.Type {
		return false
	}
	switch a.Type {
	case "mqtt":
		if strings.TrimSpace(a.Topic) != strings.TrimSpace(b.Topic) {
			return false
		}
		pa, pb := strings.TrimSpace(a.Payload), strings.TrimSpace(b.Payload)
		return pa == "" || pb == "" || strings.EqualFold(pa, pb)
	case "time":
		return a.At == b.At && daysOverlap(a.Days, b.Days)
	case "touch":
		return true
	case "idle":
		return a.IdleSeconds == b.IdleSeconds
	}
	return false
}

func timeRulesOverlap(a, b TimeRule) bool {
	if !daysOverlap(a.Days, b.Days) {
		return false
	}
	as, ae, ok1 := clockMinutes(a.Start), clockMinutes(a.End), validClock(a.Start) && validClock(a.End)
	bs, be, ok2 := clockMinutes(b.Start), clockMinutes(b.End), validClock(b.Start) && validClock(b.End)
	if !ok1 || !ok2 {
		return false
	}
	for _, x := range clockSegments(as, ae) {
		for _, y := range clockSegments(bs, be) {
			if x[0] < y[1] && y[0] < x[1] {
				return true
			}
		}
	}
	return false
}

// clockSegments splits a window that may cross midnight into same-day
// [start, end) minute ranges.
func clockSegments(start, end int) [][2]int {
	if start == end {
		return [][2]int{{0, 24 * 60}}
	}
	if start < end {
		return [][2]int{{start, end}}
	}
	return [][2]int{{start, 24 * 60}, {0, end}}
}

func daysOverlap(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return true
	}
	for _, day := range a {
		if slices.ContainsFunc(b, func(other string) bool { return shortDay(other) == shortDay(day) }) {
			return true
		}
	}
	return false
}

func shortDay(day string) string {
	day = strings.ToLower(strings.TrimSpace(day))
	if len(day) > 3 {
		return day[:3]
	}
	return day
}

func clockMinutes(value string) int {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0
	}
	hour, _ := strconv.Atoi(parts[0])
	minute, _ := strconv.Atoi(parts[1])
	return hour*60 + minute
}

func timeRuleLabel(rule TimeRule, index int) string {
	return firstNonEmpty(rule.Name, "time rule "+strconv.Itoa(index+1)) + " (" + rule.Start + "-" + rule.End + ")"
}

func ruleLabel(rule WorkflowRule) string {
	return firstNonEmpty(rule.Name, rule.ID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
