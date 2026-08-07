package adminhttp

// MarshalJSON preserves the referenced oneOf payload when the generated
// strict response wrapper is encoded by Fiber.
func (response DeletePeer200JSONResponse) MarshalJSON() ([]byte, error) {
	return PeerRegistrationResult(response).MarshalJSON()
}

// MarshalJSON preserves the referenced oneOf payload when the generated
// strict response wrapper is encoded by Fiber.
func (response GetPeer200JSONResponse) MarshalJSON() ([]byte, error) {
	return PeerRegistrationResult(response).MarshalJSON()
}
