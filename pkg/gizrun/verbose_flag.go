package gizrun

import (
	"log/slog"
	"strings"
)

type verboseFlag struct {
	level slog.Level
	set   bool
}

func (v verboseFlag) String() string {
	if !v.set {
		return ""
	}
	return v.level.String()
}

func (v *verboseFlag) Set(value string) error {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToLower(strings.TrimSpace(value)))); err != nil {
		return err
	}
	if v != nil {
		v.level = level
		v.set = true
	}
	return nil
}
