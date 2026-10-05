# Service

A Go web service created from the platform's `go-app-template`. It builds and deploys itself: you change the code, push to `main`, and the platform does the rest.

## What is here

| Path | What it is |
|---|---|
| `app/` | The Go service: source, tests, and a `Dockerfile`. See [`app/README.md`](app/README.md) |
| `.github/workflows/build-and-push.yml` | CI: vets and tests the code, builds the image, pushes it to GitHub Container Registry, and records the new image tag in `environments/dev/values-dev.yaml` |
| `environments/dev/values-dev.yaml` | Deployment settings for the dev environment. **CI writes the image repository and tag here.** |

## How a change reaches the cluster

1. Push a change under `app/` to `main`.
2. GitHub Actions vets, tests, builds, and pushes an image tagged `sha-<short commit>`.
3. The workflow commits that tag to `environments/dev/values-dev.yaml` in this repository.
4. The platform's Argo CD Application reads that file, so Argo CD rolls out the new image. Argo CD polls Git about every three minutes, so allow a short delay. Nothing in this repository talks to the cluster.

The commit CI makes only touches `environments/`, which is outside the workflow's path filter, so it does not trigger another build.

## What not to edit by hand

- `environments/dev/values-dev.yaml`: CI overwrites the image fields on every build. Other settings in it (such as `replicaCount`) are yours to change.
- The Helm chart, the Argo CD Application, and any database for this service belong to the platform and live in the platform repository: https://github.com/rcarlson-devops/self-service-idp
