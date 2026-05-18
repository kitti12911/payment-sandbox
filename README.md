# payment-sandbox

Background worker service scaffolded by
[`service-gen`](https://github.com/kitti12911/service-gen).

## What's included

- NATS / JetStream consumer using `lib-async` with typed JSON job decoding.
- At-least-once delivery with ack / nack handling.
- Configuration via `config.yml` / environment variables (`lib-util/v3`).
- Structured logging, OpenTelemetry tracing, and Pyroscope profiling, with
  trace propagation from publisher messages to the worker handler.
- A `internal/worker` package with an example job payload and handler.
- GitHub Actions and/or GitLab CI, Renovate, CODEOWNERS, golangci-lint,
  markdownlint, Prettier, `.air.toml`, and a multi-stage `Dockerfile`.

## Getting started

```sh
cp config.example.yml config.yml   # then edit values, including the NATS topic
make run
```

A local NATS broker is required at runtime.

## Message shape

```json
{
    "id": "job-1",
    "type": "debug.print",
    "payload": { "message": "hello" }
}
```

## Delivery behavior

With JetStream enabled the worker uses at-least-once delivery: a handler error
nacks the message and the broker can redeliver it, so handlers should be
idempotent.

## Common commands

| Command       | Description                               |
| ------------- | ----------------------------------------- |
| `make run`    | Start the worker locally                  |
| `make air`    | Run with live reload                      |
| `make test`   | Run tests with the race detector          |
| `make lint`   | Run Go and Markdown linting               |
| `make format` | Format Go, Markdown, YAML, JSON           |
| `make cov`    | Generate and open an HTML coverage report |

Implement your job logic in `internal/worker/handler.go`.

## Deployment

A starter Helm chart is included in [`helm/`](helm/) so the worker is
deployable out of the box. Deployment state should not live in the service
repository long-term — move the chart to your `helm-sandbox` repository (or
your preferred chart repository), then delete `helm/` from here:

```sh
mv helm /path/to/helm-sandbox/charts/payment-sandbox
```

Keep app-specific values (image, env, scaling, config) in the chart
repository, not in the service repo.
