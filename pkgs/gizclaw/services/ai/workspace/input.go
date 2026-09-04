package workspace

import (
	"context"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

// PeerWorkspaceInputPutRequest is the transport-independent input for changing
// only the input mode of one Workspace.
type PeerWorkspaceInputPutRequest struct {
	ID    string
	Input apitypes.WorkspaceInputMode
}

// PeerWorkspaceInputPutErrorKind classifies errors for transport adapters.
type PeerWorkspaceInputPutErrorKind string

const (
	PeerWorkspaceInputPutInvalid  PeerWorkspaceInputPutErrorKind = "invalid"
	PeerWorkspaceInputPutNotFound PeerWorkspaceInputPutErrorKind = "not_found"
	PeerWorkspaceInputPutConflict PeerWorkspaceInputPutErrorKind = "conflict"
	PeerWorkspaceInputPutInternal PeerWorkspaceInputPutErrorKind = "internal"
)

// PeerWorkspaceInputPutError is a typed domain failure mapped by Peer RPC and
// other authenticated Peer-owned transports.
type PeerWorkspaceInputPutError struct {
	Kind PeerWorkspaceInputPutErrorKind
	Err  error
}

func (e *PeerWorkspaceInputPutError) Error() string {
	if e == nil || e.Err == nil {
		return "workspace: input update failed"
	}
	return e.Err.Error()
}

func (e *PeerWorkspaceInputPutError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// PeerWorkspaceInputService is the authenticated Peer-owned input-mode surface.
// The Server resolves inherited Workflow parameters itself so that callers
// never read the Workspace or its Workflow before writing.
type PeerWorkspaceInputService interface {
	PutPeerWorkspaceInput(context.Context, PeerWorkspaceInputPutRequest) (apitypes.Workspace, error)
}

// PutPeerWorkspaceInput changes only the input mode of the Workspace. Every
// other Workspace parameter and the toolkit policy keep their stored value,
// including when the Workspace inherits its parameters from the Workflow.
func (s *Server) PutPeerWorkspaceInput(ctx context.Context, request PeerWorkspaceInputPutRequest) (apitypes.Workspace, error) {
	if !request.Input.Valid() {
		return apitypes.Workspace{}, peerWorkspaceInputPutError(
			PeerWorkspaceInputPutInvalid,
			fmt.Errorf("workspace: unsupported input mode %q", request.Input),
		)
	}
	response, err := s.putWorkspaceRecord(ctx, request.ID, nil, func(previous apitypes.Workspace) (adminhttp.WorkspaceUpsert, error) {
		workflow, err := s.getWorkflow(ctx, previous.WorkflowId)
		if err != nil {
			return adminhttp.WorkspaceUpsert{}, err
		}
		parameters, err := workspaceParametersWithInput(previous.Parameters, workflow.Spec.Driver, request.Input)
		if err != nil {
			return adminhttp.WorkspaceUpsert{}, err
		}
		return adminhttp.WorkspaceUpsert{
			Id: previous.Id, Name: previous.Name, WorkflowId: previous.WorkflowId,
			Labels: previous.Labels, Parameters: parameters, Toolkit: previous.Toolkit,
		}, nil
	})
	if err != nil {
		return apitypes.Workspace{}, peerWorkspaceInputPutError(PeerWorkspaceInputPutInternal, err)
	}
	switch response := response.(type) {
	case adminhttp.PutWorkspace200JSONResponse:
		return apitypes.Workspace(response), nil
	case adminhttp.PutWorkspace400JSONResponse:
		return apitypes.Workspace{}, peerWorkspaceInputPutError(PeerWorkspaceInputPutInvalid, errors.New(response.Error.Message))
	case adminhttp.PutWorkspace404JSONResponse:
		return apitypes.Workspace{}, peerWorkspaceInputPutError(PeerWorkspaceInputPutNotFound, errors.New(response.Error.Message))
	case adminhttp.PutWorkspace409JSONResponse:
		return apitypes.Workspace{}, peerWorkspaceInputPutError(PeerWorkspaceInputPutConflict, errors.New(response.Error.Message))
	case adminhttp.PutWorkspace500JSONResponse:
		return apitypes.Workspace{}, peerWorkspaceInputPutError(PeerWorkspaceInputPutInternal, errors.New(response.Error.Message))
	default:
		return apitypes.Workspace{}, peerWorkspaceInputPutError(
			PeerWorkspaceInputPutInternal,
			fmt.Errorf("put workspace %q: unexpected response %T", request.ID, response),
		)
	}
}

// workspaceParametersWithInput projects stored Workspace parameters with the
// input mode replaced. A Workspace that inherits its parameters keeps
// inheriting every other field: only the agent_type discriminator required by
// the Workflow driver and the input override are written.
func workspaceParametersWithInput(
	parameters *apitypes.WorkspaceParameters,
	driver apitypes.WorkflowDriver,
	input apitypes.WorkspaceInputMode,
) (*apitypes.WorkspaceParameters, error) {
	variant := string(driver)
	if parameters != nil {
		discriminator, err := parameters.Discriminator()
		if err != nil {
			return nil, invalidWorkspaceReference("workspace: unreadable parameters: %v", err)
		}
		if discriminator != variant {
			return nil, invalidWorkspaceReference("workspace: parameters agent_type is %q, want %q", discriminator, variant)
		}
	}
	updated := &apitypes.WorkspaceParameters{}
	switch variant {
	case string(apitypes.WorkflowDriverAstTranslate):
		value := apitypes.ASTTranslateWorkspaceParameters{AgentType: apitypes.ASTTranslateWorkspaceParametersAgentTypeAstTranslate}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsASTTranslateWorkspaceParameters); err != nil {
			return nil, err
		}
		value.Input = &input
		return updated, updated.FromASTTranslateWorkspaceParameters(value)
	case string(apitypes.WorkflowDriverDoubaoRealtime):
		value := apitypes.DoubaoRealtimeWorkspaceParameters{AgentType: apitypes.DoubaoRealtimeWorkspaceParametersAgentTypeDoubaoRealtime}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsDoubaoRealtimeWorkspaceParameters); err != nil {
			return nil, err
		}
		value.Input = &input
		return updated, updated.FromDoubaoRealtimeWorkspaceParameters(value)
	case string(apitypes.WorkflowDriverEino):
		value := apitypes.EinoWorkspaceParameters{AgentType: apitypes.EinoWorkspaceParametersAgentTypeEino}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsEinoWorkspaceParameters); err != nil {
			return nil, err
		}
		value.Input = &input
		return updated, updated.FromEinoWorkspaceParameters(value)
	case string(apitypes.WorkflowDriverFlowcraft):
		value := apitypes.FlowcraftWorkspaceParameters{AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsFlowcraftWorkspaceParameters); err != nil {
			return nil, err
		}
		value.Input = &input
		return updated, updated.FromFlowcraftWorkspaceParameters(value)
	case string(apitypes.WorkflowDriverPet):
		value := apitypes.PetWorkspaceParameters{AgentType: apitypes.PetWorkspaceParametersAgentTypePet}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsPetWorkspaceParameters); err != nil {
			return nil, err
		}
		value.Input = &input
		return updated, updated.FromPetWorkspaceParameters(value)
	default:
		return nil, invalidWorkspaceReference("workspace: %q workspaces have no input mode", variant)
	}
}

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

func peerWorkspaceInputPutError(kind PeerWorkspaceInputPutErrorKind, err error) error {
	return &PeerWorkspaceInputPutError{Kind: kind, Err: err}
}
