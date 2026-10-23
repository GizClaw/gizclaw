package rpcapi

import (
	"fmt"
	"strings"
	"unicode/utf8"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func managementText(value string, limit int) bool {
	return value != "" && len(value) <= limit && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

// ValidateManagementRequest checks the typed management contract without
// exposing sensitive input in errors. Providers own support and enforcement.
func ValidateManagementRequest(message proto.Message) error {
	valid := true
	switch request := message.(type) {
	case *rpcpb.ClientContentAgeSetRequest:
		valid = managementText(request.GetOptionId(), 64)
	case *rpcpb.ClientDeviceFeatureSetRequest:
		valid = managementText(request.GetFeatureId(), 64) && request.Enabled != nil
	case *rpcpb.ClientUsageTimeSetRequest:
		valid = request != nil && request.LimitSeconds != nil
	case *rpcpb.ClientAISafetySetRequest:
		valid = managementText(request.GetLevel(), 64)
	case *rpcpb.ClientManagementPasswordSetRequest:
		valid = managementText(request.GetPassword(), 128)
	}
	if !valid {
		return fmt.Errorf("rpc: invalid management arguments")
	}
	return nil
}

func validContentAge(state *rpcpb.ContentAgeState) bool {
	if state == nil || !managementText(state.OptionId, 64) || len(state.Options) == 0 || len(state.Options) > 16 {
		return false
	}
	seen, found := map[string]bool{}, false
	for _, option := range state.Options {
		if option == nil || !managementText(option.Id, 64) || !managementText(option.DisplayName, 128) || seen[option.Id] {
			return false
		}
		if option.MinAgeYears != nil && option.MaxAgeYears != nil && *option.MinAgeYears > *option.MaxAgeYears {
			return false
		}
		seen[option.Id] = true
		found = found || state.OptionId == option.Id
	}
	return found
}

func validFeature(feature *rpcpb.DeviceFeature) bool {
	return feature != nil && managementText(feature.Id, 64) && managementText(feature.DisplayName, 128) && feature.Enabled != nil && feature.Writable != nil
}

func validUsage(state *rpcpb.UsageTimeState) bool {
	return state != nil && state.LimitSeconds != nil && state.UsedSeconds != nil && state.Exhausted != nil &&
		*state.Exhausted == (*state.LimitSeconds > 0 && *state.UsedSeconds >= *state.LimitSeconds)
}

func validAISafety(state *rpcpb.AISafetyState) bool {
	if state == nil || !managementText(state.Level, 64) || len(state.Options) == 0 || len(state.Options) > 16 {
		return false
	}
	seen, found := map[string]bool{}, false
	for _, option := range state.Options {
		if option == nil || !managementText(option.Id, 64) || !managementText(option.DisplayName, 128) || seen[option.Id] {
			return false
		}
		seen[option.Id] = true
		found = found || state.Level == option.Id
	}
	return found
}

// ValidateManagementResponse rejects missing applied state, ambiguous false,
// unsupported options and contradictory usage results. It cannot attest to
// physical enforcement; the device provider must read its authoritative state.
func ValidateManagementResponse(message proto.Message) error {
	if message == nil || !message.ProtoReflect().IsValid() {
		return fmt.Errorf("rpc: invalid management result")
	}
	valid := true
	switch response := message.(type) {
	case *rpcpb.ClientContentAgeGetResponse:
		valid = validContentAge(response.GetState())
	case *rpcpb.ClientContentAgeSetResponse:
		valid = validContentAge(response.GetApplied())
	case *rpcpb.ClientDeviceFeaturesGetResponse:
		valid = len(response.GetFeatures()) <= 32
		seen := map[string]bool{}
		for _, feature := range response.GetFeatures() {
			if !validFeature(feature) || seen[feature.GetId()] {
				valid = false
				break
			}
			seen[feature.Id] = true
		}
	case *rpcpb.ClientDeviceFeatureSetResponse:
		valid = validFeature(response.GetApplied()) && response.GetApplied().GetWritable()
	case *rpcpb.ClientUsageTimeGetResponse:
		valid = validUsage(response.GetState())
	case *rpcpb.ClientUsageTimeSetResponse:
		valid = validUsage(response.GetApplied())
	case *rpcpb.ClientAISafetyGetResponse:
		valid = validAISafety(response.GetState())
	case *rpcpb.ClientAISafetySetResponse:
		valid = validAISafety(response.GetApplied())
	case *rpcpb.ClientManagementPasswordGetResponse:
		valid = response != nil && response.IsSet != nil
	case *rpcpb.ClientManagementPasswordSetResponse:
		valid = response != nil && response.IsSet != nil && *response.IsSet
	case *rpcpb.ClientManagementPasswordResetResponse:
		valid = response != nil && response.IsSet != nil && !*response.IsSet
	}
	if !valid {
		return fmt.Errorf("rpc: invalid management result")
	}
	return nil
}

// ValidateManagementResult also binds instance-selecting results to the
// requested feature, so a provider cannot acknowledge a different feature.
func ValidateManagementResult(request, response proto.Message) error {
	if err := ValidateManagementResponse(response); err != nil {
		return err
	}
	if input, ok := request.(*rpcpb.ClientDeviceFeatureSetRequest); ok {
		output, ok := response.(*rpcpb.ClientDeviceFeatureSetResponse)
		if !ok || output.GetApplied().GetId() != input.GetFeatureId() {
			return fmt.Errorf("rpc: invalid management result")
		}
	}
	return nil
}
