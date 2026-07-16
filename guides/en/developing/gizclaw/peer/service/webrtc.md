# Peer HTTP · WebRTC

`Implementation file: peer_service_webrtc.go`

Giznet WebRTC Offer endpoint that implements Peer HTTP: transfers typed API request to shared signaling handler, and then converts HTTP status, body and error into generated API response.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `peerHTTP.CreateGiznetWebRTCOffer` | Receive typed Offer request and call Giznet signaling handler. |
| `signalingResponseRecorder` | Capture the status, headers and body written by the signaling handler. |
| `createGiznetWebRTCOfferResponse` | Convert signaling HTTP results to generate API responses. |
| `signalingErrorPayload` | Convert the signaling error body into a stable error structure. |
