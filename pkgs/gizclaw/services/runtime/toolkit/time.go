package toolkit

import "time"

func cloneTimePtr(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
