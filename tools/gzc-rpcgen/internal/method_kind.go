package rpcgen

func methodKind(method string) string {
	switch method {
	case "all.speed_test.run":
		return "GZC_RPC_METHOD_KIND_BINARY_STREAM"
	case "server.firmware.files.download", "server.workspace.history.audio.get":
		return "GZC_RPC_METHOD_KIND_BINARY_DOWNLOAD"
	default:
		return "GZC_RPC_METHOD_KIND_UNARY"
	}
}
