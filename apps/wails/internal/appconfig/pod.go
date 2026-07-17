package appconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/contextstore"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/goccy/go-yaml"
)

const (
	PodManifestFile         = "pod.json"
	PodInitializationMarker = ".initializing"
	PodVersion              = 1
	DefaultPort             = 9820
)

var (
	podIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	hostPattern  = regexp.MustCompile(`(?i)^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)
)

type Pod struct {
	Version               int            `json:"version"`
	ID                    string         `json:"id"`
	Name                  string         `json:"name"`
	Description           string         `json:"description,omitempty"`
	IdentitiesInitialized bool           `json:"identities_initialized,omitempty"`
	LocalServer           *LocalServer   `json:"local_server,omitempty"`
	RemoteServers         []RemoteServer `json:"remote_servers,omitempty"`
	RemoteAccessPoint     string         `json:"remote_access_point,omitempty"`
	ClientPrivateKey      string         `json:"client_private_key,omitempty"`
}

type LocalServer struct {
	Port            int    `json:"port"`
	AdminPrivateKey string `json:"admin_private_key,omitempty"`
}

type RemoteServer struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Endpoint        string `json:"endpoint"`
	AdminPrivateKey string `json:"admin_private_key,omitempty"`
}

type Store struct {
	Paths           Paths
	materializeHook func(Pod) error
}

type Entry struct {
	ID  string
	Pod Pod
	Err error
}

type PodInitialization struct {
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

func (s Store) LocalServerPublicKey(id string) (string, error) {
	pod, err := s.Load(id)
	if err != nil {
		return "", err
	}
	if pod.LocalServer == nil {
		return "", fmt.Errorf("appconfig: pod %q is not local", id)
	}
	data, err := os.ReadFile(filepath.Join(s.Paths.PodsDir, id, "workspace", "config.yaml"))
	if err != nil {
		return "", fmt.Errorf("appconfig: read local server identity: %w", err)
	}
	var config struct {
		Identity struct {
			PrivateKey giznet.Key `yaml:"private-key"`
		} `yaml:"identity"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("appconfig: parse local server identity: %w", err)
	}
	if config.Identity.PrivateKey.IsZero() {
		return "", fmt.Errorf("appconfig: local server identity is missing")
	}
	kp, err := giznet.NewKeyPair(config.Identity.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("appconfig: derive local server public key: %w", err)
	}
	return kp.Public.String(), nil
}

func (s Store) List() ([]Pod, error) {
	entries, err := s.Entries()
	if err != nil {
		return nil, err
	}
	pods := make([]Pod, 0, len(entries))
	for _, entry := range entries {
		if entry.Err != nil {
			return nil, entry.Err
		}
		pods = append(pods, entry.Pod)
	}
	return pods, nil
}

func (s Store) Entries() ([]Entry, error) {
	entries, err := os.ReadDir(s.Paths.PodsDir)
	if err != nil {
		return nil, fmt.Errorf("appconfig: list pods: %w", err)
	}
	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest := filepath.Join(s.Paths.PodsDir, entry.Name(), PodManifestFile)
		if _, err := os.Lstat(manifest); os.IsNotExist(err) {
			continue
		}
		pod, err := s.Load(entry.Name())
		if err != nil {
			result = append(result, Entry{ID: entry.Name(), Err: err})
			continue
		}
		result = append(result, Entry{ID: pod.ID, Pod: pod})
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i].Pod.Name, result[j].Pod.Name
		if left == "" {
			left = result[i].ID
		}
		if right == "" {
			right = result[j].ID
		}
		return strings.ToLower(left) < strings.ToLower(right)
	})
	return result, nil
}

// MarkInitializing records that a visible Pod is still installing its local
// Server bootstrap catalog.
func (s Store) MarkInitializing(id string) error {
	if err := validateID("pod id", id); err != nil {
		return err
	}
	dir := filepath.Join(s.Paths.PodsDir, id)
	if err := secureDir(dir); err != nil {
		return fmt.Errorf("appconfig: secure initializing pod: %w", err)
	}
	return s.writeInitialization(id, PodInitialization{State: "initializing"})
}

