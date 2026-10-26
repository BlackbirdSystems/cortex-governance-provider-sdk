from __future__ import annotations

from typing import Any, Literal
from urllib.parse import urlparse

from pydantic import BaseModel, Field, model_validator


PROTOCOL_VERSION = "v1"
DYNAMIC_PROVIDER_SIGNATURE_HEADER = "X-Cortex-Dynamic-Provider-Signature"
DYNAMIC_PROVIDER_TIMESTAMP_HEADER = "X-Cortex-Dynamic-Provider-Timestamp"
DYNAMIC_PROVIDER_KEY_HEADER = "X-Cortex-Dynamic-Provider-Key"

# Accepted values for BindingInputSchema.type. "" is free text, no validation.
# Mirrors cortex-governance/go/governance's BindingType* constants. Being a
# Literal (rather than plain str) gives provider authors static type-checking
# (mypy/pyright) against these values, plus Pydantic rejects anything else
# at construction time.
# ``multiselect`` is deprecated for new provider-backed choices. Use ``picker``
# with ``PickerConfig(multiple=True)``; the legacy type remains wire-compatible.
BindingType = Literal["", "bool", "number", "select", "multiselect", "picker", "secret"]

# Source of a secret whose value is the tenant's OAuth access token. Such an
# input references its OAuth client in BindingInputSchema.oauth.
INPUT_SOURCE_OAUTH = "oauth"

# Consent URL parameters the host sets itself; extra ones may not override them.
_RESERVED_AUTHORIZATION_PARAMS = frozenset(
    {"response_type", "client_id", "redirect_uri", "state", "code_challenge", "code_challenge_method", "scope"}
)


class TenantPolicy(BaseModel):
    tenant_id: str = ""
    subject: str = ""


class ProviderOAuthConfig(BaseModel):
    """The OAuth 2.0 client a provider declares for an OAuth-backed secret
    (authorization code + PKCE). The endpoints and scopes ship with the
    provider; the tenant's own client credentials are ordinary inputs of the
    same provider that this config points at by name, so each tenant uses its
    own OAuth app. Mirrors cortex-governance's ProviderOAuthConfig."""

    authorization_endpoint: str
    token_endpoint: str
    scopes: str = ""
    # How the client secret reaches the token endpoint: "basic" (default) or "post".
    token_auth_method: Literal["", "basic", "post"] = ""
    # Extra consent URL parameters some providers need before they issue a
    # refresh token (for example access_type=offline).
    authorization_params: dict[str, str] = Field(default_factory=dict)
    # Name of the input holding the tenant's OAuth client id.
    client_id_input: str
    # Optional name of the sensitive input holding the client secret.
    client_secret_input: str = ""


class OAuthNotConnectedError(LookupError):
    """The tenant has not connected an OAuth-backed secret, so the host
    delivered no token. Surface it so the admin is told to connect."""


class BindingInputSchema(BaseModel):
    name: str
    label: str = ""
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
    picker: "PickerConfig | None" = None
    # Makes this secret's value the tenant's OAuth access token instead of a
    # pasted setting. Requires source == INPUT_SOURCE_OAUTH and type == "secret";
    # build one with oauth_secret_input().
    oauth: ProviderOAuthConfig | None = None

    def is_sensitive(self) -> bool:
        return self.sensitive or self.hash or self.type == "secret"


def _oauth_endpoint_ok(raw: str) -> bool:
    parsed = urlparse(raw)
    if not parsed.hostname or parsed.username or parsed.password:
        return False
    if parsed.scheme == "https":
        return True
    return parsed.scheme == "http" and parsed.hostname in ("localhost", "127.0.0.1", "::1")


def oauth_secret_input(
    name: str,
    config: ProviderOAuthConfig,
    *,
    label: str = "",
    description: str = "",
) -> BindingInputSchema:
    """Declare a secret whose value is the tenant's OAuth access token."""
    return BindingInputSchema(
        name=name,
        label=label,
        description=description,
        source=INPUT_SOURCE_OAUTH,
        type="secret",
        sensitive=True,
        oauth=config,
    )


class PickerConfig(BaseModel):
    multiple: bool = False
    depends_on: list[str] = Field(default_factory=list)


