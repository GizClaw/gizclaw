package cmdhandler

import (
	"errors"
	"flag"
	"strings"
)

func Parse(fs *flag.FlagSet, argv []string) (args, flags []string, err error) {
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--" {
			args = append(args, argv[i+1:]...)
			break
		}
		if !isFlag(arg) {
			args = append(args, arg)
			continue
		}
		name, value, hasValue := splitFlag(arg)
		if fs == nil || fs.Lookup(name) == nil {
			flags = append(flags, arg)
			continue
		}
		if !hasValue {
			flagValue := fs.Lookup(name).Value
			if boolValue, ok := flagValue.(interface{ IsBoolFlag() bool }); ok && boolValue.IsBoolFlag() {
				value = "true"
			} else {
				i++
				if i >= len(argv) {
					return nil, nil, errors.New("flag needs an argument: -" + name)
				}
				value = argv[i]
			}
		}
		if err := fs.Set(name, value); err != nil {
			return nil, nil, err
		}
	}
	return append([]string(nil), args...), append([]string(nil), flags...), nil
}

func isFlag(arg string) bool {
	return len(arg) > 1 && strings.HasPrefix(arg, "-") && arg != "--"
}

func splitFlag(arg string) (name, value string, hasValue bool) {
	name = strings.TrimLeft(arg, "-")
	if name, value, ok := strings.Cut(name, "="); ok {
		return name, value, true
	}
	return name, "", false
}