func (s Store) CompleteInitialization(id string) error {
	if err := validateID("pod id", id); err != nil {
		return err
	}
	marker := filepath.Join(s.Paths.PodsDir, id, PodInitializationMarker)
	if err := os.Remove(marker); err != nil {
		return fmt.Errorf("appconfig: complete pod initialization: %w", err)
	}
	return nil
}

func (s Store) FailInitialization(id string, cause error) error {
	if cause == nil {
		return errors.New("appconfig: initialization failure cause is required")
	}
	return s.writeInitialization(id, PodInitialization{State: "failed", Error: cause.Error()})
}

func (s Store) Initialization(id string) (*PodInitialization, error) {
	if err := validateID("pod id", id); err != nil {
		return nil, err
	}
	marker := filepath.Join(s.Paths.PodsDir, id, PodInitializationMarker)
	info, err := os.Lstat(marker)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("appconfig: inspect pod initialization: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("appconfig: pod initialization marker must be a regular file")
	}
	data, err := os.ReadFile(marker)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("appconfig: read pod initialization: %w", err)
	}
	if string(data) == "initializing\n" {
		return &PodInitialization{State: "initializing"}, nil
	}
	var status PodInitialization
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&status); err != nil {
		return nil, fmt.Errorf("appconfig: parse pod initialization: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return nil, errors.New("appconfig: parse pod initialization: trailing JSON value")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("appconfig: parse pod initialization: trailing data: %w", err)
	}
	if status.State != "initializing" && status.State != "failed" {
		return nil, fmt.Errorf("appconfig: unsupported pod initialization state %q", status.State)
	}
	if status.State == "failed" && strings.TrimSpace(status.Error) == "" {
		return nil, errors.New("appconfig: failed pod initialization is missing an error")
	}
	return &status, nil
}

func (s Store) writeInitialization(id string, status PodInitialization) error {
	if err := validateID("pod id", id); err != nil {
		return err
	}
	dir := filepath.Join(s.Paths.PodsDir, id)
	if err := secureDir(dir); err != nil {
		return fmt.Errorf("appconfig: secure initializing pod: %w", err)
	}
	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("appconfig: encode pod initialization: %w", err)
	}
	return atomicWrite(filepath.Join(dir, PodInitializationMarker), append(data, '\n'), 0o600)
}

// CleanupIncomplete removes Pods whose creation was interrupted before the
// initialization marker was cleared.
func (s Store) CleanupIncomplete() error {
	entries, err := os.ReadDir(s.Paths.PodsDir)
	if err != nil {
		return fmt.Errorf("appconfig: list pods for initialization cleanup: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(s.Paths.PodsDir, entry.Name())
		status, err := s.Initialization(entry.Name())
		if status == nil && err == nil {
			continue
		} else if err != nil {
			return fmt.Errorf("appconfig: inspect initializing pod %q: %w", entry.Name(), err)
		}
		if status.State == "failed" {
			continue
		}
		if err := rejectSymlinkDir(dir); err != nil {
			return err
		}
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("appconfig: remove incomplete pod %q: %w", entry.Name(), err)
		}
	}
	return nil
}

func (s Store) Load(id string) (Pod, error) {
	if err := validateID("pod id", id); err != nil {
		return Pod{}, err
	}
	dir := filepath.Join(s.Paths.PodsDir, id)
	if err := rejectSymlinkDir(dir); err != nil {
		return Pod{}, err
	}
	data, err := os.ReadFile(filepath.Join(dir, PodManifestFile))
	if err != nil {
		return Pod{}, fmt.Errorf("appconfig: read pod %q: %w", id, err)
	}
	var pod Pod
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&pod); err != nil {
		return Pod{}, fmt.Errorf("appconfig: parse pod %q: %w", id, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return Pod{}, fmt.Errorf("appconfig: parse pod %q: trailing JSON value", id)
	} else if !errors.Is(err, io.EOF) {
		return Pod{}, fmt.Errorf("appconfig: parse pod %q: trailing data: %w", id, err)
	}
	if pod.ID != id {
		return Pod{}, fmt.Errorf("appconfig: pod directory %q does not match manifest id %q", id, pod.ID)
	}
	if err := pod.Validate(); err != nil {
		return Pod{}, fmt.Errorf("appconfig: pod %q: %w", id, err)
	}
	return pod, nil
}