class ProviderCapability(BaseModel):
    name: str
    display_name: str = ""
    description: str = ""
    tools: list[str] = Field(default_factory=list)


class BindingSchema(BaseModel):
    inputs: list[BindingInputSchema] = Field(default_factory=list)

    @model_validator(mode="after")
    def _validate_oauth(self) -> "BindingSchema":
        """Apply the rules the host applies when it loads the provider, so a
        malformed OAuth declaration fails when the schema is built (provider
        start-up or tests) instead of when the host refuses to load it."""
        by_name = {i.name: i for i in self.inputs}
        for i in self.inputs:
            cfg = i.oauth
            if cfg is None:
                if i.source.strip() == INPUT_SOURCE_OAUTH:
                    raise ValueError(f"input {i.name!r}: source {INPUT_SOURCE_OAUTH!r} needs an oauth block")
                continue
            if i.source.strip() != INPUT_SOURCE_OAUTH or i.type != "secret":
                raise ValueError(f"input {i.name!r}: an oauth block needs source {INPUT_SOURCE_OAUTH!r} and type 'secret'")
            if not (_oauth_endpoint_ok(cfg.authorization_endpoint) and _oauth_endpoint_ok(cfg.token_endpoint)):
                raise ValueError(f"input {i.name!r}: oauth authorization_endpoint and token_endpoint must be https URLs")
            for key in cfg.authorization_params:
                if not key or key in _RESERVED_AUTHORIZATION_PARAMS:
                    raise ValueError(f"input {i.name!r}: authorization_params may not set {key!r}")
            client_id = by_name.get(cfg.client_id_input)
            if client_id is None or client_id.oauth is not None:
                raise ValueError(f"input {i.name!r}: oauth client_id_input {cfg.client_id_input!r} must name another input")
            if cfg.client_secret_input:
                secret = by_name.get(cfg.client_secret_input)
                if secret is None or secret.oauth is not None or not secret.is_sensitive():
                    raise ValueError(f"input {i.name!r}: oauth client_secret_input {cfg.client_secret_input!r} must name a sensitive input")
        return self


class DynamicProviderContext(BaseModel):
    authorization: str = ""
    policy: TenantPolicy | None = None
    # Decrypted sensitive inputs for this call, keyed by input name, including
    # the current access token of any OAuth-backed secret.
    secrets: dict[str, str] = Field(default_factory=dict)

    def access_token(self, input_name: str) -> str:
        """The OAuth access token the host delivered for the named OAuth-backed
        secret. Raises OAuthNotConnectedError when there is none."""
        token = self.secrets.get(input_name, "").strip()
        if not token:
            raise OAuthNotConnectedError(f"oauth is not connected for this tenant: {input_name}")
        return token

class PickerItem(BaseModel):
    value: str
    label: str
    description: str = ""
    data: Any = None
class PickerValidation(BaseModel):
    valid: bool
    message: str = ""
class PickerInput(BaseModel):
    context: DynamicProviderContext = Field(default_factory=DynamicProviderContext)
    operation: Literal["list", "validate"]
    input_name: str
    target_tenant_id: str = ""
    dependencies: dict[str, str] = Field(default_factory=dict)
    query: str = ""
    cursor: str = ""
    limit: int = 0
    selected_values: list[str] = Field(default_factory=list)
class PickerOutput(BaseModel):
    items: list[PickerItem] = Field(default_factory=list)
    next_cursor: str = ""
    validation: PickerValidation | None = None


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
    # When True, the host materializes this download and stages it into the shared
    # tenant file store, returning the file provider's governed URLs instead of a
    # per-provider delegated-download ticket. Prefer this for generated content so
    # it lands in the same store as every other provider's output.
    stage_to_file_store: bool = False
    # Optional relative destination within the tenant file store when
    # stage_to_file_store is set. Leave empty to use a conventional .staging path.
    path: str = ""


class FetchDownloadInput(BaseModel):
    context: DynamicProviderContext = Field(default_factory=DynamicProviderContext)
    download: DelegatedDownload


class FetchDownloadOutput(BaseModel):
    content_base64: str
    content_type: str = ""
    filename: str = ""
