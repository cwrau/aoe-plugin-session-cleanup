package main

import (
	"encoding/json"
	"testing"
)

func TestParseProjectRuleUnsetIsEmpty(t *testing.T) {
	for _, raw := range []string{"", "null"} {
		rule, err := parseProjectRule(json.RawMessage(raw), json.RawMessage(raw))
		if err != nil || len(rule.Paths) != 0 || rule.On != "" {
			t.Errorf("parseProjectRule(%q) = %+v, %v; want zero, nil", raw, rule, err)
		}
	}
}

func TestParseProjectRuleDecodesAndDropsBlanks(t *testing.T) {
	rule, err := parseProjectRule(json.RawMessage(`["target", "", "  ", "crates/*/target"]`), json.RawMessage(`"trash"`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rule.Paths) != 2 || rule.Paths[0] != "target" || rule.Paths[1] != "crates/*/target" || rule.On != "trash" {
		t.Errorf("unexpected rule: %+v", rule)
	}
}

func TestParseProjectRuleRejectsGarbage(t *testing.T) {
	if _, err := parseProjectRule(json.RawMessage(`{"not":"a list"}`), nil); err == nil {
		t.Error("want error for non-list paths")
	}
	if _, err := parseProjectRule(json.RawMessage(`["x"]`), json.RawMessage(`5`)); err == nil {
		t.Error("want error for non-string on")
	}
}

func TestProjectRuleApplies(t *testing.T) {
	paths := []string{"target"}
	tests := []struct {
		name    string
		rule    projectRule
		trigger string
		want    bool
	}{
		{"unset on means both: trash", projectRule{Paths: paths}, triggerTrash, true},
		{"unset on means both: delete", projectRule{Paths: paths}, triggerDelete, true},
		{"explicit both", projectRule{Paths: paths, On: onBoth}, triggerDelete, true},
		{"trash only matches trash", projectRule{Paths: paths, On: triggerTrash}, triggerTrash, true},
		{"trash only skips delete", projectRule{Paths: paths, On: triggerTrash}, triggerDelete, false},
		{"delete only skips trash", projectRule{Paths: paths, On: triggerDelete}, triggerTrash, false},
		{"no paths means no cleanup", projectRule{On: onBoth}, triggerTrash, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rule.applies(tt.trigger); got != tt.want {
				t.Errorf("applies = %v, want %v", got, tt.want)
			}
		})
	}
}
