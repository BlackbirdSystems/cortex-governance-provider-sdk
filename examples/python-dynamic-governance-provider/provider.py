#!/usr/bin/env python3
"""Reference FastMCP-backed provider built on the governance SDK Python library."""

from __future__ import annotations

import argparse
import base64
import time
from typing import Any

from pydantic import BaseModel, Field

from governance_sdk import (
    CapabilityGrant,
    ComputeBindingOutput,
    DynamicProviderApp,
    FetchDownloadOutput,
    MapRequestInput,
    MapRequestOutput,
    MapScopeOutput,
    ParseSettingsOutput,
    ProtectedResource,
    ProviderRequest,
    ResolveResourceOutput,
)


PROVIDER_NAME = "example-python"
DISPLAY_NAME = "Example Python Dynamic Provider"
DESCRIPTION = "Reference Python HTTP-over-UDS governance provider"


class ExampleResourceBinding(BaseModel):
    workspace_id: str
    document_ids: list[str] = Field(default_factory=list)

def map_request(payload: MapRequestInput) -> MapRequestOutput:
    workspace_id = str(payload.arguments.get("workspace_id") or "workspace-demo")
    document_id = str(payload.arguments.get("document_id") or "document-demo")
    return MapRequestOutput(
        request=ProviderRequest(
            provider=PROVIDER_NAME,
            action="get_echo_download" if payload.tool_name == "example_python_get_echo_download_url" else "echo",
            resource_id=f"{workspace_id}:{document_id}",
            params=payload.arguments,
        )
    )


def resolve_resource(payload) -> ResolveResourceOutput:
    _, _, document_id = payload.request.resource_id.partition(":")
    return ResolveResourceOutput(
        resource=ProtectedResource(
            provider=PROVIDER_NAME,
            resource_type="document",
            resource_id=document_id or payload.request.resource_id,
        )
    )


def map_scope(payload) -> MapScopeOutput:
    return MapScopeOutput(scope=f"invoke_{payload.action}")


def parse_settings(payload) -> ParseSettingsOutput:
    bindings = [
        ExampleResourceBinding(**binding)
        for binding in payload.settings.get("example_python_resources", [])
        if isinstance(binding, dict)
    ]
    grants: list[CapabilityGrant] = []
    for binding in bindings:
        grants.append(
            CapabilityGrant(
                tenant_id=payload.tenant_id,
                provider=PROVIDER_NAME,
                resource_type="workspace",
                resource_id=binding.workspace_id,
                scopes=["invoke_echo", "invoke_get_echo_download"],
                expires_at=int(time.time()) + 86400,
            )
        )
        for document_id in binding.document_ids:
            grants.append(
                CapabilityGrant(
                    tenant_id=payload.tenant_id,
                    provider=PROVIDER_NAME,
                    resource_type="document",
                    resource_id=document_id,
                    scopes=["invoke_echo", "invoke_get_echo_download"],
                    expires_at=int(time.time()) + 86400,
                )
            )
    if not grants:
        grants.append(
            CapabilityGrant(
                tenant_id=payload.tenant_id,
                provider=PROVIDER_NAME,
                resource_type="workspace",
                resource_id="workspace-demo",
                scopes=["invoke_echo"],
                expires_at=int(time.time()) + 3600,
            )
        )
    return ParseSettingsOutput(grants=grants)


def compute_binding(payload) -> ComputeBindingOutput:
    binding_attributes = {
        key: str(value)
        for key, value in payload.params.items()
        if key != "tenant_id" and not key.endswith("_sig")
    }
    if len(binding_attributes) > 1:
        return ComputeBindingOutput(
            envelope={
                "bindings": {
                    key: {
                        "value": value,
                        "signature": f"example-python-binding-{key}",
                    }
                    for key, value in binding_attributes.items()
                }
            }
        )
    return ComputeBindingOutput(
        binding={key: "example-python-binding" for key in payload.params if key.endswith("_sig")}
    )


def fetch_download(payload) -> FetchDownloadOutput:
    message = str(payload.download.params.get("message") or "")
    return FetchDownloadOutput(
        content_base64=base64.b64encode(message.encode("utf-8")).decode("utf-8"),
        content_type="text/plain; charset=utf-8",
        filename="example-python-echo.txt",
    )


provider = DynamicProviderApp(
    provider_name=PROVIDER_NAME,
    display_name=DISPLAY_NAME,
    description=DESCRIPTION,
    map_request=map_request,
    resolve_resource=resolve_resource,
    map_scope=map_scope,
    parse_settings=parse_settings,
    compute_binding=compute_binding,
    fetch_download=fetch_download,
    action_to_tool=lambda action: "example_python_get_echo_download_url" if action == "get_echo_download" else "example_python_echo",
)


@provider.tool
def example_python_echo(workspace_id: str, document_id: str, message: str) -> dict[str, Any]:
    """Echo a message for a specific governed document resource."""
    del workspace_id, document_id
    context = provider.context()
    policy = provider.policy()
    return {
        "provider": PROVIDER_NAME,
        "message": message,
        "tenant_id": policy.tenant_id if policy else "",
        "policy_subject": policy.subject if policy else "",
        "authorization": context.authorization,
    }


@provider.tool
def example_python_get_echo_download_url(workspace_id: str, document_id: str, message: str) -> dict[str, Any]:
    """Return an agent-owned governed download URL delegated back to this provider."""
    del workspace_id, document_id
    return {
        "delegated_download": {
            "resource_id": "example-python-echo-download",
            "filename": "example-python-echo.txt",
            "content_type": "text/plain; charset=utf-8",
            "expires_in_seconds": 300,
            "params": {"message": message},
            "next_step": "Use downloadUrl to fetch the provider-generated text file.",
        }
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=DESCRIPTION)
    parser.add_argument(
        "--socket",
        default="/var/run/cortex-governance/providers/example-python/provider.sock",
        help="Unix socket path",
    )
    args = parser.parse_args()
    provider.run(args.socket)


if __name__ == "__main__":
    main()
