package eino

import "testing"

func TestSafetyFenceBindingReachesChildGraphs(t *testing.T) {
	fields := map[string]StateType{}
	if err := validateBinding(Binding{From: "input.safety_fence"}, fields); err != nil {
		t.Fatalf("validateBinding() error = %v", err)
	}
	if stateType, err := bindingStateType(Binding{From: "input.safety_fence"}, fields); err != nil || stateType != StateString {
		t.Fatalf("bindingStateType() = %v, %v; want string", stateType, err)
	}
	for _, fence := range []string{"", "profile fence"} {
		parent, err := newRunState(nil, graphInput{ObservationID: "turn", SafetyFence: fence}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		// Children receive node data explicitly, but the fence is a run-wide
		// host value, so batch, race and subgraph runs see the same text.
		child, err := newRunState(nil, graphInputFromNodeInputs(map[string]any{"text": "item"}, parent.input, "turn:child"), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		for name, state := range map[string]*runState{"parent": parent, "child": child} {
			got, err := state.binding(Binding{From: "input.safety_fence"})
			if err != nil || got != fence {
				t.Fatalf("%s binding = %#v, %v; want %q", name, got, err, fence)
			}
		}
	}
}
