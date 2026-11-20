package observability

// Transport identifies the request protocol boundary.
type Transport string

const (
	TransportHTTP Transport = "http"
	TransportRPC  Transport = "rpc"
)

// Surface identifies a bounded GizClaw ingress surface.
type Surface string

const (
	SurfaceServerPublic Surface = "server-public"
	SurfacePeerHTTP     Surface = "peer-http"
	SurfaceAdminHTTP    Surface = "admin-http"
	SurfacePeerOpenAI   Surface = "peer-openai"
	SurfaceEdgeHTTP     Surface = "edge-http"
	SurfacePeerRPC      Surface = "peer-rpc"
)

// Result is the bounded completion classification.
type Result string

const (
	ResultSuccess        Result = "success"
	ResultClientError    Result = "client_error"
	ResultServerError    Result = "server_error"
	ResultCanceled       Result = "canceled"
	ResultPanic          Result = "panic"
	ResultTransportError Result = "transport_error"
)

// StatusClass is the bounded HTTP status family.
type StatusClass string

const (
	StatusClass2xx     StatusClass = "2xx"
	StatusClass3xx     StatusClass = "3xx"
	StatusClass4xx     StatusClass = "4xx"
	StatusClass5xx     StatusClass = "5xx"
	StatusClassUnknown StatusClass = "unknown"
)

// AnnotationKey is a server-owned safe domain attribute.
type AnnotationKey string

const (
	AnnotationWorkspaceName        AnnotationKey = "workspace_name"
	AnnotationWorkflowName         AnnotationKey = "workflow_name"
	AnnotationModelID              AnnotationKey = "model_id"
	AnnotationResourceKind         AnnotationKey = "resource_kind"
	AnnotationResourceName         AnnotationKey = "resource_name"
	AnnotationDeviceControlFailure AnnotationKey = "control_failure_reason"
)

// DeviceControlFailureReason is a fixed diagnostic label for a control failure.
type DeviceControlFailureReason string

const (
	ControlFailureWiFiTransition    DeviceControlFailureReason = "wifi_transition"
	ControlFailureRestartTransition DeviceControlFailureReason = "restart_transition"
	ControlFailurePeerMissing       DeviceControlFailureReason = "peer_missing"
	ControlFailureRPCDial           DeviceControlFailureReason = "rpc_dial_failed"
	ControlFailureRPCDisconnected   DeviceControlFailureReason = "rpc_disconnected"
	ControlFailureRPCUnavailable    DeviceControlFailureReason = "rpc_unavailable"
	ControlFailureRPCTimeout        DeviceControlFailureReason = "rpc_timeout"
)

func validDeviceControlFailureReason(value string) bool {
	switch DeviceControlFailureReason(value) {
	case ControlFailureWiFiTransition, ControlFailureRestartTransition, ControlFailurePeerMissing,
		ControlFailureRPCDial, ControlFailureRPCDisconnected, ControlFailureRPCUnavailable, ControlFailureRPCTimeout:
		return true
	default:
		return false
	}
}
