from __future__ import annotations

import contextvars
import inspect
import os
import time
from pathlib import Path
from typing import Any, Awaitable, Callable

import uvicorn
from fastapi import FastAPI, HTTPException, Request, Response
from fastmcp import FastMCP

from .models import (
    DYNAMIC_PROVIDER_KEY_HEADER,
    DYNAMIC_PROVIDER_SIGNATURE_HEADER,
    DYNAMIC_PROVIDER_TIMESTAMP_HEADER,
    ComputeBindingInput,
    ComputeBindingOutput,
    DynamicProviderContext,
    ExecuteInput,
    ExecuteOutput,
    FetchDownloadInput,
    FetchDownloadOutput,
    IsAvailableInput,
    MapRequestInput,
    MapRequestOutput,
    MapScopeInput,
    MapScopeOutput,
    PROTOCOL_VERSION,
    ParseSettingsInput,
    ParseSettingsOutput,
    ResolveResourceInput,
    ResolveResourceOutput,
)
from .signing import secret_from_env, sign_response, verify_request


class DynamicProviderApp:
    def __init__(
        self,
        provider_name: str,
        display_name: str,
        description: str,
        map_request: Callable[[MapRequestInput], MapRequestOutput],
        resolve_resource: Callable[[ResolveResourceInput], ResolveResourceOutput],
        map_scope: Callable[[MapScopeInput], MapScopeOutput],
        parse_settings: Callable[[ParseSettingsInput], ParseSettingsOutput],
        compute_binding: Callable[[ComputeBindingInput], ComputeBindingOutput],
        fetch_download: Callable[[FetchDownloadInput], FetchDownloadOutput],
        action_to_tool: Callable[[str], str],
        is_available: Callable[[IsAvailableInput], bool] | None = None,
    ):
        self.provider_name = provider_name
        self.display_name = display_name
        self.description = description
        self.map_request = map_request
        self.resolve_resource = resolve_resource
        self.map_scope = map_scope
        self.parse_settings = parse_settings
        self.compute_binding = compute_binding
        self.fetch_download = fetch_download
        self.action_to_tool = action_to_tool
        self.is_available = is_available or (lambda _: True)
        self.current_request_context: contextvars.ContextVar[DynamicProviderContext] = contextvars.ContextVar(
            "current_request_context",
            default=DynamicProviderContext(),
        )
        self.mcp = FastMCP(display_name)
        self.fastapi = FastAPI(title=display_name, version="1.0.0")
        self._register_routes()

    def _register_routes(self) -> None:
        @self.fastapi.middleware("http")
        async def sign_and_verify(request: Request, call_next):
            request_body = await request.body()
            self.verify_dynamic_provider_request(request, request_body)
            response = await call_next(request)
            response_body = b""
            async for chunk in response.body_iterator:
                response_body += chunk
            timestamp = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
            signed_response = Response(
                content=response_body,
                status_code=response.status_code,
                headers=dict(response.headers),
                media_type=response.media_type,
            )
            signed_response.headers[DYNAMIC_PROVIDER_TIMESTAMP_HEADER] = timestamp
            signed_response.headers[DYNAMIC_PROVIDER_KEY_HEADER] = self.provider_name
            signed_response.headers[DYNAMIC_PROVIDER_SIGNATURE_HEADER] = sign_response(
                request.method,
                request.url.path,
                timestamp,
                response.status_code,
                response_body,
                secret_from_env(),
            )
            return signed_response

        @self.fastapi.get("/healthz", response_model=dict[str, str])
        def healthz() -> dict[str, str]:
            return {"status": "ok"}

        @self.fastapi.get("/v1/provider/meta", response_model=dict[str, str])
        def provider_meta() -> dict[str, str]:
            return {
                "name": self.provider_name,
                "protocol_version": PROTOCOL_VERSION,
                "display_name": self.display_name,
                "description": self.description,
            }

        @self.fastapi.get("/v1/provider/tools", response_model=dict[str, list[dict[str, Any]]])
        async def provider_tools() -> dict[str, list[dict[str, Any]]]:
            tools = []
            for tool in await self.list_fastmcp_tools():
                tools.append(
                    {
                        "name": tool.name,
                        "description": tool.description,
                        "inputSchema": self.fastmcp_tool_schema(tool),
                    }
                )
            return {"tools": tools}

        @self.fastapi.post("/v1/provider/map-request", response_model=MapRequestOutput)
        def provider_map_request(payload: MapRequestInput) -> MapRequestOutput:
            return self.map_request(payload)

        @self.fastapi.post("/v1/provider/resolve-resource", response_model=ResolveResourceOutput)
        def provider_resolve_resource(payload: ResolveResourceInput) -> ResolveResourceOutput:
            return self.resolve_resource(payload)

        @self.fastapi.post("/v1/provider/map-scope", response_model=MapScopeOutput)
        def provider_map_scope(payload: MapScopeInput) -> MapScopeOutput:
            return self.map_scope(payload)

        @self.fastapi.post("/v1/provider/execute", response_model=ExecuteOutput)
        async def provider_execute(payload: ExecuteInput) -> ExecuteOutput:
            token = self.current_request_context.set(payload.context)
            try:
                return ExecuteOutput(
                    result=await self.call_fastmcp_tool(
                        self.action_to_tool(payload.request.action),
                        payload.request.params,
                    )
                )
            finally:
                self.current_request_context.reset(token)

        @self.fastapi.post("/v1/provider/parse-settings", response_model=ParseSettingsOutput)
        def provider_parse_settings(payload: ParseSettingsInput) -> ParseSettingsOutput:
            return self.parse_settings(payload)

        @self.fastapi.post("/v1/provider/compute-binding", response_model=ComputeBindingOutput)
        def provider_compute_binding(payload: ComputeBindingInput) -> ComputeBindingOutput:
            return self.compute_binding(payload)

        @self.fastapi.post("/v1/provider/is-available", response_model=dict[str, bool])
        def provider_is_available(payload: IsAvailableInput) -> dict[str, bool]:
            return {"ok": self.is_available(payload)}

        @self.fastapi.post("/v1/provider/fetch-download", response_model=FetchDownloadOutput)
        def provider_fetch_download(payload: FetchDownloadInput) -> FetchDownloadOutput:
            return self.fetch_download(payload)

    def tool(self, fn: Callable[..., Any]) -> Callable[..., Any]:
        return self.mcp.tool(fn)

    def policy(self):
        return self.current_request_context.get().policy

    def context(self) -> DynamicProviderContext:
        return self.current_request_context.get()

    def verify_dynamic_provider_request(self, request: Request, body: bytes) -> None:
        if request.url.path == "/healthz":
            return
        timestamp = request.headers.get(DYNAMIC_PROVIDER_TIMESTAMP_HEADER, "").strip()
        signature = request.headers.get(DYNAMIC_PROVIDER_SIGNATURE_HEADER, "").strip()
        provider_key = request.headers.get(DYNAMIC_PROVIDER_KEY_HEADER, "").strip()
        if not timestamp or not signature or provider_key != self.provider_name:
            raise HTTPException(status_code=401, detail="missing HMAC headers")
        if not verify_request(request.method, request.url.path, timestamp, body, signature, secret_from_env()):
            raise HTTPException(status_code=401, detail="invalid HMAC signature")

    async def maybe_await(self, value: Any) -> Any:
        if inspect.isawaitable(value):
            return await value
        return value

    async def list_fastmcp_tools(self) -> list[Any]:
        return list(await self.maybe_await(self.mcp.list_tools()))

    async def call_fastmcp_tool(self, name: str, arguments: dict[str, Any]) -> dict[str, Any]:
        result = await self.maybe_await(self.mcp.call_tool(name, arguments))
        if hasattr(result, "data"):
            return result.data
        if hasattr(result, "structured_content"):
            return result.structured_content
        if isinstance(result, dict):
            return result
        return {"result": result}

    def fastmcp_tool_schema(self, tool: Any) -> dict[str, Any]:
        schema = getattr(tool, "inputSchema", None)
        if schema is None:
            schema = getattr(tool, "input_schema", None)
        if schema is None:
            return {"type": "object", "properties": {}}
        return schema

    def run(self, socket_path: str, log_level: str = "info") -> None:
        socket = Path(socket_path)
        socket.parent.mkdir(parents=True, exist_ok=True)
        try:
            socket.unlink()
        except FileNotFoundError:
            pass

        @self.fastapi.on_event("startup")
        def set_socket_permissions() -> None:
            if socket.exists():
                os.chmod(socket, 0o666)

        config = uvicorn.Config(app=self.fastapi, uds=str(socket), log_level=log_level)
        uvicorn.Server(config).run()
