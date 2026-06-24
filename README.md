# Cortex Governance Provider SDK

Reference SDK repository for building dynamic governance providers that connect to `cortex-agent` over HTTP+JSON on Unix domain sockets.

## Repository Layout

- `go/`: reusable Go library for dynamic governance provider transport, signing, and UDS serving
- `python/`: reusable Python package for FastAPI + FastMCP dynamic governance providers
- `examples/go-dynamic-provider`: thin Go example provider
- `examples/python-dynamic-governance-provider`: thin FastAPI + FastMCP example provider
- `examples/docker/dynamic-governance-provider.compose.yml`: shared-socket compose example

## Provider Model

Third-party developers should primarily think in terms of normal MCP tool implementation:

1. Define tools.
2. Add governance mapping for request, resource, scope, bindings, and grants.
3. Expose the fixed provider HTTP contract over a Unix socket.
4. Mount the provider directory into the agent container under `GOVERNANCE_PROVIDER_DIR`.

## Libraries

The Go library provides:

- protocol DTOs
- HMAC signing helpers
- UDS HTTP server bootstrap
- callback-driven provider endpoint wiring

The Python library provides:

- shared Pydantic models
- HMAC signing helpers
- a reusable `DynamicProviderApp` for FastAPI + FastMCP
- governed request-context bridging for tool execution
- live binding schema metadata in provider meta responses

## Examples

Both examples are intentionally thin and only implement provider-specific behavior:

- HMAC request verification and response signing
- delegated governed downloads
- resource grant synthesis
- multi-attribute binding envelopes
- binding schema declaration for admin discovery

## Docker Deployment

Production providers ship as Docker images and share a single named volume with the agent so that both can access the Unix domain socket.

### Directory convention

Both `provider.json` and `provider.sock` must live at:

```
<sockets_dir>/<provider-name>/provider.json
<sockets_dir>/<provider-name>/provider.sock
```

The agent reads `provider.json` for metadata and connects to `provider.sock` for all RPC calls.

### Dockerfile

Use a multi-stage build. Bake `provider.json` into the image and copy it into the socket directory at startup via an entrypoint script.

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
COPY cortex-governance-provider-sdk/ cortex-governance-provider-sdk/
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o my-provider .

FROM alpine:3
RUN apk --no-cache add ca-certificates
COPY --from=builder /build/my-provider /usr/local/bin/my-provider
COPY provider.json /etc/my-provider/provider.json
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV SOCKETS_DIR=/var/run/cortex-governance/providers
ENV GOVERNANCE_PROVIDER_SOCKET=/var/run/cortex-governance/providers/my-provider/provider.sock

ENTRYPOINT ["/entrypoint.sh"]
```

### entrypoint.sh

```sh
#!/bin/sh
set -e
mkdir -p "$SOCKETS_DIR/my-provider"
cp /etc/my-provider/provider.json "$SOCKETS_DIR/my-provider/provider.json"
exec my-provider "$@"
```

### Docker Compose

See `examples/docker/provider-pod.compose.yml` for a complete example. The key points:

- The `governance-provider-sockets` volume is declared `external: true` — it must be created and owned by the Atlas deployment; Compose will not create or destroy it.
- Both the agent and every provider mount the same volume so the socket is visible to all.
- `GOVERNANCE_PROVIDER_DIR` on the agent must match the mount path (`/var/run/cortex-governance/providers` above).

```yaml
services:
  cortex-agent:
    image: ghcr.io/blackbirdsystems/cortex-agent-cell:latest
    environment:
      GOVERNANCE_DYNAMIC_PROVIDERS_ENABLED: "true"
      GOVERNANCE_PROVIDER_DIR: /var/run/cortex-governance/providers
    volumes:
      - governance-provider-sockets:/var/run/cortex-governance/providers

  my-provider:
    image: my-provider:latest
    environment:
      GOVERNANCE_DYNAMIC_PROVIDER_SECRET: ${GOVERNANCE_DYNAMIC_PROVIDER_SECRET}
      SOCKETS_DIR: /var/run/cortex-governance/providers
      GOVERNANCE_PROVIDER_SOCKET: /var/run/cortex-governance/providers/my-provider/provider.sock
    volumes:
      - governance-provider-sockets:/var/run/cortex-governance/providers

volumes:
  governance-provider-sockets:
    external: true
```
