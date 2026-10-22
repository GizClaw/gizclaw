# Firmware RPC

A device queries a release channel from its bound Firmware, then downloads and verifies the package itself. Admin manages Firmware resources and bindings; the RPC returns release information rather than proxying package downloads.

[RPC schema](https://github.com/GizClaw/gizclaw/blob/main/api/proto/rpc/payload/firmware.proto) · [Firmware Resource schema](https://github.com/GizClaw/gizclaw/blob/main/api/http/resources/firmware.json) · [Query implementation](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/services/runtime/peerresource/firmware.go)

The version identifies a release for display; the URL, digest, and size support downloading and integrity checks. Verify the compressed package before handing it to the device's installation flow. Receiving release information does not prove OTA completion.

## Firmware metadata

Metadata provides application-owned JSON configuration independently of release channels. For example, a device can read auxiliary modem release information from one key:

```yaml
spec:
  metadata:
    modem:
      version: vendor-release
      urls:
        - https://firmware.example.com/modem/image.bin
```

Server does not interpret the values. Read a key and decode the returned JSON text into the application's own structure; an explicit JSON null differs from a missing key.

Consult the schemas above for requests, responses, supported channels, and validation limits. Use [RPC Reference](/references/rpc#firmware) and language API references for SDK methods. C callers also follow the [request lifecycle](/references/streams#binary-streams).
