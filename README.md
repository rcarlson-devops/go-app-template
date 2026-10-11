# Service app

The Go source for this service. CI builds it into a container image, and the platform deploys it to Kubernetes through Argo CD. Day to day, this directory is the part you change.

It is built the way a platform-hosted service should be: structured JSON logging, configuration that fails fast on bad values, separate liveness and readiness checks, and a graceful shutdown so rolling deploys don't drop requests.

## Layout

| Path | Purpose |
|---|---|
| `main.go` | Startup, signal handling, and graceful shutdown |
| `config.go` | Reads and validates configuration from environment variables |
| `server.go` | HTTP handlers, request logging, readiness state |
| `db.go` | The database check behind `/db` (one connection per call, using `pgx`) |
| `*_test.go` | Tests |
| `Dockerfile` | Multi-stage build; the final image runs `/app/app` as a non-root user |
| `go.mod`, `go.sum` | Go module `github.com/rcarlson-devops/app` (Go 1.25) and its dependencies. Commit both: the Docker build copies them |

## Endpoints

| Path | Response |
|---|---|
| `GET /` | JSON with a message, hostname, version, environment, and timestamp |
| `GET /version` | JSON with `version` and `environment` |
| `GET /healthz` | Liveness: `ok` with status 200 while the process is alive |
| `GET /readyz` | Readiness: `ok` normally; `503` from the moment shutdown begins |
| `GET /internal/prestop` | Used by Kubernetes' `preStop` hook only; waits `PRESTOP_DELAY_SECONDS` and returns 200 |
| `GET /db` | Database check. `{"configured":false}` (200) with no database settings; `{"configured":true,"connected":true,"database":"<name>"}` (200) on success; `{"configured":true,"connected":false,"error":"connection failed"}` (503) on failure. The real error is logged, not returned. Unauthenticated, and it opens a new connection on every call |

Requests are logged as one JSON line each. Probe and `preStop` requests are not logged.

## Configuration

All settings are environment variables, and all are optional. The four `DB_*` variables are optional as a group.

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | Port the server listens on |
| `ENVIRONMENT` | `unknown` | Must be `dev`, `staging`, or `prod` if set; any other value stops the app at startup |
| `SHUTDOWN_TIMEOUT_SECONDS` | `10` | Longest time in-flight requests get to finish during shutdown; must be a positive integer |
| `PRESTOP_DELAY_SECONDS` | `0` | How long `/internal/prestop` waits before responding; `0` means no wait; must be a non-negative integer |
| `DB_HOST` | empty | Database host. The chart sets the full in-cluster name, `postgres-db-rw.<service>-db.svc.cluster.local`. The port is fixed at 5432 |
| `DB_USER` | empty | Database user. The chart reads it from the Secret `db-credentials` |
| `DB_PASSWORD` | empty | Database password, from the same Secret |
| `DB_NAME` | empty | Database name, from the same Secret |

An invalid value makes the app exit immediately with an error message rather than start with settings nobody intended. In the platform, the Helm chart sets all eight variables for each deployment.

**Database settings.** All four empty is fine (local runs, tests): `/db` reports `configured: false`. Setting some but not all stops the app at startup with an error that names the missing variables and never prints a value. The app does not connect at startup, so it starts even if the database does not exist yet, and `/readyz` never depends on the database. The connection URL sets no `sslmode`, so I believe the driver tries TLS and falls back to plain without verifying the certificate; this has not been checked.

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

Try other settings with `ENVIRONMENT=dev PORT=9090 go run .`. Press Ctrl+C to see the shutdown messages. `curl localhost:8080/db` returns `{"configured":false}` unless you set all four `DB_*` variables.

## Run the tests

```sh
cd app
go test ./... -cover
```

The tests cover configuration loading (including the database settings all set, none set, and partly set, where the password must not leak into the error), each handler, `/db` with no database, a working database and a failing one (the error must not leak), readiness staying healthy when the database is down, the readiness change during shutdown, which requests are logged, the `preStop` delay, and a full-server check over a real listener. The tests use a fake database check; none connects to a real Postgres.

## Build and run the container

```sh
cd app
docker build -t service:local .
docker run --rm -p 8080:8080 service:local
```

The build runs `go vet` and `go test` first, so a failing test stops `docker build`, not just CI. The base image is `golang:1.25-alpine`, because the `pgx` dependency needs Go 1.25 or newer. The CI workflow's `setup-go` step still says 1.22; align it with `go.mod` (for example `go-version-file: app/go.mod`).

## How it gets deployed

1. A push to `main` that changes `app/` (or the build workflow) triggers the workflow in `.github/workflows/`.
2. The workflow vets, tests, builds, and pushes an image to GitHub Container Registry tagged `sha-<short commit>`.
3. The workflow then commits the image repository and tag to `environments/dev/values-dev.yaml` in this repository.
4. The platform's Argo CD Application reads that file and rolls the new image out. Argo CD polls about every three minutes, so allow a short delay.

CI never talks to the cluster directly.

## What not to edit by hand

- `environments/dev/values-dev.yaml`: CI overwrites the image fields on every build.
- Deployment settings (the Helm chart, the Argo CD Application, the database) belong to the platform, not this repository. See the platform repo: https://github.com/rcarlson-devops/self-service-idp
