package gizwebrtc

type addr string

func (a addr) Network() string { return "webrtc" }
func (a addr) String() string  { return string(a) }
