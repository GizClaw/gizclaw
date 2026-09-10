package paths

import "github.com/GizClaw/gizclaw-go/sdk/go/gizcli/contextconn"

// ConfigDir returns the gizclaw configuration root directory.
//
// On Unix-like systems (Linux, macOS), it follows the XDG convention:
//
//	$XDG_CONFIG_HOME/gizclaw  (if set)
//	~/.config/gizclaw          (fallback)
//
// On Windows, it uses the standard app-data location:
//
//	%AppData%\gizclaw          (via os.UserConfigDir)
func ConfigDir() (string, error) {
	return contextconn.ConfigDir()
}
