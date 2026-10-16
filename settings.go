// A project's cleanup configuration: the per-project `paths` and `on`
// settings, read through config.get for the session's main repository.
package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	triggerTrash  = "trash"
	triggerDelete = "delete"

	onBoth = "both"
)

// projectRule is what one project asks to have cleaned.
type projectRule struct {
	Paths []string
	On    string
}

// parseProjectRule decodes the raw `paths` and `on` values config.get returns.
// Unset values (null) mean no paths and the default trigger, not an error.
// Blank entries in the list are dropped.
func parseProjectRule(paths, on json.RawMessage) (projectRule, error) {
	var rule projectRule
	if len(paths) > 0 && string(paths) != "null" {
		var all []string
		if err := json.Unmarshal(paths, &all); err != nil {
			return rule, fmt.Errorf("decode paths: %w", err)
		}
		for _, p := range all {
			if strings.TrimSpace(p) != "" {
				rule.Paths = append(rule.Paths, p)
			}
		}
	}
	if len(on) > 0 && string(on) != "null" {
		if err := json.Unmarshal(on, &rule.On); err != nil {
			return rule, fmt.Errorf("decode on: %w", err)
		}
	}
	return rule, nil
}

// applies reports whether the project is cleaned for trigger (triggerTrash or
// triggerDelete). A project with no paths is never cleaned, and an unset `on`
// means both, matching the manifest default.
func (r projectRule) applies(trigger string) bool {
	if len(r.Paths) == 0 {
		return false
	}
	return r.On == "" || r.On == onBoth || r.On == trigger
}
