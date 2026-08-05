//go:build gizclaw_e2e

package edge_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

type relayTraffic struct {
	rx uint64
	tx uint64
}

func TestGatewayRelayRecoversSameClientBeforeSessionAcceptance(t *testing.T) {
	if requiredEnv(t, "GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY") == "" {
		t.Fatal("gateway relay recovery requires its Compose overlay")
	}
	endpoint := loadGatewayEndpoint(t, requiredEnv(t, "GIZCLAW_E2E_EDGE_ENDPOINT"))
	tokens := createGatewayRecoveryRegistrationTokens(t, 2)
	beforeBaseline := map[string]relayTraffic{
		"coturn-a": relayTrafficSnapshot(t, "coturn-a"),
		"coturn-b": relayTrafficSnapshot(t, "coturn-b"),
	}

	baseline := connect(t, endpoint)
	registerAndPingGatewayRecovery(t, baseline, tokens[0], "gateway-relay-baseline")
	baseline.Close()
	time.Sleep(500 * time.Millisecond)
	afterBaseline := map[string]relayTraffic{
		"coturn-a": relayTrafficSnapshot(t, "coturn-a"),
		"coturn-b": relayTrafficSnapshot(t, "coturn-b"),
	}
	selected, alternate := selectRelayFromTraffic(t, beforeBaseline, afterBaseline)
	selectedContainer := composeOutput(t, "ps", "-q", selected)
	if selectedContainer == "" {
		t.Fatal("selected Coturn container is not running")
	}
	assertDirectServerUDPBlocked(t)
	dropRelayUDP(t, selected)

	beforeRecovery := map[string]relayTraffic{
		"coturn-a": relayTrafficSnapshot(t, "coturn-a"),
		"coturn-b": relayTrafficSnapshot(t, "coturn-b"),
	}
	started := time.Now()
	recovered := connect(t, endpoint)
	defer recovered.Close()
	registerAndPingGatewayRecovery(t, recovered, tokens[1], "gateway-relay-recovered")
	if elapsed := time.Since(started); elapsed >= 30*time.Second {
		t.Fatalf("same-client recovery took %s, want less than the establishment budget", elapsed)
	}
	afterRecovery := map[string]relayTraffic{
		"coturn-a": relayTrafficSnapshot(t, "coturn-a"),
		"coturn-b": relayTrafficSnapshot(t, "coturn-b"),
	}
	if delta := trafficDelta(beforeRecovery[alternate], afterRecovery[alternate]); delta == 0 {
		t.Fatalf("alternate Coturn member %s carried no recovery traffic", alternate)
	}
	if got := composeOutput(t, "ps", "-q", selected); got != selectedContainer {
		t.Fatalf("selected Coturn process changed during silent fault: before=%s after=%s", selectedContainer, got)
	}

	edgeLog := composeOutput(t, "exec", "-T", "edge", "sh", "-c", "cat /src/tests/gizclaw-e2e/testdata/edge-workspace/gizclaw-edge.log")
	for _, marker := range []string{
		"state=draining reason=session_handshake_timeout",
		"gateway logical session retry",
		"gateway logical session alternate succeeded",
	} {
		if !strings.Contains(edgeLog, marker) {
			t.Fatalf("Edge log is missing sanitized recovery marker %q", marker)
		}
	}
	for name, sensitive := range map[string]string{
		"relay username":   requiredEnv(t, "GIZCLAW_E2E_GATEWAY_RELAY_USERNAME"),
		"relay credential": requiredEnv(t, "GIZCLAW_E2E_GATEWAY_RELAY_CREDENTIAL"),
		"Server address":   requiredEnv(t, "GIZCLAW_E2E_GATEWAY_RELAY_SERVER_IP"),
		"Edge address":     requiredEnv(t, "GIZCLAW_E2E_GATEWAY_RELAY_EDGE_IP"),
		"Coturn A address": requiredEnv(t, "GIZCLAW_E2E_GATEWAY_RELAY_TURN_A_IP"),
		"Coturn B address": requiredEnv(t, "GIZCLAW_E2E_GATEWAY_RELAY_TURN_B_IP"),
	} {
		if strings.Contains(edgeLog, sensitive) {
			t.Fatalf("Edge log exposed %s", name)
		}
	}
	t.Logf("sanitized Coturn traffic selected=%s alternate=%s baseline_delta=%d recovery_delta=%d",
		selected,
		alternate,
		trafficDelta(beforeBaseline[selected], afterBaseline[selected]),
		trafficDelta(beforeRecovery[alternate], afterRecovery[alternate]),
	)
}

