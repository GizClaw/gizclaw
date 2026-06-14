# 511 Admin Firmwares

## User Story

As an admin, I can manage a firmware release line from the CLI and let an
authorized device consume it through connect firmware RPC commands:

- put a complete firmware JSON document
- list and get the stored firmware
- upload bin payloads for declared channel/bin entries
- verify server-owned objectstore metadata is returned
- assign a peer firmware id/channel through peer config
- grant the peer `firmware.read` through ACL
- query firmware list/get through `gizclaw connect firmware`
- download a bin payload through `gizclaw connect firmware download`
- release slots atomically
- rollback by shifting develop/beta/stable/pending one step right
- show the same firmware through declarative resources
- delete the firmware
