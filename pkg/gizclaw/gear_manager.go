package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/acl"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/credential"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/model"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peer"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peerrun"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/workflow"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/workspace"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

var (
	ErrDeviceOffline = errors.New("gizclaw: device offline")
	errNoRefreshData = errors.New("gizclaw: no refresh data")
)

type activePeer struct {
	conn *giznet.Conn
}

type Manager struct {
	Peers   *peer.Server
	PeerRun *peerrun.Server
	ACL     *acl.Server

	Workspaces  workspace.WorkspaceAdminService
	Workflows   workflow.WorkflowAdminService
	Models      model.ModelAdminService
	Credentials credential.CredentialAdminService

	mu    sync.RWMutex
	peers map[giznet.PublicKey]*activePeer
}

func NewManager(peersService *peer.Server) *Manager {
	return &Manager{
		Peers: peersService,
		peers: make(map[giznet.PublicKey]*activePeer),
	}
}

func (m *Manager) allowService(ctx context.Context, publicKey giznet.PublicKey, service uint64) bool {
	switch service {
	case ServiceRPC, ServiceServerPublic:
		return true
	}
	switch service {
	case ServiceAdmin:
		gear, err := m.Peers.LoadGear(ctx, publicKey)
		if err != nil {
			return false
		}
		return gear.Status == apitypes.GearStatusActive && gear.Role == apitypes.GearRoleAdmin
	default:
		return false
	}
}

func (m *Manager) SetPeerUp(publicKey giznet.PublicKey, conn *giznet.Conn) {
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
	state.conn = conn
}

func (m *Manager) SetPeerDown(publicKey giznet.PublicKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.peers, publicKey)
}

func (m *Manager) Peer(publicKey giznet.PublicKey) (*giznet.Conn, bool) {
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

func (m *Manager) EnsurePeer(ctx context.Context, publicKey giznet.PublicKey) (apitypes.Gear, error) {
	if m == nil || m.Peers == nil {
		return apitypes.Gear{}, errors.New("gizclaw: peers service not configured")
	}
	return m.Peers.EnsureConnectedGear(ctx, publicKey)
}

func (m *Manager) RefreshPeer(ctx context.Context, publicKey giznet.PublicKey) (adminservice.RefreshResult, bool, error) {
	if m.Peers == nil {
		return adminservice.RefreshResult{}, false, errors.New("gizclaw: peers service not configured")
	}
	gear, err := m.Peers.LoadGear(ctx, publicKey)
	if err != nil {
		return adminservice.RefreshResult{}, false, err
	}
	next, updatedFields, errs, err := m.refreshPeer(ctx, publicKey, gear)
	if err != nil {
		online := true
		if errors.Is(err, ErrDeviceOffline) {
			m.SetPeerDown(publicKey)
			online = false
		}
		return adminservice.RefreshResult{
			Gear:   gear,
			Errors: optionalStrings(errs),
		}, online, err
	}
	if len(updatedFields) == 0 {
		return adminservice.RefreshResult{
			Gear:          next,
			Errors:        optionalStrings(errs),
			UpdatedFields: nil,
		}, true, nil
	}
	saved, err := m.Peers.SaveGear(ctx, next)
	if err != nil {
		return adminservice.RefreshResult{
			Gear:          next,
			Errors:        optionalStrings(errs),
			UpdatedFields: optionalStrings(updatedFields),
		}, true, err
	}
	return adminservice.RefreshResult{
		Gear:          saved,
		Errors:        optionalStrings(errs),
		UpdatedFields: optionalStrings(updatedFields),
	}, true, nil
}

func (m *Manager) peerRPCConn(publicKey giznet.PublicKey) (net.Conn, error) {
	m.mu.RLock()
	state, ok := m.peers[publicKey]
	var peerConn *giznet.Conn
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

func (m *Manager) refreshPeer(ctx context.Context, publicKey giznet.PublicKey, gear apitypes.Gear) (apitypes.Gear, []string, []string, error) {
	var (
		errs          []string
		updatedFields []string
		haveData      bool
		disconnected  bool
	)

	infoResp, err := callPeerRPC(m, ctx, publicKey, func(client *rpcClient, conn net.Conn) (*rpcapi.DeviceGetInfoResponse, error) {
		return client.GetDeviceInfo(ctx, conn, "device.info.get")
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
			applyPeerRefreshInfo(&gear, info, &updatedFields)
		}
	}

	identifiersResp, err := callPeerRPC(m, ctx, publicKey, func(client *rpcClient, conn net.Conn) (*rpcapi.DeviceGetIdentifiersResponse, error) {
		return client.GetDeviceIdentifiers(ctx, conn, "device.identifiers.get")
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
			applyPeerRefreshIdentifiers(&gear, identifiers, &updatedFields)
		}
	}

	if !haveData {
		if disconnected {
			return gear, updatedFields, errs, ErrDeviceOffline
		}
		return gear, updatedFields, errs, errNoRefreshData
	}
	return gear, updatedFields, errs, nil
}

func isPeerDisconnectedError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "conn closed") ||
		strings.Contains(msg, "closed network connection")
}

