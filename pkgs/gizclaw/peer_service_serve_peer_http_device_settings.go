package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
)

const deviceToolNotFoundCode = "TOOL_NOT_FOUND"

func invalidDeviceRequest(message string) *deviceControlError {
	return &deviceControlError{Status: http.StatusBadRequest, Code: publicHTTPInvalidRequestCode, Message: message}
}

func internalDeviceControlError() *deviceControlError {
	return &deviceControlError{Status: http.StatusInternalServerError, Code: publicHTTPInternalErrorCode, Message: http.StatusText(http.StatusInternalServerError)}
}

// deviceSettingsFromRPC projects the device's answer onto the Public HTTP
// schema. The device owns these values, so they pass through unchanged.
func deviceSettingsFromRPC(settings rpcapi.DeviceSettings) (peerhttp.DeviceSettings, *deviceControlError) {
	out, err := convertRPCType[peerhttp.DeviceSettings](settings)
	if err != nil {
		return peerhttp.DeviceSettings{}, internalDeviceControlError()
	}
	return out, nil
}

func (s *peerHTTP) GetDeviceSettings(ctx context.Context, _ peerhttp.GetDeviceSettingsRequestObject) (peerhttp.GetDeviceSettingsResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.GetDeviceSettings401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	result, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientDeviceSettingsGetResponse, error) {
		return client.GetDeviceSettings(ctx, conn, "client.device.settings.get")
	}, nil)
	if controlErr != nil {
		return getDeviceSettingsError(controlErr), nil
	}
	settings, controlErr := deviceSettingsFromRPC(result.Value)
	if controlErr != nil {
		return getDeviceSettingsError(controlErr), nil
	}
	return peerhttp.GetDeviceSettings200JSONResponse(settings), nil
}

func getDeviceSettingsError(e *deviceControlError) peerhttp.GetDeviceSettingsResponseObject {
	body := e.response()
	switch e.Status {
	case http.StatusBadRequest:
		return peerhttp.GetDeviceSettings400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case http.StatusConflict:
		return peerhttp.GetDeviceSettings409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case http.StatusNotImplemented:
		return peerhttp.GetDeviceSettings501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case http.StatusGatewayTimeout:
		return peerhttp.GetDeviceSettings504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case http.StatusInternalServerError:
		return peerhttp.GetDeviceSettings500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	default:
		return peerhttp.GetDeviceSettings502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}

// UpdateDeviceSettings forwards only the members present in the body. The
// whole patch is checked before it is sent, so a device is never asked to
// apply part of a request the Server already knows is out of range.
func (s *peerHTTP) UpdateDeviceSettings(ctx context.Context, request peerhttp.UpdateDeviceSettingsRequestObject) (peerhttp.UpdateDeviceSettingsResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.UpdateDeviceSettings401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if request.Body == nil {
		return updateDeviceSettingsError(invalidDeviceRequest("request body is required")), nil
	}
	patch, err := convertRPCType[rpcapi.DeviceSettings](*request.Body)
	if err != nil || !patch.Valid() {
		return updateDeviceSettingsError(invalidDeviceRequest("settings contain a value outside its range")), nil
	}
	result, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientDeviceSettingsSetResponse, error) {
		return client.SetDeviceSettings(ctx, conn, "client.device.settings.set", rpcapi.ClientDeviceSettingsSetRequest{Value: patch})
	}, nil)
	if controlErr != nil {
		return updateDeviceSettingsError(controlErr), nil
	}
	settings, controlErr := deviceSettingsFromRPC(result.Value)
	if controlErr != nil {
		return updateDeviceSettingsError(controlErr), nil
	}
	return peerhttp.UpdateDeviceSettings200JSONResponse(settings), nil
}

func updateDeviceSettingsError(e *deviceControlError) peerhttp.UpdateDeviceSettingsResponseObject {
	body := e.response()
	switch e.Status {
	case http.StatusBadRequest:
		return peerhttp.UpdateDeviceSettings400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case http.StatusConflict:
		return peerhttp.UpdateDeviceSettings409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case http.StatusNotImplemented:
		return peerhttp.UpdateDeviceSettings501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case http.StatusGatewayTimeout:
		return peerhttp.UpdateDeviceSettings504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case http.StatusInternalServerError:
		return peerhttp.UpdateDeviceSettings500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	default:
		return peerhttp.UpdateDeviceSettings502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}

// FactoryResetDevice marks the answering connection as transitioning, like a
// reboot: the device acknowledges before it erases its state, and later
// commands answer DEVICE_OFFLINE until it reconnects.
func (s *peerHTTP) FactoryResetDevice(ctx context.Context, request peerhttp.FactoryResetDeviceRequestObject) (peerhttp.FactoryResetDeviceResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.FactoryResetDevice401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	params := rpcapi.ClientDeviceFactoryResetRequest{}
	if request.Body != nil {
		params.KeepNetwork = request.Body.KeepNetwork
	}
	_, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{markTransition: true}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientDeviceFactoryResetResponse, error) {
		return client.FactoryResetDevice(ctx, conn, "client.device.factory_reset", params)
	}, nil)
	if controlErr != nil {
		return factoryResetDeviceError(controlErr), nil
	}
	return peerhttp.FactoryResetDevice204Response{}, nil
}

