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

## OAuth-backed secrets

A provider that talks to an OAuth 2.0 API does not have to ask admins to paste an
access token, or store and refresh one itself. It declares an OAuth-backed
secret: a `secret` input whose `source` is `oauth` and that references its OAuth
client. The host then runs the authorization flow when an admin connects the
tenant, stores the tokens, refreshes them (including in the background), and
delivers the current access token with every call in `context.secrets`, under the
input's name.

The tenant's own OAuth client id and secret are ordinary inputs of the provider,
so each tenant uses its own OAuth app. The `oauth` block points at them by name.

Go:

```go
schema := &dynamicprovider.BindingSchema{Inputs: []dynamicprovider.BindingInputSchema{
	{Name: "client_id", Source: "settings", Label: "Client ID", Required: true},
	{Name: "client_secret", Source: "settings", Type: dynamicprovider.BindingTypeSecret},
	dynamicprovider.OAuthSecretInput("access_token", "QuickBooks", "Connect your company", dynamicprovider.ProviderOAuthConfig{
		AuthorizationEndpoint: "https://appcenter.intuit.com/connect/oauth2",
		TokenEndpoint:         "https://oauth.platform.intuit.com/oauth2/v1/tokens/bearer",
		Scopes:                "com.intuit.quickbooks.accounting",
		ClientIDInput:         "client_id",
		ClientSecretInput:     "client_secret",
	}),
}}

// In Execute:
token, err := input.Context.AccessToken("access_token") // ErrOAuthNotConnected if none
```

Python:

```python
from governance_sdk import BindingInputSchema, BindingSchema, ProviderOAuthConfig, oauth_secret_input

schema = BindingSchema(inputs=[
    BindingInputSchema(name="client_id", source="settings", label="Client ID", required=True),
    BindingInputSchema(name="client_secret", source="settings", type="secret"),
    oauth_secret_input(
        "access_token",
        ProviderOAuthConfig(
            authorization_endpoint="https://appcenter.intuit.com/connect/oauth2",
            token_endpoint="https://oauth.platform.intuit.com/oauth2/v1/tokens/bearer",
            scopes="com.intuit.quickbooks.accounting",
            client_id_input="client_id",
            client_secret_input="client_secret",
        ),
        label="QuickBooks",
    ),
])

# Inside a tool:
token = app.access_token("access_token")  # raises OAuthNotConnectedError if none
```

Notes:

- The declaration is validated with the host's rules when the schema is built
  (`NewServer` panics, the Python `BindingSchema` raises), so a mistake shows up
  at start-up or in your tests, not when the host refuses to load the provider.
- Optional `AuthorizationParams` / `authorization_params` add consent URL
  parameters some providers need before they issue a refresh token (for example
  `access_type=offline`); protocol parameters cannot be overridden.
- Rotating the tenant's client secret takes effect when the tenant reconnects.
- Other sensitive inputs are delivered in the same `context.secrets`, decrypted.
- Requires a host with OAuth-backed secret support (cortex-governance-kernel
  with the `oauth_connections` store).

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

Use `ghcr.io/blackbirdsystems/cortex-governance-provider-base:latest` as the base image for runtime. It includes CA certificates, default `SOCKETS_DIR`, and a generic entrypoint that automatically sets up the socket directory, copies `provider.json`, and resolves `GOVERNANCE_PROVIDER_SOCKET`.

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o my-provider .

FROM ghcr.io/blackbirdsystems/cortex-governance-provider-base:latest
COPY --from=builder /build/my-provider /usr/local/bin/my-provider
COPY provider.json /etc/cortex-provider/provider.json
CMD ["my-provider"]
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
    networks:
      - cortex-agent-gateway

networks:
  cortex-agent-gateway:
    external: true

volumes:
  governance-provider-sockets:
    external: true
```