func applyPeerRefreshInfo(gear *apitypes.Gear, info apitypes.RefreshInfo, updatedFields *[]string) {
	if gear == nil {
		return
	}
	if info.Name != nil && *info.Name != "" && !equalStringPtr(gear.Device.Name, info.Name) {
		gear.Device.Name = info.Name
		*updatedFields = append(*updatedFields, "device.name")
	}
	if info.Manufacturer != nil && *info.Manufacturer != "" {
		hardware := ensureGearHardware(&gear.Device)
		if !equalStringPtr(hardware.Manufacturer, info.Manufacturer) {
			hardware.Manufacturer = info.Manufacturer
			*updatedFields = append(*updatedFields, "device.hardware.manufacturer")
		}
	}
	if info.Model != nil && *info.Model != "" {
		hardware := ensureGearHardware(&gear.Device)
		if !equalStringPtr(hardware.Model, info.Model) {
			hardware.Model = info.Model
			*updatedFields = append(*updatedFields, "device.hardware.model")
		}
	}
	if info.HardwareRevision != nil && *info.HardwareRevision != "" {
		hardware := ensureGearHardware(&gear.Device)
		if !equalStringPtr(hardware.HardwareRevision, info.HardwareRevision) {
			hardware.HardwareRevision = info.HardwareRevision
			*updatedFields = append(*updatedFields, "device.hardware.hardware_revision")
		}
	}
}

func applyPeerRefreshIdentifiers(gear *apitypes.Gear, identifiers apitypes.RefreshIdentifiers, updatedFields *[]string) {
	if gear == nil {
		return
	}
	if identifiers.Sn != nil && *identifiers.Sn != "" && !equalStringPtr(gear.Device.Sn, identifiers.Sn) {
		gear.Device.Sn = identifiers.Sn
		*updatedFields = append(*updatedFields, "device.sn")
	}
	if identifiers.Imeis != nil && len(*identifiers.Imeis) > 0 {
		items := toGearIMEIs(*identifiers.Imeis)
		hardware := ensureGearHardware(&gear.Device)
		if !equalGearIMEISlice(hardware.Imeis, items) {
			hardware.Imeis = &items
			*updatedFields = append(*updatedFields, "device.hardware.imeis")
		}
	}
	if identifiers.Labels != nil && len(*identifiers.Labels) > 0 {
		items := toGearLabels(*identifiers.Labels)
		hardware := ensureGearHardware(&gear.Device)
		if !equalGearLabelSlice(hardware.Labels, items) {
			hardware.Labels = &items
			*updatedFields = append(*updatedFields, "device.hardware.labels")
		}
	}
}

func ensureGearHardware(device *apitypes.DeviceInfo) *apitypes.HardwareInfo {
	if device.Hardware == nil {
		device.Hardware = &apitypes.HardwareInfo{}
	}
	return device.Hardware
}

func toGearIMEIs(in []apitypes.GearIMEI) []apitypes.GearIMEI {
	out := make([]apitypes.GearIMEI, 0, len(in))
	for _, item := range in {
		out = append(out, apitypes.GearIMEI{
			Name:   item.Name,
			Tac:    item.Tac,
			Serial: item.Serial,
		})
	}
	return out
}

func toGearLabels(in []apitypes.GearLabel) []apitypes.GearLabel {
	out := make([]apitypes.GearLabel, 0, len(in))
	for _, item := range in {
		out = append(out, apitypes.GearLabel{
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

func equalGearIMEISlice(current *[]apitypes.GearIMEI, next []apitypes.GearIMEI) bool {
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

func equalGearLabelSlice(current *[]apitypes.GearLabel, next []apitypes.GearLabel) bool {
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
