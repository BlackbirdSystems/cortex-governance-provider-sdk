import json
import time

import pytest
from fastapi.testclient import TestClient

from governance_sdk import (
    ApprovalPreviewInput,
    ApprovalPreviewOutput,
    ApprovalPreviewRow,
    DynamicProviderApp,
)
from governance_sdk.models import (
    DYNAMIC_PROVIDER_KEY_HEADER,
    DYNAMIC_PROVIDER_SIGNATURE_HEADER,
    DYNAMIC_PROVIDER_TIMESTAMP_HEADER,
)
from governance_sdk.signing import sign_request

SECRET = "test-secret"
PATH = "/v1/provider/approval-preview"


def _unused(*_args, **_kwargs):
    raise AssertionError("not part of this test")


def make_app(approval_preview=None) -> DynamicProviderApp:
    return DynamicProviderApp(
        provider_name="sample",
        display_name="Sample",
        description="sample provider",
        map_request=_unused,
        resolve_resource=_unused,
        map_scope=_unused,
        parse_settings=_unused,
        compute_binding=_unused,
        fetch_download=_unused,
        action_to_tool=lambda action: action,
        approval_preview=approval_preview,
    )


def post(app: DynamicProviderApp, payload: dict):
    body = json.dumps(payload).encode()
    timestamp = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    headers = {
        DYNAMIC_PROVIDER_TIMESTAMP_HEADER: timestamp,
        DYNAMIC_PROVIDER_KEY_HEADER: "sample",
        DYNAMIC_PROVIDER_SIGNATURE_HEADER: sign_request("POST", PATH, timestamp, body, SECRET.encode()),
        "Content-Type": "application/json",
    }
    return TestClient(app.fastapi).post(PATH, content=body, headers=headers)


@pytest.fixture(autouse=True)
def secret(monkeypatch):
    monkeypatch.setenv("GOVERNANCE_SECRET", SECRET)
    monkeypatch.delenv("GOVERNANCE_DYNAMIC_PROVIDER_SECRET", raising=False)


REQUEST = {
    "context": {"authorization": "Bearer t", "secrets": {"access_token": "tok"}},
    "request": {"provider": "sample", "action": "send", "resource_id": "r1", "params": {"message_id": "m1"}},
}


def test_rows_round_trip_and_the_call_context_is_available():
    seen = {}

    def preview(payload: ApprovalPreviewInput) -> ApprovalPreviewOutput:
        seen["message_id"] = payload.request.params["message_id"]
        seen["token"] = app.access_token("access_token")
        return ApprovalPreviewOutput(
            rows=[ApprovalPreviewRow(label="Replying to Ann", value="hi", replaces="/params/message_id")]
        )

    app = make_app(preview)
    response = post(app, REQUEST)

    assert response.status_code == 200, response.text
    assert response.json()["rows"] == [
        {"label": "Replying to Ann", "value": "hi", "format": "", "replaces": "/params/message_id"}
    ]
    assert seen == {"message_id": "m1", "token": "tok"}


def test_no_callback_answers_404_so_the_host_knows_there_is_nothing_to_add():
    assert post(make_app(), REQUEST).status_code == 404


def test_a_callback_error_is_a_failure_not_an_empty_result():
    def preview(_payload: ApprovalPreviewInput) -> ApprovalPreviewOutput:
        raise RuntimeError("upstream 403")

    client_app = make_app(preview)
    with pytest.raises(RuntimeError):
        post(client_app, REQUEST)
