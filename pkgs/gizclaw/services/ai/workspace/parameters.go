package workspace

import (
	"context"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

// PeerWorkspaceParametersSetRequest contains driver-neutral Workspace parameter updates.
// Nil fields are preserved from the stored parameters.
type PeerWorkspaceParametersSetRequest struct {
	ID           string
	Input        *apitypes.WorkspaceInputMode
	Conversation *apitypes.ConversationParameters
}

// PeerWorkspaceParametersSetErrorKind classifies errors for transport adapters.
type PeerWorkspaceParametersSetErrorKind string

const (
	PeerWorkspaceParametersSetInvalid  PeerWorkspaceParametersSetErrorKind = "invalid"
	PeerWorkspaceParametersSetNotFound PeerWorkspaceParametersSetErrorKind = "not_found"
	PeerWorkspaceParametersSetConflict PeerWorkspaceParametersSetErrorKind = "conflict"
	PeerWorkspaceParametersSetInternal PeerWorkspaceParametersSetErrorKind = "internal"
)

// PeerWorkspaceParametersSetError is a typed domain failure mapped by Peer transports.
type PeerWorkspaceParametersSetError struct {
	Kind PeerWorkspaceParametersSetErrorKind
	Err  error
}

func (e *PeerWorkspaceParametersSetError) Error() string {
	if e == nil || e.Err == nil {
		return "workspace: parameter update failed"
	}
	return e.Err.Error()
}

func (e *PeerWorkspaceParametersSetError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// PeerWorkspaceParametersService is the authenticated Peer-owned parameter update surface.
type PeerWorkspaceParametersService interface {
	SetPeerWorkspaceParameters(context.Context, PeerWorkspaceParametersSetRequest) (apitypes.Workspace, error)
}

// SetPeerWorkspaceParameters updates supported fields while deriving agent_type
// from the Workspace's bound Workflow driver.
func (s *Server) SetPeerWorkspaceParameters(ctx context.Context, request PeerWorkspaceParametersSetRequest) (apitypes.Workspace, error) {
	if err := validateWorkspaceParametersPatch(request); err != nil {
		return apitypes.Workspace{}, peerWorkspaceParametersSetError(PeerWorkspaceParametersSetInvalid, err)
	}
	response, err := s.putWorkspaceRecord(ctx, request.ID, nil, func(previous apitypes.Workspace) (adminhttp.WorkspaceUpsert, error) {
		workflow, err := s.getWorkflow(ctx, previous.WorkflowId)
		if err != nil {
			return adminhttp.WorkspaceUpsert{}, err
		}
		parameters, err := workspaceParametersWithPatch(previous.Parameters, workflow.Spec.Driver, request.Input, request.Conversation)
		if err != nil {
			return adminhttp.WorkspaceUpsert{}, err
		}
		return adminhttp.WorkspaceUpsert{
			Id: previous.Id, Name: previous.Name, WorkflowId: previous.WorkflowId,
			Labels: previous.Labels, Parameters: parameters, Toolkit: previous.Toolkit,
		}, nil
	})
	if err != nil {
		return apitypes.Workspace{}, peerWorkspaceParametersSetError(PeerWorkspaceParametersSetInternal, err)
	}
	switch response := response.(type) {
	case adminhttp.PutWorkspace200JSONResponse:
		return apitypes.Workspace(response), nil
	case adminhttp.PutWorkspace400JSONResponse:
		return apitypes.Workspace{}, peerWorkspaceParametersSetError(PeerWorkspaceParametersSetInvalid, errors.New(response.Error.Message))
	case adminhttp.PutWorkspace404JSONResponse:
		return apitypes.Workspace{}, peerWorkspaceParametersSetError(PeerWorkspaceParametersSetNotFound, errors.New(response.Error.Message))
	case adminhttp.PutWorkspace409JSONResponse:
		return apitypes.Workspace{}, peerWorkspaceParametersSetError(PeerWorkspaceParametersSetConflict, errors.New(response.Error.Message))
	case adminhttp.PutWorkspace500JSONResponse:
		return apitypes.Workspace{}, peerWorkspaceParametersSetError(PeerWorkspaceParametersSetInternal, errors.New(response.Error.Message))
	default:
		return apitypes.Workspace{}, peerWorkspaceParametersSetError(
			PeerWorkspaceParametersSetInternal,
			fmt.Errorf("put workspace %q: unexpected response %T", request.ID, response),
		)
	}
}

func validateWorkspaceParametersPatch(request PeerWorkspaceParametersSetRequest) error {
	if request.Input == nil && request.Conversation == nil {
		return errors.New("workspace: at least one parameter is required")
	}
	if request.Input != nil && !request.Input.Valid() {
		return fmt.Errorf("workspace: unsupported input mode %q", *request.Input)
	}
	if request.Conversation == nil {
		return nil
	}
	conversation := request.Conversation
	if conversation.Initiative == nil && conversation.AgentInitiativePolicy == nil {
		return errors.New("workspace: conversation update must not be empty")
	}
	if conversation.Initiative != nil && !conversation.Initiative.Valid() {
		return fmt.Errorf("workspace: unsupported conversation initiative %q", *conversation.Initiative)
	}
	if conversation.AgentInitiativePolicy != nil && !conversation.AgentInitiativePolicy.Valid() {
		return fmt.Errorf("workspace: unsupported agent initiative policy %q", *conversation.AgentInitiativePolicy)
	}
	return nil
}

func workspaceParametersWithPatch(
	parameters *apitypes.WorkspaceParameters,
	driver apitypes.WorkflowDriver,
	input *apitypes.WorkspaceInputMode,
	conversation *apitypes.ConversationParameters,
) (*apitypes.WorkspaceParameters, error) {
	if conversation == nil {
		if input == nil {
			return nil, invalidWorkspaceReference("workspace: at least one parameter is required")
		}
		return workspaceParametersWithInput(parameters, driver, *input)
	}
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
	switch driver {
	case apitypes.WorkflowDriverEino:
		value := apitypes.EinoWorkspaceParameters{AgentType: apitypes.EinoWorkspaceParametersAgentTypeEino}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsEinoWorkspaceParameters); err != nil {
			return nil, err
		}
		value.Conversation = mergeConversationParameters(value.Conversation, conversation)
		if input != nil {
			value.Input = input
		}
		return updated, updated.FromEinoWorkspaceParameters(value)
	case apitypes.WorkflowDriverFlowcraft:
		value := apitypes.FlowcraftWorkspaceParameters{AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsFlowcraftWorkspaceParameters); err != nil {
			return nil, err
		}
		value.Conversation = mergeConversationParameters(value.Conversation, conversation)
		if input != nil {
			value.Input = input
		}
		return updated, updated.FromFlowcraftWorkspaceParameters(value)
	default:
		return nil, invalidWorkspaceReference("workspace: %q workspaces do not support conversation parameters", variant)
	}
}

func mergeConversationParameters(current, patch *apitypes.ConversationParameters) *apitypes.ConversationParameters {
	result := &apitypes.ConversationParameters{}
	if current != nil {
		*result = *current
	}
	if patch.Initiative != nil {
		value := *patch.Initiative
		result.Initiative = &value
	}
	if patch.AgentInitiativePolicy != nil {
		value := *patch.AgentInitiativePolicy
		result.AgentInitiativePolicy = &value
	}
	return result
}

func peerWorkspaceParametersSetError(kind PeerWorkspaceParametersSetErrorKind, err error) error {
	return &PeerWorkspaceParametersSetError{Kind: kind, Err: err}
}
