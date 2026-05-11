package service

import "fmt"

func RuntimeWorkspaceFromArgs(args []string) (string, bool, error) {
	for i := range args {
		arg := args[i]
		if arg == InternalRunFlag {
			if i+1 >= len(args) {
				return "", false, fmt.Errorf("service: missing workspace for %s", InternalRunFlag)
			}
			return args[i+1], true, nil
		}
	}
	return "", false, nil
}
