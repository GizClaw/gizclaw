package runtimealias

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		alias   string
		wantErr bool
	}{
		{name: "legacy", alias: "pet-chat"},
		{name: "dotted", alias: "pet-care.extract"},
		{name: "multiple scopes", alias: "story.journey.center-earth"},
		{name: "63 bytes", alias: strings.Repeat("a", 30) + "." + strings.Repeat("b", 32)},
		{name: "empty", wantErr: true},
		{name: "64 bytes", alias: strings.Repeat("a", 31) + "." + strings.Repeat("b", 32), wantErr: true},
		{name: "leading dot", alias: ".voice", wantErr: true},
		{name: "trailing dot", alias: "raid.", wantErr: true},
		{name: "empty segment", alias: "raid..voice", wantErr: true},
		{name: "leading segment hyphen", alias: "raid.-voice", wantErr: true},
		{name: "trailing segment hyphen", alias: "raid-.voice", wantErr: true},
		{name: "underscore", alias: "story.journey_center_earth", wantErr: true},
		{name: "uppercase", alias: "Story.journey", wantErr: true},
		{name: "slash", alias: "story/journey", wantErr: true},
		{name: "leading whitespace", alias: " story.journey", wantErr: true},
		{name: "internal whitespace", alias: "story. journey", wantErr: true},
		{name: "trailing whitespace", alias: "story.journey ", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate("test alias", test.alias)
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate(%q) error = %v, wantErr %t", test.alias, err, test.wantErr)
			}
			if test.wantErr && (err == nil || !strings.Contains(err.Error(), "1-63 bytes of dot-separated lowercase kebab-case segments")) {
				t.Fatalf("Validate(%q) error = %v, want byte and segment grammar", test.alias, err)
			}
		})
	}
}
