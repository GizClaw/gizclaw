package gizclaw

import (
	"context"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

func TestClientResourceMethodsRequireConnection(t *testing.T) {
	client := &Client{}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() (any, error)
	}{
		{"workspace list", func() (any, error) {
			return client.ListWorkspaces(ctx, "workspace-list", rpcapi.WorkspaceListRequest{})
		}},
		{"workspace get", func() (any, error) {
			return client.GetWorkspace(ctx, "workspace-get", rpcapi.WorkspaceGetRequest{Name: "workspace-a"})
		}},
		{"workspace create", func() (any, error) {
			return client.CreateWorkspace(ctx, "workspace-create", rpcapi.WorkspaceCreateRequest{Name: "workspace-a", WorkflowName: "flow-a"})
		}},
		{"workspace put", func() (any, error) {
			return client.PutWorkspace(ctx, "workspace-put", rpcapi.WorkspacePutRequest{Name: "workspace-a", Body: rpcapi.Workspace{Name: "workspace-a", WorkflowName: "flow-a"}})
		}},
		{"workspace delete", func() (any, error) {
			return client.DeleteWorkspace(ctx, "workspace-delete", rpcapi.WorkspaceDeleteRequest{Name: "workspace-a"})
		}},
		{"workflow list", func() (any, error) { return client.ListWorkflows(ctx, "workflow-list", rpcapi.WorkflowListRequest{}) }},
		{"workflow get", func() (any, error) {
			return client.GetWorkflow(ctx, "workflow-get", rpcapi.WorkflowGetRequest{Name: "flow-a"})
		}},
		{"workflow create", func() (any, error) {
			return client.CreateWorkflow(ctx, "workflow-create", resourceWorkflowDoc("flow-a"))
		}},
		{"workflow put", func() (any, error) {
			return client.PutWorkflow(ctx, "workflow-put", rpcapi.WorkflowPutRequest{Name: "flow-a", Body: resourceWorkflowDoc("flow-a")})
		}},
		{"workflow delete", func() (any, error) {
			return client.DeleteWorkflow(ctx, "workflow-delete", rpcapi.WorkflowDeleteRequest{Name: "flow-a"})
		}},
		{"model list", func() (any, error) { return client.ListModels(ctx, "model-list", rpcapi.ModelListRequest{}) }},
		{"model get", func() (any, error) { return client.GetModel(ctx, "model-get", rpcapi.ModelGetRequest{Id: "model-a"}) }},
		{"model create", func() (any, error) { return client.CreateModel(ctx, "model-create", resourceModel("model-a")) }},
		{"model put", func() (any, error) {
			return client.PutModel(ctx, "model-put", rpcapi.ModelPutRequest{Id: "model-a", Body: resourceModel("model-a")})
		}},
		{"model delete", func() (any, error) {
			return client.DeleteModel(ctx, "model-delete", rpcapi.ModelDeleteRequest{Id: "model-a"})
		}},
		{"credential list", func() (any, error) {
			return client.ListCredentials(ctx, "credential-list", rpcapi.CredentialListRequest{})
		}},
		{"credential get", func() (any, error) {
			return client.GetCredential(ctx, "credential-get", rpcapi.CredentialGetRequest{Name: "credential-a"})
		}},
		{"credential create", func() (any, error) {
			return client.CreateCredential(ctx, "credential-create", resourceCredential("credential-a"))
		}},
		{"credential put", func() (any, error) {
			return client.PutCredential(ctx, "credential-put", rpcapi.CredentialPutRequest{Name: "credential-a", Body: resourceCredential("credential-a")})
		}},
		{"credential delete", func() (any, error) {
			return client.DeleteCredential(ctx, "credential-delete", rpcapi.CredentialDeleteRequest{Name: "credential-a"})
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.call(); err == nil || !strings.Contains(err.Error(), "client is not connected") {
				t.Fatalf("resource client call error = %v", err)
			}
		})
	}
}
