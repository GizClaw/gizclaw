package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/credential"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/providertenants"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/voice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

var (
	ErrDeviceOffline = errors.New("gizclaw: device offline")
	errNoRefreshData = errors.New("gizclaw: no refresh data")
)

type activePeer struct {
	conn giznet.Conn
}

type Manager struct {
	Peers     *peer.Server
	PeerRun   *peerrun.Server
	AgentHost *agenthost.Host
	ACL       *acl.Server

	Workspaces   workspace.WorkspaceAdminService
	Workflows    workflow.WorkflowAdminService
	Firmwares    *firmware.Server
	Models       model.ModelAdminService
	Credentials  credential.CredentialAdminService
	Voices       voice.VoiceAdminService
	Contacts     *contact.Server
	Friends      *friend.Server
	FriendGroups *friendgroup.Server
	Gameplay     *gameplay.Runtime

	ProviderTenants providertenants.ProviderTenantsAdminService
	Metrics         metrics.Store

	mu    sync.RWMutex
	peers map[giznet.PublicKey]*activePeer

	telemetryStatusMu    sync.Mutex
	telemetryStatusLocks map[giznet.PublicKey]*sync.Mutex
}

func NewManager(peersService *peer.Server) *Manager {
	return &Manager{
		Peers:                peersService,
		peers:                make(map[giznet.PublicKey]*activePeer),
		telemetryStatusLocks: make(map[giznet.PublicKey]*sync.Mutex),
	}
}

func (m *Manager) telemetryStatusLock(publicKey giznet.PublicKey) *sync.Mutex {
	if m == nil {
		return nil
	}
	m.telemetryStatusMu.Lock()
	defer m.telemetryStatusMu.Unlock()
	if m.telemetryStatusLocks == nil {
		m.telemetryStatusLocks = make(map[giznet.PublicKey]*sync.Mutex)
	}
	lock := m.telemetryStatusLocks[publicKey]
	if lock == nil {
		lock = &sync.Mutex{}
		m.telemetryStatusLocks[publicKey] = lock
	}
	return lock
}

func (m *Manager) allowService(ctx context.Context, publicKey giznet.PublicKey, service uint64) bool {
	switch service {
	case ServiceRPC, ServiceServerPublic, ServiceOpenAI, ServiceAgentStream:
		return true
	}
	switch service {
	case ServiceAdmin:
		peer, err := m.Peers.LoadPeer(ctx, publicKey)
		if err != nil {
			return false
		}
		return peer.Status == apitypes.PeerRegistrationStatusActive && peer.Role == apitypes.PeerRoleAdmin
	default:
		return false
	}
}

func (m *Manager) SetPeerUp(publicKey giznet.PublicKey, conn giznet.Conn) giznet.Conn {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.peers == nil {
		m.peers = make(map[giznet.PublicKey]*activePeer)
	}
	state, ok := m.peers[publicKey]
	if !ok {
		state = &activePeer{}
		m.peers[publicKey] = state
	}
	oldConn := state.conn
	state.conn = conn
	if oldConn == conn {
		return nil
	}
	return oldConn
}

func (m *Manager) SetPeerDown(publicKey giznet.PublicKey, conn giznet.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.peers[publicKey]
	if !ok || state.conn != conn {
		return
	}
	delete(m.peers, publicKey)
}

func (m *Manager) ForcePeerDown(publicKey giznet.PublicKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.peers, publicKey)
}

func (m *Manager) Peer(publicKey giznet.PublicKey) (giznet.Conn, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.peers[publicKey]
	if !ok || state.conn == nil {
		return nil, false
	}
	return state.conn, true
}

func (m *Manager) PeerRuntime(_ context.Context, publicKey giznet.PublicKey) apitypes.Runtime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.peers[publicKey]
	if !ok {
		return apitypes.Runtime{}
	}
	runtime := apitypes.Runtime{
		Online: true,
	}
	if state.conn != nil {
		if info := state.conn.PeerInfo(); info != nil {
			runtime.LastSeenAt = info.LastSeen
			runtime.RxBytes = &info.RxBytes
			runtime.TxBytes = &info.TxBytes
			if info.Endpoint != nil {
				lastAddr := info.Endpoint.String()
				runtime.LastAddr = &lastAddr
			}
		}
	}
	return runtime
}

func (m *Manager) EnsurePeer(ctx context.Context, publicKey giznet.PublicKey) (apitypes.Peer, error) {
	if m == nil || m.Peers == nil {
		return apitypes.Peer{}, errors.New("gizclaw: peers service not configured")
	}
	return m.Peers.EnsureConnectedPeer(ctx, publicKey)
}

