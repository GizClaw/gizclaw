package provider

// RegistrationToken uses nullable limits and defaults enabled to true. Preserve
// configured omissions while still detecting an administrator's restrictions.
func registrationTokenDefault(kind string, path []string, value any) bool {
	if kind != "RegistrationToken" || len(path) != 1 {
		return false
	}
	switch path[0] {
	case "enabled":
		return value == nil || value == true
	case "expires_at", "max_activations":
		return value == nil
	default:
		return false
	}
}
