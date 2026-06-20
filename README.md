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

## Examples

Both examples are intentionally thin and only implement provider-specific behavior:

- HMAC request verification and response signing
- delegated governed downloads
- resource grant synthesis
- multi-attribute binding envelopes
