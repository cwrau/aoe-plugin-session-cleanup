// Package rule models a project's cleanup configuration: the per-project
// `paths` and `on` settings, read through config.get for the session's main
// repository.
package rule

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Triggers a rule is matched against.
const (
	Trash  = "trash"
	Delete = "delete"

	both = "both"
)

// Project is what one project asks to have cleaned.
type Project struct {
	Paths []string
	On    string
}

// Parse decodes the raw `paths` and `on` values config.get returns. Unset
// values (null) mean no paths and the default trigger, not an error. Blank
// entries in the list are dropped.
func Parse(paths, on json.RawMessage) (Project, error) {
	var p Project
	if len(paths) > 0 && string(paths) != "null" {
		var all []string
		if err := json.Unmarshal(paths, &all); err != nil {
			return p, fmt.Errorf("decode paths: %w", err)
		}
		for _, path := range all {
			if strings.TrimSpace(path) != "" {
				p.Paths = append(p.Paths, path)
			}
		}
	}
	if len(on) > 0 && string(on) != "null" {
		if err := json.Unmarshal(on, &p.On); err != nil {
			return p, fmt.Errorf("decode on: %w", err)
		}
	}
	return p, nil
}

// Applies reports whether the project is cleaned for trigger (Trash or
// Delete). A project with no paths is never cleaned, and an unset `on` means
// both, matching the manifest default.
func (p Project) Applies(trigger string) bool {
	if len(p.Paths) == 0 {
		return false
	}
	return p.On == "" || p.On == both || p.On == trigger
}
