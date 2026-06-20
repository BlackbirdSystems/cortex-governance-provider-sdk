from __future__ import annotations

import hashlib
import hmac
import os

from .models import PROTOCOL_VERSION


def secret_from_env() -> bytes:
    secret = os.getenv("GOVERNANCE_DYNAMIC_PROVIDER_SECRET") or os.getenv("GOVERNANCE_SECRET")
    if not secret:
        raise RuntimeError("set GOVERNANCE_DYNAMIC_PROVIDER_SECRET or GOVERNANCE_SECRET")
    return secret.encode("utf-8")


def sign_request(method: str, path: str, timestamp: str, body: bytes, secret: bytes) -> str:
    digest = hashlib.sha256(body).hexdigest()
    payload = "\n".join(
        [
            "cortex-dynamic-provider-request",
            PROTOCOL_VERSION,
            method.strip().upper(),
            path.strip(),
            timestamp.strip(),
            digest,
        ]
    )
    return hmac.new(secret, payload.encode("utf-8"), hashlib.sha256).hexdigest()


def sign_response(method: str, path: str, timestamp: str, status_code: int, body: bytes, secret: bytes) -> str:
    digest = hashlib.sha256(body).hexdigest()
    payload = "\n".join(
        [
            "cortex-dynamic-provider-response",
            PROTOCOL_VERSION,
            method.strip().upper(),
            path.strip(),
            str(status_code),
            timestamp.strip(),
            digest,
        ]
    )
    return hmac.new(secret, payload.encode("utf-8"), hashlib.sha256).hexdigest()


def verify_request(method: str, path: str, timestamp: str, body: bytes, signature: str, secret: bytes) -> bool:
    return hmac.compare_digest(signature, sign_request(method, path, timestamp, body, secret))
