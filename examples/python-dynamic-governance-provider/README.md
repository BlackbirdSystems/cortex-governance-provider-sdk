# Example Python Dynamic Governance Provider

This is a thin reference provider for the dynamic governance `HTTP+JSON` over `UDS` contract.
Internally it uses `FastMCP` for tool behavior and the reusable SDK package in `python/governance_sdk` for the governance transport and FastAPI integration.

## Files

- `provider.py`: runnable provider server
- `provider.json`: discovery manifest to mount beside the socket
- `requirements.txt`: Python dependencies for the example

The example exposes both:

- `example_python_echo`: normal proxied tool execution
- `example_python_get_echo_download_url`: delegated governed download that returns an agent-owned URL and serves content later via `fetch-download`

That split is intentional:

- `FastMCP` is the contributor-friendly tool layer
- `python/governance_sdk` is the reusable governance adapter layer
- `FastAPI` is the fixed governance proxy surface
- the dynamic provider adapter in the agent only talks to the FastAPI endpoints over the mounted Unix socket

It also demonstrates resource-scoped grant synthesis in `parse-settings`:

- provider settings include `example_python_resources`
- each workspace entry can mint one `workspace` grant and multiple `document` grants
- request mapping uses `workspace_id` + `document_id` so the API request/response flow lines up with the generated grants

## Install

```bash
cd cortex-governance-provider-sdk
python3 -m venv .venv-dynamic-provider
. .venv-dynamic-provider/bin/activate
pip install -r examples/python-dynamic-governance-provider/requirements.txt
pip install -e ./python
```

Set the shared HMAC secret used by the proxy and provider:

```bash
export GOVERNANCE_DYNAMIC_PROVIDER_SECRET=replace-me
```

`FastMCP` guidance:

- tool schemas are generated from Python signatures and docstrings
- `/v1/provider/tools` adapts the registered `FastMCP` tool metadata into the governance provider contract
- `/v1/provider/execute` delegates execution to the registered `FastMCP` tool instead of maintaining a second manual dispatch table

## Run Locally

```bash
cd cortex-governance-provider-sdk
source .venv-dynamic-provider/bin/activate
python3 examples/python-dynamic-governance-provider/provider.py \
  --socket /var/run/cortex-governance/providers/example-python/provider.sock
```

## Mount Layout

Mount the provider directory into the agent container so the manifest and socket are both visible under `GOVERNANCE_PROVIDER_DIR`:

```text
/var/run/cortex-governance/providers/
  example-python/
    provider.json
    provider.sock
```

The agent will discover `provider.json`, validate that `provider.sock` is inside the mounted root, and proxy requests to the Python process over the Unix socket.

If your provider returns more than one binding attribute from `/v1/provider/compute-binding`, prefer the structured envelope form so each attribute carries its own signature.

## Example Resource Settings

The example expects provider settings like:

```json
{
  "example_python_resources": [
    {
      "workspace_id": "workspace-alpha",
      "document_ids": ["doc-001", "doc-002"]
    }
  ]
}
```

From that, `parse-settings` returns:

- one `workspace/workspace-alpha` grant
- one `document/doc-001` grant
- one `document/doc-002` grant
