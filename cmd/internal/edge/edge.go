package edge

// Serve loads and validates an edge-node workspace. The listener and
// edge-to-server data plane are intentionally added in later implementation
// steps so this first slice cannot appear to proxy traffic successfully.
func Serve(root string) error {
	if _, err := PrepareWorkspaceConfig(root); err != nil {
		return err
	}
	return ErrRuntimeNotImplemented
}