func createGatewayRecoveryRegistrationTokens(t *testing.T, count int) []string {
	t.Helper()
	harness := clitest.NewSetupHarness(t, "gateway-relay-recovery")
	harness.InstallFixedAdminContext("gateway-relay-recovery-admin").MustSucceed(t)
	admin := harness.ConnectClientFromContextEventually("gateway-relay-recovery-admin", 30*time.Second)
	t.Cleanup(func() { _ = admin.Close() })
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create gateway recovery admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	profileID := createGatewayRecoveryRuntimeProfile(t, ctx, api)
	tokens := make([]string, 0, count)
	for index := range count {
		name := fmt.Sprintf("gateway-relay-recovery-%d-%d", time.Now().UnixNano(), index)
		response, err := api.CreateRegistrationTokenWithResponse(ctx, adminhttp.RegistrationTokenUpsert{
			Id:               name,
			Token:            name,
			RuntimeProfileId: profileID,
		})
		if err != nil {
			t.Fatalf("create gateway recovery RegistrationToken: %v", err)
		}
		if response.JSON200 == nil || response.JSON200.Token == "" {
			t.Fatalf("create gateway recovery RegistrationToken status=%d", response.StatusCode())
		}
		tokenID := response.JSON200.Id
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			_, _ = api.DeleteRegistrationTokenWithResponse(cleanupCtx, tokenID)
		})
		tokens = append(tokens, response.JSON200.Token)
	}
	return tokens
}

func createGatewayRecoveryRuntimeProfile(
	t *testing.T,
	ctx context.Context,
	api *adminhttp.ClientWithResponses,
) string {
	t.Helper()
	workflowIDs := make([]string, 0, 3)
	for index := range 3 {
		name := fmt.Sprintf("gw-relay-wf-%d-%d", time.Now().UnixNano(), index)
		response, err := api.CreateWorkflowWithResponse(ctx, gatewayRecoveryWorkflow(name, index))
		if err != nil {
			t.Fatalf("create gateway recovery Workflow: %v", err)
		}
		if response.JSON200 == nil {
			t.Fatalf("create gateway recovery Workflow status=%d response=%+v", response.StatusCode(), response.JSON400)
		}
		workflowID := response.JSON200.Id
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			_, _ = api.DeleteWorkflowWithResponse(cleanupCtx, workflowID)
		})
		workflowIDs = append(workflowIDs, workflowID)
	}
	name := fmt.Sprintf("gw-relay-profile-%d", time.Now().UnixNano())
	response, err := api.CreateRuntimeProfileWithResponse(ctx, adminhttp.RuntimeProfileUpsert{
		Id: name,
		Spec: apitypes.RuntimeProfileSpec{
			Resources: apitypes.RuntimeProfileResources{},
			Workflows: apitypes.RuntimeProfileWorkflows{
				Collections: apitypes.RuntimeProfileWorkflowCollections{},
				System: apitypes.RuntimeProfileSystemWorkflows{
					FriendChatroom: workflowIDs[0],
					GroupChatroom:  workflowIDs[1],
					Pet:            workflowIDs[2],
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create gateway recovery RuntimeProfile: %v", err)
	}
	if response.JSON200 == nil {
		t.Fatalf("create gateway recovery RuntimeProfile status=%d response=%+v", response.StatusCode(), response.JSON400)
	}
	profileID := response.JSON200.Id
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = api.DeleteRuntimeProfileWithResponse(cleanupCtx, profileID)
	})
	return profileID
}

func gatewayRecoveryWorkflow(name string, index int) adminhttp.WorkflowUpsert {
	spec := apitypes.WorkflowSpec{
		Driver:   apitypes.WorkflowDriverChatroom,
		Chatroom: &apitypes.ChatRoomWorkflowSpec{},
	}
	if index == 2 {
		spec = apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverPet,
			Pet: &apitypes.PetWorkflowSpec{
				Driver:   apitypes.ReusableWorkflowDriverChatroom,
				Chatroom: &apitypes.ChatRoomWorkflowSpec{},
			},
		}
	}
	return adminhttp.WorkflowUpsert{Id: name, Spec: spec}
}