func (s Store) Save(pod Pod) error {
	if err := pod.Validate(); err != nil {
		return err
	}
	if err := normalizeSecrets(&pod); err != nil {
		return err
	}
	manifest := filepath.Join(s.Paths.PodsDir, pod.ID, PodManifestFile)
	previousManifest, previousErr := os.ReadFile(manifest)
	hadManifest := previousErr == nil
	if previousErr != nil && !os.IsNotExist(previousErr) {
		return fmt.Errorf("appconfig: read previous pod manifest: %w", previousErr)
	}
	if _, err := os.Lstat(manifest); err == nil {
		existing, loadErr := s.Load(pod.ID)
		if loadErr != nil {
			return fmt.Errorf("appconfig: refuse to overwrite invalid pod %q: %w", pod.ID, loadErr)
		}
		if err := s.verifyProjections(existing); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("appconfig: inspect pod manifest: %w", err)
	}
	dir := filepath.Join(s.Paths.PodsDir, pod.ID)
	if err := secureDir(dir); err != nil {
		return fmt.Errorf("appconfig: secure pod directory: %w", err)
	}
	backup, err := beginProjectionBackup(dir, pod.LocalServer == nil)
	if err != nil {
		return err
	}
	defer backup.discard()
	data, err := json.MarshalIndent(pod, "", "  ")
	if err != nil {
		_ = backup.rollback()
		return fmt.Errorf("appconfig: encode pod: %w", err)
	}
	data = append(data, '\n')
	if err := atomicWrite(filepath.Join(dir, PodManifestFile), data, 0o600); err != nil {
		_ = backup.rollback()
		return err
	}
	materialize := s.materialize
	if s.materializeHook != nil {
		materialize = s.materializeHook
	}
	if err := materialize(pod); err != nil {
		projectionErr := backup.rollback()
		var rollbackErr error
		if hadManifest {
			rollbackErr = atomicWrite(manifest, previousManifest, 0o600)
		} else {
			rollbackErr = os.Remove(manifest)
			if os.IsNotExist(rollbackErr) {
				rollbackErr = nil
			}
		}
		if rollbackErr != nil || projectionErr != nil {
			return fmt.Errorf("%w; restore projections: %v; restore pod manifest: %v", err, projectionErr, rollbackErr)
		}
		return err
	}
	return nil
}

func (s Store) verifyProjections(pod Pod) error {
	dir := filepath.Join(s.Paths.PodsDir, pod.ID)
	check := func(path, key, endpoint string) error {
		if key == "" {
			return nil
		}
		config, err := contextstore.LoadConfig(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("appconfig: conflicting materialized context %q: %w", path, err)
		}
		want, err := parsePrivateKey(key)
		if err != nil {
			return err
		}
		if config.Identity.PrivateKey != want || config.Server.Endpoint != endpoint {
			return fmt.Errorf("appconfig: materialized context %q conflicts with pod.json; repair or remove it before updating", path)
		}
		return nil
	}
	if pod.LocalServer != nil {
		endpoint := fmt.Sprintf("127.0.0.1:%d", pod.LocalServer.Port)
		if err := check(filepath.Join(dir, "admin_context", "local"), pod.LocalServer.AdminPrivateKey, endpoint); err != nil {
			return err
		}
		if err := check(filepath.Join(dir, "client_context"), pod.ClientPrivateKey, endpoint); err != nil {
			return err
		}
		return nil
	}
	for _, server := range pod.RemoteServers {
		if err := check(filepath.Join(dir, "admin_context", server.ID), server.AdminPrivateKey, server.Endpoint); err != nil {
			return err
		}
	}
	return check(filepath.Join(dir, "client_context"), pod.ClientPrivateKey, pod.RemoteAccessPoint)
}

type projectionBackup struct {
	dir                 string
	root                string
	workspaceMoved      bool
	workspaceExisted    bool
	workspaceConfig     []byte
	workspaceConfigMode os.FileMode
	workspaceConfigSet  bool
	moved               []string
}

