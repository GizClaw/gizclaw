package workspace

import "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

func decodeWorkspaceParametersVariant[T any](
	parameters *apitypes.WorkspaceParameters,
	out *T,
	as func(apitypes.WorkspaceParameters) (T, error),
) error {
	if parameters == nil {
		return nil
	}
	value, err := as(*parameters)
	if err != nil {
		return invalidWorkspaceReference("workspace: unreadable parameters: %v", err)
	}
	*out = value
	return nil
}