func factoryResetDeviceError(e *deviceControlError) peerhttp.FactoryResetDeviceResponseObject {
	body := e.response()
	switch e.Status {
	case http.StatusBadRequest:
		return peerhttp.FactoryResetDevice400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case http.StatusConflict:
		return peerhttp.FactoryResetDevice409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case http.StatusNotImplemented:
		return peerhttp.FactoryResetDevice501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case http.StatusGatewayTimeout:
		return peerhttp.FactoryResetDevice504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case http.StatusInternalServerError:
		return peerhttp.FactoryResetDevice500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	default:
		return peerhttp.FactoryResetDevice502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}

func (s *peerHTTP) ListDeviceRPCMethods(ctx context.Context, _ peerhttp.ListDeviceRPCMethodsRequestObject) (peerhttp.ListDeviceRPCMethodsResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ListDeviceRPCMethods401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	result, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientRPCMethodsGetResponse, error) {
		return client.GetRPCMethods(ctx, conn, "client.rpc.methods.get")
	}, nil)
	if controlErr != nil {
		return listDeviceRPCMethodsError(controlErr), nil
	}
	methods := result.Methods
	if methods == nil {
		methods = []string{}
	}
	return peerhttp.ListDeviceRPCMethods200JSONResponse{Methods: methods}, nil
}

func listDeviceRPCMethodsError(e *deviceControlError) peerhttp.ListDeviceRPCMethodsResponseObject {
	body := e.response()
	switch e.Status {
	case http.StatusBadRequest:
		return peerhttp.ListDeviceRPCMethods400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case http.StatusConflict:
		return peerhttp.ListDeviceRPCMethods409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case http.StatusNotImplemented:
		return peerhttp.ListDeviceRPCMethods501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case http.StatusGatewayTimeout:
		return peerhttp.ListDeviceRPCMethods504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case http.StatusInternalServerError:
		return peerhttp.ListDeviceRPCMethods500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	default:
		return peerhttp.ListDeviceRPCMethods502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}

func (s *peerHTTP) SetDeviceRunWorkspace(ctx context.Context, request peerhttp.SetDeviceRunWorkspaceRequestObject) (peerhttp.SetDeviceRunWorkspaceResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.SetDeviceRunWorkspace401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if request.Body == nil {
		return setDeviceRunWorkspaceError(invalidDeviceRequest("request body is required")), nil
	}
	body := request.Body
	valid := func(value *string) bool { return value != nil && *value != "" && len(*value) <= 256 }
	byName := body.WorkspaceName != nil
	if (byName && (!valid(body.WorkspaceName) || body.Collection != nil || body.WorkflowName != nil)) ||
		(!byName && (!valid(body.Collection) || !valid(body.WorkflowName))) {
		return setDeviceRunWorkspaceError(invalidDeviceRequest("set exactly one of workspace_name, or collection with workflow_name")), nil
	}
	reads, ok := s.deviceReads(owner)
	if !ok {
		return setDeviceRunWorkspaceError(internalDeviceControlError()), nil
	}
	var name, collection, workflowName string
	if byName {
		name = *body.WorkspaceName
	} else {
		collection, workflowName = *body.Collection, *body.WorkflowName
	}
	// The device reloads by name only, so a workflow target is resolved here
	// to exactly one Workspace before the device is contacted.
	resolved, err := reads.ResolveRunWorkspace(ctx, name, collection, workflowName)
	if errors.Is(err, peerresource.ErrDeviceWorkspaceNotFound) {
		return setDeviceRunWorkspaceError(&deviceControlError{Status: http.StatusNotFound, Code: "WORKSPACE_NOT_FOUND", Message: "no available Workspace matches the target"}), nil
	}
	if err != nil {
		return setDeviceRunWorkspaceError(internalDeviceControlError()), nil
	}
	params := rpcapi.ClientRunWorkspaceSetRequest{WorkspaceName: resolved, Kickoff: body.Kickoff}
	_, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ClientRunWorkspaceSetResponse, error) {
		return client.SetRunWorkspace(ctx, conn, "client.run.workspace.set", params)
	}, nil)
	if controlErr != nil {
		return setDeviceRunWorkspaceError(controlErr), nil
	}
	return peerhttp.SetDeviceRunWorkspace202Response{}, nil
}

