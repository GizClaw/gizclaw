package gizclaw

import (
	"context"
	"net"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

func (c *Client) ListWorkspaces(ctx context.Context, id string, request rpcapi.WorkspaceListRequest) (*rpcapi.WorkspaceListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkspaceListResponse, error) {
		return client.ListWorkspaces(ctx, conn, id, request)
	})
}

func (c *Client) GetWorkspace(ctx context.Context, id string, request rpcapi.WorkspaceGetRequest) (*rpcapi.WorkspaceGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkspaceGetResponse, error) {
		return client.GetWorkspace(ctx, conn, id, request)
	})
}

func (c *Client) CreateWorkspace(ctx context.Context, id string, request rpcapi.WorkspaceCreateRequest) (*rpcapi.WorkspaceCreateResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkspaceCreateResponse, error) {
		return client.CreateWorkspace(ctx, conn, id, request)
	})
}

func (c *Client) PutWorkspace(ctx context.Context, id string, request rpcapi.WorkspacePutRequest) (*rpcapi.WorkspacePutResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkspacePutResponse, error) {
		return client.PutWorkspace(ctx, conn, id, request)
	})
}

func (c *Client) DeleteWorkspace(ctx context.Context, id string, request rpcapi.WorkspaceDeleteRequest) (*rpcapi.WorkspaceDeleteResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkspaceDeleteResponse, error) {
		return client.DeleteWorkspace(ctx, conn, id, request)
	})
}

func (c *Client) ListWorkflows(ctx context.Context, id string, request rpcapi.WorkflowListRequest) (*rpcapi.WorkflowListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkflowListResponse, error) {
		return client.ListWorkflows(ctx, conn, id, request)
	})
}

func (c *Client) GetWorkflow(ctx context.Context, id string, request rpcapi.WorkflowGetRequest) (*rpcapi.WorkflowGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkflowGetResponse, error) {
		return client.GetWorkflow(ctx, conn, id, request)
	})
}

func (c *Client) CreateWorkflow(ctx context.Context, id string, request rpcapi.WorkflowCreateRequest) (*rpcapi.WorkflowCreateResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkflowCreateResponse, error) {
		return client.CreateWorkflow(ctx, conn, id, request)
	})
}

func (c *Client) PutWorkflow(ctx context.Context, id string, request rpcapi.WorkflowPutRequest) (*rpcapi.WorkflowPutResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkflowPutResponse, error) {
		return client.PutWorkflow(ctx, conn, id, request)
	})
}

func (c *Client) DeleteWorkflow(ctx context.Context, id string, request rpcapi.WorkflowDeleteRequest) (*rpcapi.WorkflowDeleteResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.WorkflowDeleteResponse, error) {
		return client.DeleteWorkflow(ctx, conn, id, request)
	})
}

func (c *Client) ListModels(ctx context.Context, id string, request rpcapi.ModelListRequest) (*rpcapi.ModelListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ModelListResponse, error) {
		return client.ListModels(ctx, conn, id, request)
	})
}

func (c *Client) GetModel(ctx context.Context, id string, request rpcapi.ModelGetRequest) (*rpcapi.ModelGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ModelGetResponse, error) {
		return client.GetModel(ctx, conn, id, request)
	})
}

func (c *Client) CreateModel(ctx context.Context, id string, request rpcapi.ModelCreateRequest) (*rpcapi.ModelCreateResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ModelCreateResponse, error) {
		return client.CreateModel(ctx, conn, id, request)
	})
}

func (c *Client) PutModel(ctx context.Context, id string, request rpcapi.ModelPutRequest) (*rpcapi.ModelPutResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ModelPutResponse, error) {
		return client.PutModel(ctx, conn, id, request)
	})
}

func (c *Client) DeleteModel(ctx context.Context, id string, request rpcapi.ModelDeleteRequest) (*rpcapi.ModelDeleteResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ModelDeleteResponse, error) {
		return client.DeleteModel(ctx, conn, id, request)
	})
}

func (c *Client) ListCredentials(ctx context.Context, id string, request rpcapi.CredentialListRequest) (*rpcapi.CredentialListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.CredentialListResponse, error) {
		return client.ListCredentials(ctx, conn, id, request)
	})
}

func (c *Client) GetCredential(ctx context.Context, id string, request rpcapi.CredentialGetRequest) (*rpcapi.CredentialGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.CredentialGetResponse, error) {
		return client.GetCredential(ctx, conn, id, request)
	})
}

func (c *Client) CreateCredential(ctx context.Context, id string, request rpcapi.CredentialCreateRequest) (*rpcapi.CredentialCreateResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.CredentialCreateResponse, error) {
		return client.CreateCredential(ctx, conn, id, request)
	})
}

func (c *Client) PutCredential(ctx context.Context, id string, request rpcapi.CredentialPutRequest) (*rpcapi.CredentialPutResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.CredentialPutResponse, error) {
		return client.PutCredential(ctx, conn, id, request)
	})
}

func (c *Client) DeleteCredential(ctx context.Context, id string, request rpcapi.CredentialDeleteRequest) (*rpcapi.CredentialDeleteResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.CredentialDeleteResponse, error) {
		return client.DeleteCredential(ctx, conn, id, request)
	})
}
