# Service app

The Go source for this service. CI builds it into a container image, and the platform deploys it to Kubernetes through Argo CD. Day to day, this directory is the part you change.

It is built the way a platform-hosted service should be: structured JSON logging, configuration that fails fast on bad values, separate liveness and readiness checks, and a graceful shutdown so rolling deploys don't drop requests.

## Layout

| Path | Purpose |
|---|---|
| `main.go` | Startup, signal handling, and graceful shutdown |
| `config.go` | Reads and validates configuration from environment variables |
| `server.go` | HTTP handlers, request logging, readiness state |
| `*_test.go` | Tests |
| `Dockerfile` | Multi-stage build; the final image runs `/app/app` as a non-root user |
| `go.mod` | Go module `github.com/rcarlson-devops/app` |

## Endpoints

| Path | Response |
|---|---|
| `GET /` | JSON with a message, hostname, version, environment, and timestamp |
| `GET /version` | JSON with `version` and `environment` |
| `GET /healthz` | Liveness: `ok` with status 200 while the process is alive |
| `GET /readyz` | Readiness: `ok` normally; `503` from the moment shutdown begins |
| `GET /internal/prestop` | Used by Kubernetes' `preStop` hook only; waits `PRESTOP_DELAY_SECONDS` and returns 200 |

Requests are logged as one JSON line each. Probe and `preStop` requests are not logged.

## Configuration

All settings are environment variables, and all are optional.

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | Port the server listens on |
| `ENVIRONMENT` | `unknown` | Must be `dev`, `staging`, or `prod` if set; any other value stops the app at startup |
| `SHUTDOWN_TIMEOUT_SECONDS` | `10` | Longest time in-flight requests get to finish during shutdown; must be a positive integer |
| `PRESTOP_DELAY_SECONDS` | `0` | How long `/internal/prestop` waits before responding; `0` means no wait; must be a non-negative integer |

An invalid value makes the app exit immediately with an error message rather than start with settings nobody intended. In the platform, the Helm chart sets these four variables for each deployment.

## Graceful shutdown

When Kubernetes stops a pod, it first calls `/internal/prestop`, then sends `SIGTERM`. On `SIGTERM` the app switches `/readyz` to `503`, stops accepting new connections, and waits up to `SHUTDOWN_TIMEOUT_SECONDS` for in-flight requests to finish before exiting. The final image has no shell, which is why the `preStop` hook is an HTTP call to the app itself rather than a `sleep` command.

## Run locally

```sh
cd app
go run .
```

In another terminal:

```sh
curl localhost:8080/
curl localhost:8080/version
curl localhost:8080/readyz
```

Try other settings with `ENVIRONMENT=dev PORT=9090 go run .`. Press Ctrl+C to see the shutdown messages.

## Run the tests

```sh
cd app
go test ./... -cover
```

The tests cover configuration loading, each handler, the readiness change during shutdown, which requests are logged, the `preStop` delay, and a full-server check over a real listener.

## Build and run the container

```sh
cd app
docker build -t service:local .
docker run --rm -p 8080:8080 service:local
```

The build runs `go vet` and `go test` first, so a failing test stops `docker build`, not just CI.

## How it gets deployed

1. A push to `main` that changes `app/` (or the build workflow) triggers the workflow in `.github/workflows/`.
2. The workflow vets, tests, builds, and pushes an image to GitHub Container Registry tagged `sha-<short commit>`.
3. The workflow then commits the image repository and tag to `environments/dev/values-dev.yaml` in this repository.
4. The platform's Argo CD Application reads that file and rolls the new image out. Argo CD polls about every three minutes, so allow a short delay.

CI never talks to the cluster directly.

## What not to edit by hand

- `environments/dev/values-dev.yaml`: CI overwrites the image fields on every build.
- Deployment settings (the Helm chart, the Argo CD Application, the database) belong to the platform, not this repository. See the platform repo: https://github.com/rcarlson-devops/self-service-idp
