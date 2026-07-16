# Admin HTTP · Logs

`Implementation file: peer_service_serve_admin_logs.go`

Implement Admin SSE stream for Server log: parse filter conditions, handle first events and streaming errors, and encode SSE events.

Log query contract is located in Server Log Query; the actual log backend is provided by the host.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `StreamServerLogs` | Verify query and establish Admin SSE log stream. |
| `serverLogStreamRequestFromParams` | Convert HTTP query params into log backend requests. |
| `streamServerLogsResponse` | Holds the first event and subsequent stream writers. |
| `waitFirstServerLogEvent` | Wait for the first event or error before sending HTTP headers. |
| `writeServerLogSSE` | Encoding SSE events and JSON data. |
