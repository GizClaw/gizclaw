# OpenAI HTTP

`Implementation file: server_openai_http.go`

Assemble the Peer-scoped OpenAI-compatible handler for the ordinary Server HTTP entry, and access the public login session and the corresponding Peer resource view.

The domain implementation of the OpenAI API belongs to the AI ​​service; this file is only responsible for Server-level HTTP composition.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `peerOpenAIHTTPHandler` | Assemble the OpenAI-compatible handler based on the Peer identity in the HTTP session. |