func setDeviceRunWorkspaceError(e *deviceControlError) peerhttp.SetDeviceRunWorkspaceResponseObject {
	body := e.response()
	switch e.Status {
	case http.StatusNotFound:
		return peerhttp.SetDeviceRunWorkspace404JSONResponse{WorkspaceNotFoundJSONResponse: peerhttp.WorkspaceNotFoundJSONResponse(body)}
	case http.StatusBadRequest:
		return peerhttp.SetDeviceRunWorkspace400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case http.StatusConflict:
		return peerhttp.SetDeviceRunWorkspace409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case http.StatusNotImplemented:
		return peerhttp.SetDeviceRunWorkspace501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case http.StatusGatewayTimeout:
		return peerhttp.SetDeviceRunWorkspace504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case http.StatusInternalServerError:
		return peerhttp.SetDeviceRunWorkspace500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	default:
		return peerhttp.SetDeviceRunWorkspace502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}

func (s *peerHTTP) ListDeviceTools(ctx context.Context, _ peerhttp.ListDeviceToolsRequestObject) (peerhttp.ListDeviceToolsResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ListDeviceTools401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	reads, ok := s.deviceReads(owner)
	if !ok {
		return peerhttp.ListDeviceTools500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	tools, err := reads.DeviceControlTools(ctx)
	if err != nil {
		return peerhttp.ListDeviceTools500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	items := make([]peerhttp.DeviceTool, 0, len(tools))
	for _, tool := range tools {
		item, err := deviceToolItem(tool)
		if err != nil {
			return peerhttp.ListDeviceTools500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
		items = append(items, item)
	}
	return peerhttp.ListDeviceTools200JSONResponse{Items: items}, nil
}

func deviceToolItem(tool peerresource.ControlTool) (peerhttp.DeviceTool, error) {
	schema, err := convertRPCType[map[string]any](tool.Tool.InputSchema)
	if err != nil {
		return peerhttp.DeviceTool{}, err
	}
	if schema == nil {
		schema = map[string]any{}
	}
	i18n := make(map[string]peerhttp.DeviceToolI18nText, len(tool.Binding.I18n))
	for locale, text := range tool.Binding.I18n {
		i18n[locale] = peerhttp.DeviceToolI18nText{DisplayName: text.DisplayName, Description: text.Description}
	}
	return peerhttp.DeviceTool{
		Name: tool.Name, ControlAccess: peerhttp.DeviceToolControlAccess(*tool.Binding.ControlAccess),
		I18n: i18n, InputSchema: schema,
	}, nil
}

// InvokeDeviceTool runs one control-exposed Tool on the device. Arguments are
// checked against input_schema on the Server, so a malformed call never
// reaches the device, and a Tool the control app cannot list is not found.
func (s *peerHTTP) InvokeDeviceTool(ctx context.Context, request peerhttp.InvokeDeviceToolRequestObject) (peerhttp.InvokeDeviceToolResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.InvokeDeviceTool401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	reads, ok := s.deviceReads(owner)
	if !ok {
		return invokeDeviceToolError(internalDeviceControlError()), nil
	}
	tool, err := reads.DeviceControlTool(ctx, request.Name)
	if errors.Is(err, peerresource.ErrDeviceToolNotExposed) {
		return invokeDeviceToolError(&deviceControlError{Status: http.StatusNotFound, Code: deviceToolNotFoundCode, Message: "tool not found"}), nil
	}
	if err != nil {
		return invokeDeviceToolError(internalDeviceControlError()), nil
	}
	args := map[string]any{}
	if request.Body != nil && request.Body.Args != nil {
		args = *request.Body.Args
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return invokeDeviceToolError(invalidDeviceRequest("args must be a JSON object")), nil
	}
	if err := toolkit.ValidateToolArgs(tool.Tool, raw); err != nil {
		return invokeDeviceToolError(invalidDeviceRequest("args do not match the tool input_schema")), nil
	}
	params := rpcapi.ToolInvokeRequest{InvokeName: tool.Tool.InvokeName, Args: args}
	result, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{notFoundCode: deviceToolNotFoundCode}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcapi.ToolInvokeResponse, error) {
		return client.InvokeTool(ctx, conn, "client.tool.invoke", params)
	}, nil)
	if controlErr != nil {
		return invokeDeviceToolError(controlErr), nil
	}
	// data_json reaches the control app as JSON text; a device answer that is
	// not JSON is the device's fault and is not passed through.
	if !json.Valid([]byte(result.DataJson)) {
		return invokeDeviceToolError(&deviceControlError{Status: http.StatusBadGateway, Code: deviceErrorCode, Message: "device returned an error"}), nil
	}
	return peerhttp.InvokeDeviceTool200JSONResponse{DataJson: result.DataJson}, nil
}

func invokeDeviceToolError(e *deviceControlError) peerhttp.InvokeDeviceToolResponseObject {
	body := e.response()
	switch e.Status {
	case http.StatusBadRequest:
		return peerhttp.InvokeDeviceTool400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case http.StatusNotFound:
		return peerhttp.InvokeDeviceTool404JSONResponse{ToolNotFoundJSONResponse: peerhttp.ToolNotFoundJSONResponse(body)}
	case http.StatusConflict:
		return peerhttp.InvokeDeviceTool409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case http.StatusNotImplemented:
		return peerhttp.InvokeDeviceTool501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case http.StatusGatewayTimeout:
		return peerhttp.InvokeDeviceTool504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case http.StatusInternalServerError:
		return peerhttp.InvokeDeviceTool500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	default:
		return peerhttp.InvokeDeviceTool502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}
