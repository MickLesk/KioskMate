package config

import (
	"fmt"
	"sort"
	"strings"
)

type WorkflowIssue struct {
	Code     string   `json:"code"`
	Severity string   `json:"severity"`
	Pages    []string `json:"pages,omitempty"`
	Message  string   `json:"message"`
}

type workflowRule struct {
	name  string
	start int
	end   int
	days  []int
}

// WorkflowIssues returns deterministic, non-destructive diagnostics for the
// canonical page workflow. Invalid values are still rejected by Validate.
func WorkflowIssues(kiosk KioskConfig) []WorkflowIssue {
	var issues []WorkflowIssue
	seenIDs := map[string]string{}
	triggers := map[string]string{}
	rules := workflowRules(kiosk)

	for index, page := range kiosk.Pages {
		if page.Disabled {
			continue
		}
		name := workflowPageName(page, index)
		id := strings.TrimSpace(page.PageID)
		if id == "" {
			issues = append(issues, WorkflowIssue{Code: "missing_page_id", Severity: "warning", Pages: []string{name}, Message: "Page has no stable identity."})
		} else if previous, exists := seenIDs[id]; exists {
			issues = append(issues, WorkflowIssue{Code: "duplicate_page_id", Severity: "error", Pages: []string{previous, name}, Message: "Pages share the same stable identity."})
		} else {
			seenIDs[id] = name
		}
		if page.DisplayMode == "mqtt" && strings.TrimSpace(page.Trigger.Topic) != "" {
			key := strings.TrimSpace(page.Trigger.Topic) + "\x00" + page.Trigger.Payload
			if previous, exists := triggers[key]; exists {
				issues = append(issues, WorkflowIssue{Code: "duplicate_mqtt_trigger", Severity: "error", Pages: []string{previous, name}, Message: "Pages use the same MQTT trigger topic and payload."})
			} else {
				triggers[key] = name
			}
		}
	}

	for left := 0; left < len(rules); left++ {
		for right := left + 1; right < len(rules); right++ {
			if rulesOverlap(rules[left], rules[right]) {
				issues = append(issues, WorkflowIssue{
					Code: "overlapping_schedule", Severity: "warning", Pages: []string{rules[left].name, rules[right].name},
					Message: "Scheduled pages overlap; the first matching page wins.",
				})
			}
		}
	}

	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Severity != issues[j].Severity {
			return issues[i].Severity < issues[j].Severity
		}
		if issues[i].Code != issues[j].Code {
			return issues[i].Code < issues[j].Code
		}
		return strings.Join(issues[i].Pages, "\x00") < strings.Join(issues[j].Pages, "\x00")
	})
	return issues
}

func workflowRules(kiosk KioskConfig) []workflowRule {
	var out []workflowRule
	for index, rule := range kiosk.TimeRules {
		if rule.Disabled || !validClock(rule.Start) || !validClock(rule.End) {
			continue
		}
		start, _ := clockMinutes(rule.Start)
		end, _ := clockMinutes(rule.End)
		if start == end {
			continue
		}
		name := strings.TrimSpace(rule.Name)
		if name == "" {
			name = fmt.Sprintf("Rule %d", index+1)
		}
		out = append(out, workflowRule{name: name, start: start, end: end, days: workflowDays(rule.Days)})
	}
	return out
}

func workflowDays(values []string) []int {
	lookup := map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}
	seen := map[int]bool{}
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if len(key) > 3 {
			key = key[:3]
		}
		day, ok := lookup[key]
		if ok {
			seen[day] = true
		}
	}
	if len(seen) == 0 {
		return []int{0, 1, 2, 3, 4, 5, 6}
	}
	var out []int
	for day := 0; day < 7; day++ {
		if seen[day] {
			out = append(out, day)
		}
	}
	return out
}

func rulesOverlap(left, right workflowRule) bool {
	occupied := make([]bool, 7*24*60)
	markRuleMinutes(occupied, left)
	return ruleTouchesMinutes(occupied, right)
}

func markRuleMinutes(occupied []bool, rule workflowRule) {
	for _, day := range rule.days {
		for minute := rule.start; minute != rule.end; minute = (minute + 1) % (24 * 60) {
			offsetDay := day
			if rule.start > rule.end && minute < rule.end {
				offsetDay = (day + 1) % 7
			}
			occupied[offsetDay*24*60+minute] = true
		}
	}
}

func ruleTouchesMinutes(occupied []bool, rule workflowRule) bool {
	for _, day := range rule.days {
		for minute := rule.start; minute != rule.end; minute = (minute + 1) % (24 * 60) {
			offsetDay := day
			if rule.start > rule.end && minute < rule.end {
				offsetDay = (day + 1) % 7
			}
			if occupied[offsetDay*24*60+minute] {
				return true
			}
		}
	}
	return false
}

func clockMinutes(value string) (int, bool) {
	if !validClock(value) {
		return 0, false
	}
	parts := strings.Split(strings.TrimSpace(value), ":")
	var hour, minute int
	_, err := fmt.Sscanf(parts[0]+":"+parts[1], "%d:%d", &hour, &minute)
	return hour*60 + minute, err == nil
}

func workflowPageName(page KioskPage, index int) string {
	if strings.TrimSpace(page.Name) != "" {
		return strings.TrimSpace(page.Name)
	}
	return fmt.Sprintf("Page %d", index+1)
}
