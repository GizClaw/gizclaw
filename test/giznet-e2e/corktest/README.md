# giznet corktest

Manual giznet peer-connection probe for two failure modes:

- `trickle`: long-lived low-rate KCP streams with intermittent reader corking.
- `disconnect`: device disconnect propagation and server-side peer Conn cleanup.

This command is guarded by the `manual` build tag so it is not part of normal
`go test ./...` discovery.

Quick smoke:

```sh
go run -tags manual ./test/giznet-e2e/corktest \
  -mode trickle \
  -duration 3s \
  -streams 2 \
  -chunk 2048 \
  -cork-every 16 \
  -cork 20ms \
  -write-interval 50ms \
  -report 1s
```

Long trickle run:

```sh
go run -tags manual ./test/giznet-e2e/corktest \
  -mode trickle \
  -duration 2h \
  -streams 4 \
  -chunk 1024 \
  -write-interval 10s \
  -cork-every 3 \
  -cork 2s \
  -read-timeout 45s \
  -report 1m \
  -pprof-addr 127.0.0.1:6060 \
  -force-gc-interval 30m \
  -mem-csv /tmp/giznet-corktest-mem.csv
```

`-force-gc-interval` records post-GC low-watermark samples. Use
`-free-os-memory-interval` for heavier probes that also ask the runtime to
return idle spans to the OS.

Inspect pprof while it runs:

```sh
go tool pprof -top http://127.0.0.1:6060/debug/pprof/heap
go tool pprof -top 'http://127.0.0.1:6060/debug/pprof/heap?gc=1'
go tool pprof -top http://127.0.0.1:6060/debug/pprof/allocs
go tool pprof -http :0 http://127.0.0.1:6060/debug/pprof/heap
```

Disconnect cleanup probe:

```sh
go run -tags manual ./test/giznet-e2e/corktest \
  -mode disconnect \
  -streams 2 \
  -disconnect-wait 10s
```

By default, `disconnect` fails only if the server-side listener still owns a
peer `Conn` handle or server stream reads do not unblock. It prints the lower
level UDP peer state for diagnostics. Add `-require-offline` when you also want
the low-level peer state to become `offline` or disappear within
`-disconnect-wait`.
