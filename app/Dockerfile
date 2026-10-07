# --- Build stage ---
FROM golang:1.25-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
COPY *.go ./
RUN go vet ./...
RUN go test ./... -v
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${VERSION}" -o /out/app .

# --- Runtime stage ---
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/app .
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/app"]
