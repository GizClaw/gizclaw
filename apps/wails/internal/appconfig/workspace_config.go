package appconfig

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"text/template"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/goccy/go-yaml"
)

//go:embed templates/local_server_workspace.yaml.gotmpl
var localServerWorkspaceTemplate string

type localServerWorkspaceData struct {
	PrivateKey     string
	Listen         string
	Endpoint       string
	ServeToClients bool
	AdminPublicKey string
}

func materializeLocalServerWorkspace(pod Pod, configPath string) error {
	privateKey, err := preservedLocalServerIdentity(configPath)
	if err != nil {
		return err
	}
	if privateKey == "" {
		keyPair, err := giznet.GenerateKeyPair()
		if err != nil {
			return fmt.Errorf("appconfig: generate local server identity: %w", err)
		}
		privateKey = keyPair.Private.String()
	}
	adminPublicKey := ""
	if pod.LocalServer.AdminPrivateKey != "" {
		keyPair, err := keyPair(pod.LocalServer.AdminPrivateKey)
		if err != nil {
			return err
		}
		adminPublicKey = keyPair.Public.String()
	}
	data := localServerWorkspaceData{
		PrivateKey: privateKey, Listen: fmt.Sprintf("0.0.0.0:%d", pod.LocalServer.Port),
		Endpoint:       PreferredLANEndpoint(pod.LocalServer.Port),
		ServeToClients: pod.ClientPrivateKey != "" || pod.LocalServer.AdminPrivateKey != "",
		AdminPublicKey: adminPublicKey,
	}
	functions := template.FuncMap{"quote": func(value string) string {
		encoded, _ := json.Marshal(value)
		return string(encoded)
	}}
	tmpl, err := template.New("local_server_workspace.yaml").Funcs(functions).Option("missingkey=error").Parse(localServerWorkspaceTemplate)
	if err != nil {
		return fmt.Errorf("appconfig: parse embedded workspace template: %w", err)
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		return fmt.Errorf("appconfig: render embedded workspace template: %w", err)
	}
	var check map[string]any
	if err := yaml.Unmarshal(rendered.Bytes(), &check); err != nil {
		return fmt.Errorf("appconfig: validate rendered workspace config: %w", err)
	}
	return atomicWrite(configPath, rendered.Bytes(), 0o600)
}

func preservedLocalServerIdentity(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("appconfig: read existing workspace config: %w", err)
	}
	var existing struct {
		Identity struct {
			PrivateKey giznet.Key `yaml:"private-key"`
		} `yaml:"identity"`
	}
	if err := yaml.Unmarshal(data, &existing); err != nil {
		return "", fmt.Errorf("appconfig: parse existing workspace config: %w", err)
	}
	if existing.Identity.PrivateKey.IsZero() {
		return "", errors.New("appconfig: existing workspace config is missing its server identity")
	}
	return existing.Identity.PrivateKey.String(), nil
}