func (m *Manager) RefreshPeer(ctx context.Context, publicKey giznet.PublicKey) (adminservice.RefreshResult, bool, error) {
	if m.Peers == nil {
		return adminservice.RefreshResult{}, false, errors.New("gizclaw: peers service not configured")
	}
	peer, err := m.Peers.LoadPeer(ctx, publicKey)
	if err != nil {
		return adminservice.RefreshResult{}, false, err
	}
	next, updatedFields, errs, err := m.refreshPeer(ctx, publicKey, peer)
	if err != nil {
		online := true
		if errors.Is(err, ErrDeviceOffline) {
			m.ForcePeerDown(publicKey)
			online = false
		}
		return adminservice.RefreshResult{
			Peer:   peer,
			Errors: optionalStrings(errs),
		}, online, err
	}
	if len(updatedFields) == 0 {
		return adminservice.RefreshResult{
			Peer:          next,
			Errors:        optionalStrings(errs),
			UpdatedFields: nil,
		}, true, nil
	}
	saved, err := m.Peers.SavePeer(ctx, next)
	if err != nil {
		return adminservice.RefreshResult{
			Peer:          next,
			Errors:        optionalStrings(errs),
			UpdatedFields: optionalStrings(updatedFields),
		}, true, err
	}
	return adminservice.RefreshResult{
		Peer:          saved,
		Errors:        optionalStrings(errs),
		UpdatedFields: optionalStrings(updatedFields),
	}, true, nil
}

func (m *Manager) peerRPCConn(publicKey giznet.PublicKey) (net.Conn, error) {
	m.mu.RLock()
	state, ok := m.peers[publicKey]
	var peerConn giznet.Conn
	if ok {
		peerConn = state.conn
	}
	m.mu.RUnlock()

	if !ok || peerConn == nil {
		return nil, ErrDeviceOffline
	}
	stream, err := peerConn.Dial(ServiceRPC)
	if err != nil {
		return nil, fmt.Errorf("dial peer rpc: %w", err)
	}
	return stream, nil
}

func callPeerRPC[T any](m *Manager, ctx context.Context, publicKey giznet.PublicKey, call func(*rpcClient, net.Conn) (*T, error)) (*T, error) {
	stream, err := m.peerRPCConn(publicKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stream.Close() }()
	return call(&rpcClient{}, stream)
}

func (m *Manager) refreshPeer(ctx context.Context, publicKey giznet.PublicKey, peer apitypes.Peer) (apitypes.Peer, []string, []string, error) {
	var (
		errs          []string
		updatedFields []string
		haveData      bool
		disconnected  bool
	)

	infoResp, err := callPeerRPC(m, ctx, publicKey, func(client *rpcClient, conn net.Conn) (*rpcapi.ClientGetInfoResponse, error) {
		return client.GetClientInfo(ctx, conn, "client.info.get")
	})
	if err != nil {
		if errors.Is(err, ErrDeviceOffline) || isPeerDisconnectedError(err) {
			disconnected = true
		}
		errs = append(errs, "info: "+err.Error())
	} else {
		info, err := convertRPCType[apitypes.RefreshInfo](*infoResp)
		if err != nil {
			errs = append(errs, "info: "+err.Error())
		} else {
			haveData = true
			applyPeerRefreshInfo(&peer, info, &updatedFields)
		}
	}

	identifiersResp, err := callPeerRPC(m, ctx, publicKey, func(client *rpcClient, conn net.Conn) (*rpcapi.ClientGetIdentifiersResponse, error) {
		return client.GetClientIdentifiers(ctx, conn, "client.identifiers.get")
	})
	if err != nil {
		if errors.Is(err, ErrDeviceOffline) || isPeerDisconnectedError(err) {
			disconnected = true
		}
		errs = append(errs, "identifiers: "+err.Error())
	} else {
		identifiers, err := convertRPCType[apitypes.RefreshIdentifiers](*identifiersResp)
		if err != nil {
			errs = append(errs, "identifiers: "+err.Error())
		} else {
			haveData = true
			applyPeerRefreshIdentifiers(&peer, identifiers, &updatedFields)
		}
	}

	if !haveData {
		if disconnected {
			return peer, updatedFields, errs, ErrDeviceOffline
		}
		return peer, updatedFields, errs, errNoRefreshData
	}
	return peer, updatedFields, errs, nil
}

func isPeerDisconnectedError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "conn closed") ||
		strings.Contains(msg, "closed network connection")
}