func beginProjectionBackup(dir string, moveWorkspace bool) (*projectionBackup, error) {
	for _, name := range []string{"workspace", "admin_context", "client_context"} {
		if err := rejectSymlinkDir(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	root, err := os.MkdirTemp(dir, ".projection-backup-")
	if err != nil {
		return nil, fmt.Errorf("appconfig: create projection backup: %w", err)
	}
	b := &projectionBackup{dir: dir, root: root}
	workspace := filepath.Join(dir, "workspace")
	if _, statErr := os.Stat(workspace); statErr == nil {
		b.workspaceExisted = true
		if moveWorkspace {
			if err := os.Rename(workspace, filepath.Join(root, "workspace")); err != nil {
				b.discard()
				return nil, fmt.Errorf("appconfig: back up workspace: %w", err)
			}
			b.workspaceMoved = true
		} else {
			configPath := filepath.Join(workspace, "config.yaml")
			if data, readErr := os.ReadFile(configPath); readErr == nil {
				b.workspaceConfig = data
				b.workspaceConfigSet = true
				if configInfo, infoErr := os.Stat(configPath); infoErr == nil {
					b.workspaceConfigMode = configInfo.Mode().Perm()
				}
			} else if !os.IsNotExist(readErr) {
				b.discard()
				return nil, fmt.Errorf("appconfig: back up workspace config: %w", readErr)
			}
		}
	} else if !os.IsNotExist(statErr) {
		b.discard()
		return nil, fmt.Errorf("appconfig: inspect workspace: %w", statErr)
	}
	for _, name := range []string{"admin_context", "client_context"} {
		path := filepath.Join(dir, name)
		if _, statErr := os.Stat(path); statErr == nil {
			if err := os.Rename(path, filepath.Join(root, name)); err != nil {
				_ = b.abort()
				return nil, fmt.Errorf("appconfig: back up %s: %w", name, err)
			}
			b.moved = append(b.moved, name)
		} else if !os.IsNotExist(statErr) {
			_ = b.abort()
			return nil, fmt.Errorf("appconfig: inspect %s: %w", name, statErr)
		}
	}
	return b, nil
}

func (b *projectionBackup) abort() error {
	var errs []error
	for i := len(b.moved) - 1; i >= 0; i-- {
		name := b.moved[i]
		if err := os.Rename(filepath.Join(b.root, name), filepath.Join(b.dir, name)); err != nil {
			errs = append(errs, err)
		}
	}
	if b.workspaceMoved {
		if err := os.Rename(filepath.Join(b.root, "workspace"), filepath.Join(b.dir, "workspace")); err != nil {
			errs = append(errs, err)
		}
	}
	b.discard()
	return errors.Join(errs...)
}

func (b *projectionBackup) rollback() error {
	var errs []error
	for _, name := range []string{"admin_context", "client_context"} {
		if err := os.RemoveAll(filepath.Join(b.dir, name)); err != nil {
			errs = append(errs, err)
		}
	}
	for _, name := range b.moved {
		if err := os.Rename(filepath.Join(b.root, name), filepath.Join(b.dir, name)); err != nil {
			errs = append(errs, err)
		}
	}
	workspace := filepath.Join(b.dir, "workspace")
	if b.workspaceMoved {
		if err := os.RemoveAll(workspace); err != nil {
			errs = append(errs, err)
		} else if err := os.Rename(filepath.Join(b.root, "workspace"), workspace); err != nil {
			errs = append(errs, err)
		}
	} else if b.workspaceConfigSet {
		mode := b.workspaceConfigMode
		if mode == 0 {
			mode = 0o600
		}
		if err := atomicWrite(filepath.Join(workspace, "config.yaml"), b.workspaceConfig, mode); err != nil {
			errs = append(errs, err)
		}
	} else if !b.workspaceExisted {
		if err := os.RemoveAll(workspace); err != nil {
			errs = append(errs, err)
		}
	} else if err := os.Remove(filepath.Join(workspace, "config.yaml")); err != nil && !os.IsNotExist(err) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (b *projectionBackup) discard() { _ = os.RemoveAll(b.root) }

func (s Store) Delete(id string) error {
	if err := validateID("pod id", id); err != nil {
		return err
	}
	dir := filepath.Join(s.Paths.PodsDir, id)
	if _, err := os.Stat(filepath.Join(dir, PodManifestFile)); err != nil {
		return fmt.Errorf("appconfig: delete pod %q: %w", id, err)
	}
	return os.RemoveAll(dir)
}

func (s Store) PodDir(id string) (string, error) {
	if err := validateID("pod id", id); err != nil {
		return "", err
	}
	dir := filepath.Join(s.Paths.PodsDir, id)
	if err := rejectSymlinkDir(dir); err != nil {
		return "", fmt.Errorf("appconfig: open pod %q: %w", id, err)
	}
	return dir, nil
}

func (p Pod) Validate() error {
	if p.Version != PodVersion {
		return fmt.Errorf("unsupported pod version %d", p.Version)
	}
	if err := validateID("pod id", p.ID); err != nil {
		return err
	}
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("pod name is required")
	}
	local := p.LocalServer != nil
	remote := p.RemoteAccessPoint != ""
	if local == remote {
		return errors.New("configure exactly one of local_server or remote_servers with remote_access_point")
	}
	if local {
		if len(p.RemoteServers) != 0 {
			return errors.New("local Pods cannot configure remote_servers")
		}
		if p.LocalServer.Port < 1 || p.LocalServer.Port > 65535 {
			return errors.New("local_server.port must be between 1 and 65535")
		}
		if err := validatePrivateKey("local_server.admin_private_key", p.LocalServer.AdminPrivateKey); err != nil {
			return err
		}
	} else {
		if err := validateEndpoint("remote_access_point", p.RemoteAccessPoint); err != nil {
			return err
		}
		seen := map[string]bool{}
		for i, server := range p.RemoteServers {
			prefix := fmt.Sprintf("remote_servers[%d]", i)
			if err := validateID(prefix+".id", server.ID); err != nil {
				return err
			}
			if seen[server.ID] {
				return fmt.Errorf("duplicate remote server id %q", server.ID)
			}
			seen[server.ID] = true
			if strings.TrimSpace(server.Name) == "" {
				return fmt.Errorf("%s.name is required", prefix)
			}
			if err := validateEndpoint(prefix+".endpoint", server.Endpoint); err != nil {
				return err
			}
			if err := validatePrivateKey(prefix+".admin_private_key", server.AdminPrivateKey); err != nil {
				return err
			}
		}
	}
	return validatePrivateKey("client_private_key", p.ClientPrivateKey)
}

func (s Store) materialize(pod Pod) error {
	dir := filepath.Join(s.Paths.PodsDir, pod.ID)
	if pod.LocalServer != nil {
		if err := s.materializeWorkspace(pod, dir); err != nil {
			return err
		}
	} else if err := os.RemoveAll(filepath.Join(dir, "workspace")); err != nil {
		return fmt.Errorf("appconfig: remove remote pod workspace: %w", err)
	}
	adminRoot := filepath.Join(dir, "admin_context")
	if err := os.RemoveAll(adminRoot); err != nil {
		return fmt.Errorf("appconfig: reset admin contexts: %w", err)
	}
	if pod.LocalServer != nil && pod.LocalServer.AdminPrivateKey != "" {
		if err := writeContext(filepath.Join(adminRoot, "local"), pod.Name+" Admin", pod.LocalServer.AdminPrivateKey, fmt.Sprintf("127.0.0.1:%d", pod.LocalServer.Port)); err != nil {
			return err
		}
	}
	for _, server := range pod.RemoteServers {
		if server.AdminPrivateKey == "" {
			continue
		}
		if err := writeContext(filepath.Join(adminRoot, server.ID), server.Name+" Admin", server.AdminPrivateKey, server.Endpoint); err != nil {
			return err
		}
	}
	clientDir := filepath.Join(dir, "client_context")
	if pod.ClientPrivateKey == "" {
		return os.RemoveAll(clientDir)
	}
	endpoint := pod.RemoteAccessPoint
	if pod.LocalServer != nil {
		endpoint = fmt.Sprintf("127.0.0.1:%d", pod.LocalServer.Port)
	}
	return writeContext(clientDir, pod.Name+" Play", pod.ClientPrivateKey, endpoint)
}

func (s Store) materializeWorkspace(pod Pod, dir string) error {
	workspace := filepath.Join(dir, "workspace")
	if err := secureDir(workspace); err != nil {
		return fmt.Errorf("appconfig: create workspace: %w", err)
	}
	configPath := filepath.Join(workspace, "config.yaml")
	return materializeLocalServerWorkspace(pod, configPath)
}

func writeContext(dir, description, privateKey, endpoint string) error {
	key, err := parsePrivateKey(privateKey)
	if err != nil {
		return err
	}
	config := contextstore.Config{
		Description: description,
		Identity:    contextstore.IdentityConfig{PrivateKey: key},
		Server:      contextstore.ServerConfig{Endpoint: endpoint},
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("appconfig: encode context: %w", err)
	}
	if err := secureDir(dir); err != nil {
		return fmt.Errorf("appconfig: create context: %w", err)
	}
	return atomicWrite(filepath.Join(dir, contextstore.ConfigFile), data, 0o600)
}

func FindAvailablePort(preferred int) (int, error) {
	if preferred > 0 && CheckPortAvailable(preferred) == nil {
		return preferred, nil
	}
	for range 20 {
		listener, err := net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			return 0, fmt.Errorf("appconfig: choose local server port: %w", err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()
		if CheckPortAvailable(port) == nil {
			return port, nil
		}
	}
	return 0, fmt.Errorf("appconfig: no TCP and UDP port is available")
}

func CheckPortAvailable(port int) error {
	address := fmt.Sprintf("0.0.0.0:%d", port)
	tcpListener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer tcpListener.Close()
	udpListener, err := net.ListenPacket("udp", address)
	if err != nil {
		return err
	}
	return udpListener.Close()
}

func validateID(field, id string) error {
	if len(id) > 64 || !podIDPattern.MatchString(id) {
		return fmt.Errorf("%s must use lowercase letters, digits, and single hyphen separators", field)
	}
	return nil
}

func validateEndpoint(field, endpoint string) error {
	if endpoint == "" || strings.Contains(endpoint, "://") {
		return fmt.Errorf("%s must be host:port", field)
	}
	host, portText, err := net.SplitHostPort(endpoint)
	if err != nil || strings.TrimSpace(host) == "" || host != strings.TrimSpace(host) {
		return fmt.Errorf("%s must be host:port", field)
	}
	if net.ParseIP(host) == nil && !hostPattern.MatchString(host) {
		return fmt.Errorf("%s host must be an IP address or DNS name", field)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%s port must be between 1 and 65535", field)
	}
	return nil
}

func PreferredLANEndpoint(port int) string {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	}
	var candidates []string
	for _, address := range addresses {
		ip, _, parseErr := net.ParseCIDR(address.String())
		if parseErr != nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
			continue
		}
		candidates = append(candidates, ip.String())
	}
	if len(candidates) == 0 {
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	}
	sort.Strings(candidates)
	return net.JoinHostPort(candidates[0], strconv.Itoa(port))
}

func validatePrivateKey(field, value string) error {
	if value == "" {
		return nil
	}
	if _, err := keyPair(value); err != nil {
		return fmt.Errorf("%s is not a valid Giznet private key: %w", field, err)
	}
	return nil
}

func parsePrivateKey(value string) (giznet.Key, error) {
	var key giznet.Key
	if err := key.UnmarshalText([]byte(value)); err != nil {
		return giznet.Key{}, err
	}
	return key, nil
}

func keyPair(value string) (*giznet.KeyPair, error) {
	key, err := parsePrivateKey(value)
	if err != nil {
		return nil, err
	}
	return giznet.NewKeyPair(key)
}

func normalizeSecrets(pod *Pod) error {
	normalize := func(value *string) error {
		if *value == "" {
			return nil
		}
		kp, err := keyPair(*value)
		if err != nil {
			return err
		}
		*value = kp.Private.String()
		return nil
	}
	if pod.LocalServer != nil {
		if err := normalize(&pod.LocalServer.AdminPrivateKey); err != nil {
			return err
		}
	}
	for i := range pod.RemoteServers {
		if err := normalize(&pod.RemoteServers[i].AdminPrivateKey); err != nil {
			return err
		}
	}
	return normalize(&pod.ClientPrivateKey)
}

func secureDir(path string) error {
	if err := rejectSymlinkDir(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if err := rejectSymlinkDir(path); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func rejectSymlinkDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("appconfig: directory %q must not be a symbolic link", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("appconfig: %q is not a directory", path)
	}
	return nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("appconfig: create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("appconfig: replace %s: %w", path, err)
	}
	return os.Chmod(path, mode)
}
