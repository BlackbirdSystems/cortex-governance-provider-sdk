import pytest
from pydantic import ValidationError

from governance_sdk import (
    BindingInputSchema,
    BindingSchema,
    DynamicProviderContext,
    OAuthNotConnectedError,
    ProviderOAuthConfig,
    oauth_secret_input,
)


def qb_inputs() -> list[BindingInputSchema]:
    return [
        BindingInputSchema(name="client_id", source="settings", label="Client ID"),
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
            description="Connect your QuickBooks company",
        ),
    ]


def test_oauth_secret_input_is_a_sensitive_oauth_sourced_secret():
    token = qb_inputs()[2]
    assert (token.source, token.type, token.sensitive, token.label) == ("oauth", "secret", True, "QuickBooks")
    assert token.oauth is not None


def test_declaration_matches_the_host_wire_format():
    wire = BindingSchema(inputs=qb_inputs()).model_dump(mode="json", exclude_defaults=True)["inputs"][2]
    assert wire["source"] == "oauth" and wire["type"] == "secret"
    assert wire["oauth"]["client_id_input"] == "client_id"
    assert wire["oauth"]["client_secret_input"] == "client_secret"
    assert wire["oauth"]["token_endpoint"].startswith("https://")


def test_valid_schema_is_accepted_and_plain_schemas_are_unaffected():
    BindingSchema(inputs=qb_inputs())
    BindingSchema(inputs=[BindingInputSchema(name="api_key", source="settings", type="secret")])


@pytest.mark.parametrize(
    "mutate",
    [
        lambda i: setattr(i[2], "type", ""),
        lambda i: setattr(i[2], "source", "settings"),
        lambda i: setattr(i[2], "oauth", None),
        lambda i: setattr(i[2].oauth, "token_endpoint", "http://a.example/token"),
        lambda i: setattr(i[2].oauth, "client_id_input", "nope"),
        lambda i: setattr(i[0], "oauth", i[2].oauth),
        lambda i: setattr(i[1], "type", ""),
        lambda i: setattr(i[2].oauth, "authorization_params", {"state": "x"}),
    ],
)
def test_malformed_declarations_fail_when_the_schema_is_built(mutate):
    inputs = qb_inputs()
    mutate(inputs)
    with pytest.raises(ValidationError):
        BindingSchema.model_validate({"inputs": [i.model_dump() for i in inputs]})


def test_bad_token_auth_method_is_rejected():
    with pytest.raises(ValidationError):
        ProviderOAuthConfig(
            authorization_endpoint="https://a.example/authorize",
            token_endpoint="https://a.example/token",
            token_auth_method="jwt",
            client_id_input="client_id",
        )


def test_access_token_reads_the_delivered_secret():
    ctx = DynamicProviderContext.model_validate({"secrets": {"access_token": " tok "}})
    assert ctx.access_token("access_token") == "tok"


@pytest.mark.parametrize("payload", [{}, {"secrets": {"access_token": "  "}}])
def test_missing_token_raises_not_connected(payload):
    with pytest.raises(OAuthNotConnectedError):
        DynamicProviderContext.model_validate(payload).access_token("access_token")


def test_secrets_decode_from_the_host_payload():
    ctx = DynamicProviderContext.model_validate({"authorization": "Bearer x", "secrets": {"access_token": "at-1"}})
    assert ctx.access_token("access_token") == "at-1"


def test_a_tool_reads_the_token_through_the_app_during_an_execute_call():
    import asyncio

    from governance_sdk import DynamicProviderApp

    app = DynamicProviderApp(
        provider_name="qb",
        display_name="QuickBooks",
        description="",
        map_request=lambda _: None,
        resolve_resource=lambda _: None,
        map_scope=lambda _: None,
        parse_settings=lambda _: None,
        compute_binding=lambda _: None,
        fetch_download=lambda _: None,
        action_to_tool=lambda action: action,
        binding_schema=BindingSchema(inputs=qb_inputs()),
    )

    @app.tool
    def whoami() -> dict:
        return {"token": app.access_token("access_token")}

    # provider_execute sets the request-scoped context to the host's payload
    # context before dispatching the tool; do the same here.
    reset = app.current_request_context.set(DynamicProviderContext.model_validate({"secrets": {"access_token": "at-9"}}))
    try:
        assert asyncio.run(app.call_fastmcp_tool("whoami", {})) == {"token": "at-9"}
    finally:
        app.current_request_context.reset(reset)

    # With no secret delivered the same call is a clear not-connected error.
    with pytest.raises(Exception, match="not connected"):
        asyncio.run(app.call_fastmcp_tool("whoami", {}))


def test_oauth_param_reads_the_delivered_callback_value():
    ctx = DynamicProviderContext.model_validate({"oauth_params": {"access_token": {"realmId": " 9341 "}}})
    assert ctx.oauth_param("access_token", "realmId") == "9341"
    assert ctx.oauth_param("access_token", "other") == ""
    assert DynamicProviderContext().oauth_param("access_token", "realmId") == ""