func registerAndPingGatewayRecovery(t *testing.T, client *gizcli.Client, token, id string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	if _, err := client.Register(ctx, id+".register", token); err != nil {
		t.Fatalf("%s Register: %v", id, err)
	}
	if _, err := client.Ping(ctx, id+".ping"); err != nil {
		t.Fatalf("%s Ping: %v", id, err)
	}
}

func relayTrafficSnapshot(t *testing.T, service string) relayTraffic {
	t.Helper()
	output := composeOutput(t, "exec", "-T", service, "sh", "-c", "printf '%s %s' \"$(cat /sys/class/net/eth0/statistics/rx_bytes)\" \"$(cat /sys/class/net/eth0/statistics/tx_bytes)\"")
	var traffic relayTraffic
	if _, err := fmt.Sscanf(output, "%d %d", &traffic.rx, &traffic.tx); err != nil {
		t.Fatalf("parse sanitized %s traffic counters: %v", service, err)
	}
	return traffic
}

func trafficDelta(before, after relayTraffic) uint64 {
	var delta uint64
	if after.rx >= before.rx {
		delta += after.rx - before.rx
	}
	if after.tx >= before.tx {
		delta += after.tx - before.tx
	}
	return delta
}

func selectRelayFromTraffic(t *testing.T, before, after map[string]relayTraffic) (string, string) {
	t.Helper()
	a := trafficDelta(before["coturn-a"], after["coturn-a"])
	b := trafficDelta(before["coturn-b"], after["coturn-b"])
	switch {
	case a > b && a > 0:
		return "coturn-a", "coturn-b"
	case b > a && b > 0:
		return "coturn-b", "coturn-a"
	default:
		t.Fatalf("Coturn traffic did not identify one selected member: a=%d b=%d", a, b)
		return "", ""
	}
}

func assertDirectServerUDPBlocked(t *testing.T) {
	t.Helper()
	compose(t, "exec", "-T", "gateway-fault", "sh", "-ec", "iptables -C OUTPUT -p udp -d \"$GIZCLAW_E2E_GATEWAY_RELAY_SERVER_IP\" -j REJECT && iptables -C INPUT -p udp -s \"$GIZCLAW_E2E_GATEWAY_RELAY_SERVER_IP\" -j REJECT")
}

func dropRelayUDP(t *testing.T, selected string) {
	t.Helper()
	var variable string
	switch selected {
	case "coturn-a":
		variable = "GIZCLAW_E2E_GATEWAY_RELAY_TURN_A_IP"
	case "coturn-b":
		variable = "GIZCLAW_E2E_GATEWAY_RELAY_TURN_B_IP"
	default:
		t.Fatalf("unsupported selected relay %q", selected)
	}
	script := "target=\"$(printenv \"$1\")\"; test -n \"$target\"; iptables -I OUTPUT 1 -p udp -d \"$target\" -j DROP; iptables -I INPUT 1 -p udp -s \"$target\" -j DROP"
	compose(t, "exec", "-T", "gateway-fault", "sh", "-ec", script, "sh", variable)
}
