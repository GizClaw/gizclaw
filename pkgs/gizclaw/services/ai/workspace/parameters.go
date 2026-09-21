package workspace

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

// PeerWorkspaceParametersSetRequest contains driver-neutral Workspace parameter updates.
// Nil fields are preserved from the stored parameters.
type PeerWorkspaceParametersSetRequest struct {
	ID                   string
	Input                *apitypes.WorkspaceInputMode
	Conversation         *apitypes.ConversationParameters
	TTSSpeechRatePercent *int
	SafetyFenceLevel     *apitypes.SafetyFenceLevel
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

var errWorkspaceParametersUnchanged = errors.New("workspace: parameters unchanged")

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
		parameters, err := workspaceParametersWithPatch(previous.Parameters, workflow.Spec.Driver, request.Input, request.Conversation, request.TTSSpeechRatePercent, request.SafetyFenceLevel)
		if err != nil {
			return adminhttp.WorkspaceUpsert{}, err
		}
		desired := adminhttp.WorkspaceUpsert{
			Id: previous.Id, Name: previous.Name, WorkflowId: previous.WorkflowId,
			Labels: previous.Labels, Parameters: parameters, Toolkit: previous.Toolkit,
		}
		if reflect.DeepEqual(previous.Parameters, parameters) ||
			(workspaceIsSystem(previous) && !systemWorkspaceAllowsInputUpdate(previous, desired)) {
			return adminhttp.WorkspaceUpsert{}, errWorkspaceParametersUnchanged
		}
		return desired, nil
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
	if request.Input == nil && request.Conversation == nil && request.TTSSpeechRatePercent == nil && request.SafetyFenceLevel == nil {
		return errors.New("workspace: at least one parameter is required")
	}
	if request.Input != nil && !request.Input.Valid() {
		return fmt.Errorf("workspace: unsupported input mode %q", *request.Input)
	}
	if err := apitypes.ValidateTTSSpeechRatePercent(request.TTSSpeechRatePercent); err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	if err := apitypes.ValidateSafetyFenceLevel(request.SafetyFenceLevel); err != nil {
		return fmt.Errorf("workspace: %w", err)
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

// workspaceParametersWithPatch projects stored Workspace parameters with the
// patch applied. A Workspace that inherits its parameters keeps inheriting
// every other field: only the agent_type discriminator required by the Workflow
// driver and the patched fields are written. Fields a driver does not support
// are ignored, and a patch with no supported field preserves the original
// parameters without an update.
func workspaceParametersWithPatch(
	parameters *apitypes.WorkspaceParameters,
	driver apitypes.WorkflowDriver,
	input *apitypes.WorkspaceInputMode,
	conversation *apitypes.ConversationParameters,
	ttsSpeechRatePercent *int,
	safetyFenceLevel *apitypes.SafetyFenceLevel,
) (*apitypes.WorkspaceParameters, error) {
	if input == nil && conversation == nil && ttsSpeechRatePercent == nil && safetyFenceLevel == nil {
		return nil, invalidWorkspaceReference("workspace: at least one parameter is required")
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
	rate := cloneInt(ttsSpeechRatePercent)
	updated := &apitypes.WorkspaceParameters{}
	switch driver {
	case apitypes.WorkflowDriverEino:
		value := apitypes.EinoWorkspaceParameters{AgentType: apitypes.EinoWorkspaceParametersAgentTypeEino}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsEinoWorkspaceParameters); err != nil {
			return nil, err
		}
		patchInput(&value.Input, input)
		patchConversation(&value.Conversation, conversation)
		patchRate(&value.TtsSpeechRatePercent, rate)
		if safetyFenceLevel != nil {
			value.SafetyFenceLevel = new(*safetyFenceLevel)
		}
		return updated, updated.FromEinoWorkspaceParameters(value)
	case apitypes.WorkflowDriverFlowcraft:
		value := apitypes.FlowcraftWorkspaceParameters{AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsFlowcraftWorkspaceParameters); err != nil {
			return nil, err
		}
		patchInput(&value.Input, input)
		patchConversation(&value.Conversation, conversation)
		patchRate(&value.TtsSpeechRatePercent, rate)
		if safetyFenceLevel != nil {
			value.SafetyFenceLevel = new(*safetyFenceLevel)
		}
		return updated, updated.FromFlowcraftWorkspaceParameters(value)
	case apitypes.WorkflowDriverDoubaoRealtime:
		value := apitypes.DoubaoRealtimeWorkspaceParameters{AgentType: apitypes.DoubaoRealtimeWorkspaceParametersAgentTypeDoubaoRealtime}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsDoubaoRealtimeWorkspaceParameters); err != nil {
			return nil, err
		}
		patchInput(&value.Input, input)
		patchConversation(&value.Conversation, conversation)
		patchRate(&value.TtsSpeechRatePercent, rate)
		if safetyFenceLevel != nil {
			value.SafetyFenceLevel = new(*safetyFenceLevel)
		}
		return updated, updated.FromDoubaoRealtimeWorkspaceParameters(value)
	case apitypes.WorkflowDriverAstTranslate:
		if input == nil && rate == nil && safetyFenceLevel == nil {
			return parameters, nil
		}
		value := apitypes.ASTTranslateWorkspaceParameters{AgentType: apitypes.ASTTranslateWorkspaceParametersAgentTypeAstTranslate}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsASTTranslateWorkspaceParameters); err != nil {
			return nil, err
		}
		patchInput(&value.Input, input)
		patchRate(&value.TtsSpeechRatePercent, rate)
		if safetyFenceLevel != nil {
			value.SafetyFenceLevel = new(*safetyFenceLevel)
		}
		return updated, updated.FromASTTranslateWorkspaceParameters(value)
	case apitypes.WorkflowDriverDashscopeRealtime:
		if rate == nil && safetyFenceLevel == nil {
			return parameters, nil
		}
		value := apitypes.DashScopeRealtimeWorkspaceParameters{AgentType: apitypes.DashScopeRealtimeWorkspaceParametersAgentTypeDashscopeRealtime}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsDashScopeRealtimeWorkspaceParameters); err != nil {
			return nil, err
		}
		patchRate(&value.TtsSpeechRatePercent, rate)
		if safetyFenceLevel != nil {
			value.SafetyFenceLevel = new(*safetyFenceLevel)
		}
		return updated, updated.FromDashScopeRealtimeWorkspaceParameters(value)
	case apitypes.WorkflowDriverDoubaoRealtimeDuplex:
		if rate == nil && safetyFenceLevel == nil {
			return parameters, nil
		}
		value := apitypes.DoubaoRealtimeDuplexWorkspaceParameters{AgentType: apitypes.DoubaoRealtimeDuplexWorkspaceParametersAgentTypeDoubaoRealtimeDuplex}
		if err := decodeWorkspaceParametersVariant(parameters, &value, apitypes.WorkspaceParameters.AsDoubaoRealtimeDuplexWorkspaceParameters); err != nil {
			return nil, err
		}
		patchRate(&value.TtsSpeechRatePercent, rate)
		if safetyFenceLevel != nil {
			value.SafetyFenceLevel = new(*safetyFenceLevel)
		}
		return updated, updated.FromDoubaoRealtimeDuplexWorkspaceParameters(value)
	default:
		return parameters, nil
	}
}

func patchInput(target **apitypes.WorkspaceInputMode, input *apitypes.WorkspaceInputMode) {
	if input != nil {
		value := *input
		*target = &value
	}
}

func patchConversation(target **apitypes.ConversationParameters, conversation *apitypes.ConversationParameters) {
	if conversation != nil {
		*target = mergeConversationParameters(*target, conversation)
	}
}

func patchRate(target **int, rate *int) {
	if rate != nil {
		*target = rate
	}
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
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
