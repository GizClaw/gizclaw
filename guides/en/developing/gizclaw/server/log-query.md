# Log Query

`Implementation file: server_log_query.go`

Define the service interface, request parameters, sorting method, result type and structured errors for Server log query and streaming reading, and map query errors to HTTP error responses.

There is a query contract here; the specific log backend is injected by the host.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `ServerLogStreamRequest` | Describe log filtering, sorting, cursor and follow parameters. |
| `ServerLogQueryService` | The host log backend needs to implement the streaming query interface. |
| `ServerLogQueryError` | Carry stable error code and underlying error. |
| `InvalidServerLogQuery` | Invalid query construction error. |
| `LogQueryNotConfigured` | Indicates that the Server is not configured with a log query backend. |
| `ServerLogBackendError` | Package backend execution error. |
