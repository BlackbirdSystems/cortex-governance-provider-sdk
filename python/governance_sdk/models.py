from __future__ import annotations

from typing import Any, Literal

from pydantic import BaseModel, Field


PROTOCOL_VERSION = "v1"
DYNAMIC_PROVIDER_SIGNATURE_HEADER = "X-Cortex-Dynamic-Provider-Signature"
DYNAMIC_PROVIDER_TIMESTAMP_HEADER = "X-Cortex-Dynamic-Provider-Timestamp"
DYNAMIC_PROVIDER_KEY_HEADER = "X-Cortex-Dynamic-Provider-Key"

# Accepted values for BindingInputSchema.type. "" is free text, no validation.
# Mirrors cortex-governance/go/governance's BindingType* constants. Being a
# Literal (rather than plain str) gives provider authors static type-checking
# (mypy/pyright) against these values, plus Pydantic rejects anything else
# at construction time.
BindingType = Literal["", "bool", "number", "select", "multiselect"]


class TenantPolicy(BaseModel):
    tenant_id: str = ""
    subject: str = ""


class BindingInputSchema(BaseModel):
    name: str
    description: str = ""
    source: str
    claim: str = ""
    setting_key: str = ""
    required: bool = False
    sensitive: bool = False
    hash: bool = False
    signature_key: str = ""
    type: BindingType = ""
    options: list[str] = Field(default_factory=list)


class BindingSchema(BaseModel):
    inputs: list[BindingInputSchema] = Field(default_factory=list)


class DynamicProviderContext(BaseModel):
    authorization: str = ""
    policy: TenantPolicy | None = None


class ProviderRequest(BaseModel):
    provider: str
    action: str
    resource_id: str
    params: dict[str, Any] = Field(default_factory=dict)


class ProtectedResource(BaseModel):
    provider: str
    resource_type: str
    resource_id: str


class CapabilityGrant(BaseModel):
    tenant_id: str
    provider: str
    resource_type: str
    resource_id: str
    scopes: list[str]
    expires_at: int
    signature: str = ""


class MapRequestInput(BaseModel):
    tool_name: str
    arguments: dict[str, Any] = Field(default_factory=dict)


class MapRequestOutput(BaseModel):
    request: ProviderRequest


class ResolveResourceInput(BaseModel):
    context: DynamicProviderContext = Field(default_factory=DynamicProviderContext)
    request: ProviderRequest


class ResolveResourceOutput(BaseModel):
    resource: ProtectedResource


class MapScopeInput(BaseModel):
    action: str


class MapScopeOutput(BaseModel):
    scope: str


class ExecuteInput(BaseModel):
    context: DynamicProviderContext = Field(default_factory=DynamicProviderContext)
    request: ProviderRequest


class ExecuteOutput(BaseModel):
    result: dict[str, Any]


class ParseSettingsInput(BaseModel):
    tenant_id: str
    token: str = ""
    settings: dict[str, Any] = Field(default_factory=dict)


class ParseSettingsOutput(BaseModel):
    grants: list[CapabilityGrant]


class ComputeBindingInput(BaseModel):
    secret: str = ""
    params: dict[str, Any] = Field(default_factory=dict)


class ComputeBindingOutput(BaseModel):
    binding: dict[str, str] = Field(default_factory=dict)
    envelope: dict[str, Any] | None = None


class IsAvailableInput(BaseModel):
    context: DynamicProviderContext = Field(default_factory=DynamicProviderContext)
    tenant_id: str = ""


class DelegatedDownload(BaseModel):
    resource_id: str
    filename: str = ""
    content_type: str = ""
    expires_in_seconds: int = 0
    params: dict[str, Any] = Field(default_factory=dict)
    next_step: str = ""


class FetchDownloadInput(BaseModel):
    context: DynamicProviderContext = Field(default_factory=DynamicProviderContext)
    download: DelegatedDownload


class FetchDownloadOutput(BaseModel):
    content_base64: str
    content_type: str = ""
    filename: str = ""