func applyPeerRefreshInfo(peer *apitypes.Peer, info apitypes.RefreshInfo, updatedFields *[]string) {
	if peer == nil {
		return
	}
	if info.Name != nil && *info.Name != "" && !equalStringPtr(peer.Device.Name, info.Name) {
		peer.Device.Name = info.Name
		*updatedFields = append(*updatedFields, "device.name")
	}
	if info.Manufacturer != nil && *info.Manufacturer != "" {
		hardware := ensurePeerHardware(&peer.Device)
		if !equalStringPtr(hardware.Manufacturer, info.Manufacturer) {
			hardware.Manufacturer = info.Manufacturer
			*updatedFields = append(*updatedFields, "device.hardware.manufacturer")
		}
	}
	if info.Model != nil && *info.Model != "" {
		hardware := ensurePeerHardware(&peer.Device)
		if !equalStringPtr(hardware.Model, info.Model) {
			hardware.Model = info.Model
			*updatedFields = append(*updatedFields, "device.hardware.model")
		}
	}
	if info.HardwareRevision != nil && *info.HardwareRevision != "" {
		hardware := ensurePeerHardware(&peer.Device)
		if !equalStringPtr(hardware.HardwareRevision, info.HardwareRevision) {
			hardware.HardwareRevision = info.HardwareRevision
			*updatedFields = append(*updatedFields, "device.hardware.hardware_revision")
		}
	}
}

func applyPeerRefreshIdentifiers(peer *apitypes.Peer, identifiers apitypes.RefreshIdentifiers, updatedFields *[]string) {
	if peer == nil {
		return
	}
	if identifiers.Sn != nil && *identifiers.Sn != "" && !equalStringPtr(peer.Device.Sn, identifiers.Sn) {
		peer.Device.Sn = identifiers.Sn
		*updatedFields = append(*updatedFields, "device.sn")
	}
	if identifiers.Imeis != nil && len(*identifiers.Imeis) > 0 {
		items := toPeerIMEIs(*identifiers.Imeis)
		hardware := ensurePeerHardware(&peer.Device)
		if !equalPeerIMEISlice(hardware.Imeis, items) {
			hardware.Imeis = &items
			*updatedFields = append(*updatedFields, "device.hardware.imeis")
		}
	}
	if identifiers.Labels != nil && len(*identifiers.Labels) > 0 {
		items := toPeerLabels(*identifiers.Labels)
		hardware := ensurePeerHardware(&peer.Device)
		if !equalPeerLabelSlice(hardware.Labels, items) {
			hardware.Labels = &items
			*updatedFields = append(*updatedFields, "device.hardware.labels")
		}
	}
}

func ensurePeerHardware(device *apitypes.DeviceInfo) *apitypes.HardwareInfo {
	if device.Hardware == nil {
		device.Hardware = &apitypes.HardwareInfo{}
	}
	return device.Hardware
}

func toPeerIMEIs(in []apitypes.PeerIMEI) []apitypes.PeerIMEI {
	out := make([]apitypes.PeerIMEI, 0, len(in))
	for _, item := range in {
		out = append(out, apitypes.PeerIMEI{
			Name:   item.Name,
			Tac:    item.Tac,
			Serial: item.Serial,
		})
	}
	return out
}

func toPeerLabels(in []apitypes.PeerLabel) []apitypes.PeerLabel {
	out := make([]apitypes.PeerLabel, 0, len(in))
	for _, item := range in {
		out = append(out, apitypes.PeerLabel{
			Key:   item.Key,
			Value: item.Value,
		})
	}
	return out
}

func optionalStrings(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	out := append([]string(nil), values...)
	return &out
}

func equalStringPtr(left, right *string) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return *left == *right
	}
}

func equalPeerIMEISlice(current *[]apitypes.PeerIMEI, next []apitypes.PeerIMEI) bool {
	if current == nil {
		return len(next) == 0
	}
	if len(*current) != len(next) {
		return false
	}
	for i := range next {
		if !equalStringPtr((*current)[i].Name, next[i].Name) ||
			(*current)[i].Tac != next[i].Tac ||
			(*current)[i].Serial != next[i].Serial {
			return false
		}
	}
	return true
}

func equalPeerLabelSlice(current *[]apitypes.PeerLabel, next []apitypes.PeerLabel) bool {
	if current == nil {
		return len(next) == 0
	}
	if len(*current) != len(next) {
		return false
	}
	for i := range next {
		if (*current)[i].Key != next[i].Key || (*current)[i].Value != next[i].Value {
			return false
		}
	}
	return true
}
