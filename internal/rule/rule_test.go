package rule

import (
	"encoding/json"
	"testing"
)

func TestParseUnsetIsEmpty(t *testing.T) {
	for _, raw := range []string{"", "null"} {
		p, err := Parse(json.RawMessage(raw), json.RawMessage(raw))
		if err != nil || len(p.Paths) != 0 || p.On != "" {
			t.Errorf("Parse(%q) = %+v, %v; want zero, nil", raw, p, err)
		}
	}
}

func TestParseDecodesAndDropsBlanks(t *testing.T) {
	p, err := Parse(json.RawMessage(`["target", "", "  ", "crates/*/target"]`), json.RawMessage(`"trash"`))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Paths) != 2 || p.Paths[0] != "target" || p.Paths[1] != "crates/*/target" || p.On != "trash" {
		t.Errorf("unexpected project rule: %+v", p)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse(json.RawMessage(`{"not":"a list"}`), nil); err == nil {
		t.Error("want error for non-list paths")
	}
	if _, err := Parse(json.RawMessage(`["x"]`), json.RawMessage(`5`)); err == nil {
		t.Error("want error for non-string on")
	}
}

func TestApplies(t *testing.T) {
	paths := []string{"target"}
	tests := []struct {
		name    string
		project Project
		trigger string
		want    bool
	}{
		{"unset on means both: trash", Project{Paths: paths}, Trash, true},
		{"unset on means both: delete", Project{Paths: paths}, Delete, true},
		{"explicit both", Project{Paths: paths, On: both}, Delete, true},
		{"trash only matches trash", Project{Paths: paths, On: Trash}, Trash, true},
		{"trash only skips delete", Project{Paths: paths, On: Trash}, Delete, false},
		{"delete only skips trash", Project{Paths: paths, On: Delete}, Trash, false},
		{"no paths means no cleanup", Project{On: both}, Trash, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.project.Applies(tt.trigger); got != tt.want {
				t.Errorf("Applies = %v, want %v", got, tt.want)
			}
		})
	}
}
