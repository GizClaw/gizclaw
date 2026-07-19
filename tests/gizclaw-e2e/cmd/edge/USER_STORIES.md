# Edge CLI

## User Story

As an edge operator, I want the real `gizclaw edge` command to expose its
supported ingress lifecycle and reject incomplete startup requests without
leaving a process behind.

## Covered Behaviors

- Top-level and `serve` help are available from the built binary.
- `edge serve` rejects a missing workspace directory before starting a service.
- The long-running successful ingress lifecycle remains owned by the Docker
  setup phase, which starts and health-checks the edge container under a phase
  deadline and removes it during gate cleanup.
