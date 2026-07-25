"""Credential-backed Mem0 OSS service for the manual GizClaw E2E stack.

The HTTP surface follows the standard self-hosted Mem0 entity contract. GizClaw
encodes its complete provider-neutral Scope into the native user_id before the
request reaches this service. Extraction, embedding, persistence, and semantic
search remain real Mem0 operations backed by OpenAI and embedded Qdrant.
"""

from __future__ import annotations

import os
import threading
from contextlib import asynccontextmanager
from typing import Any

from fastapi import FastAPI, HTTPException, Response
from mem0 import Memory
from dotenv import dotenv_values
from pydantic import BaseModel, ConfigDict, Field

_ROUTING_FIELDS = ("user_id", "agent_id", "run_id")
_CREDENTIAL_FILE = "/run/gizclaw-e2e.env"
_memory: Memory | None = None
_memory_lock = threading.RLock()


class Message(BaseModel):
    role: str
    content: str


class MemoryCreate(BaseModel):
    model_config = ConfigDict(extra="forbid")

    messages: list[Message]
    metadata: dict[str, Any] | None = None
    infer: bool = True
    user_id: str | None = None
    agent_id: str | None = None
    run_id: str | None = None


class MemoryUpdate(BaseModel):
    text: str


class SearchRequest(BaseModel):
    query: str = Field(min_length=1)
    filters: dict[str, Any]
    top_k: int = Field(default=10, ge=1, le=1000)


def _routing_kwargs(values: dict[str, Any]) -> dict[str, str]:
    routing: dict[str, str] = {}
    for field in _ROUTING_FIELDS:
        raw = values.get(field)
        if raw is None:
            continue
        if not isinstance(raw, str):
            raise ValueError(f"{field} must be a string")
        value = raw.strip()
        if not value:
            continue
        if value == "*":
            raise ValueError(f"{field} wildcard is unsupported")
        routing[field] = value
    if not routing:
        raise ValueError("at least one Mem0 entity field is required")
    return routing


def _result_entries(value: Any) -> list[dict[str, Any]]:
    if isinstance(value, dict):
        value = value.get("results", [])
    if not isinstance(value, list):
        raise ValueError("Mem0 returned an invalid result envelope")
    if not all(isinstance(entry, dict) for entry in value):
        raise ValueError("Mem0 returned an invalid result entry")
    return value


def _get_memory() -> Memory:
    if _memory is None:
        raise RuntimeError("Mem0 is not initialized")
    return _memory


def _build_memory() -> Memory:
    api_key = os.environ.get("OPENAI_API_KEY", "").strip()
    if not api_key:
        api_key = str(
            dotenv_values(_CREDENTIAL_FILE).get(
                "GIZCLAW_E2E_OPENAI_API_KEY", ""
            )
        ).strip()
    if not api_key:
        raise RuntimeError("GIZCLAW_E2E_OPENAI_API_KEY is required")
    return Memory.from_config(
        {
            "version": "v1.1",
            "vector_store": {
                "provider": "qdrant",
                "config": {
                    "collection_name": "gizclaw_e2e",
                    "path": "/tmp/gizclaw-e2e-qdrant",
                    "on_disk": True,
                },
            },
            "llm": {
                "provider": "openai",
                "config": {
                    "api_key": api_key,
                    "model": "gpt-4o-mini",
                    "temperature": 0.1,
                },
            },
            "embedder": {
                "provider": "openai",
                "config": {
                    "api_key": api_key,
                    "model": "text-embedding-3-small",
                },
            },
            "history_db_path": "/tmp/gizclaw-e2e-memory-history.db",
        }
    )


@asynccontextmanager
async def _lifespan(_: FastAPI):
    global _memory
    _memory = _build_memory()
    yield
    _memory = None


app = FastAPI(title="GizClaw Mem0 E2E", lifespan=_lifespan)


@app.get("/health")
def health() -> dict[str, str]:
    _get_memory()
    return {"status": "ready"}


@app.post("/memories")
def add_memory(request: MemoryCreate) -> dict[str, Any]:
    try:
        routing = _routing_kwargs(request.model_dump())
        with _memory_lock:
            result = _get_memory().add(
                messages=[message.model_dump() for message in request.messages],
                metadata=request.metadata,
                infer=request.infer,
                **routing,
            )
        return {"results": _result_entries(result)}
    except ValueError as error:
        raise HTTPException(status_code=400, detail=str(error)) from error


@app.post("/search")
def search_memories(request: SearchRequest) -> dict[str, Any]:
    try:
        with _memory_lock:
            result = _get_memory().search(
                query=request.query,
                filters=request.filters,
                top_k=request.top_k,
            )
        return {"results": _result_entries(result)}
    except ValueError as error:
        raise HTTPException(status_code=400, detail=str(error)) from error


@app.get("/memories/{memory_id}")
def get_memory(memory_id: str) -> dict[str, Any]:
    try:
        with _memory_lock:
            result = _get_memory().get(memory_id)
        if not isinstance(result, dict):
            raise ValueError("Mem0 returned an invalid memory")
        return result
    except ValueError as error:
        raise HTTPException(status_code=400, detail=str(error)) from error


@app.put("/memories/{memory_id}")
def update_memory(memory_id: str, request: MemoryUpdate) -> dict[str, Any]:
    try:
        with _memory_lock:
            _get_memory().update(memory_id=memory_id, data=request.text)
            result = _get_memory().get(memory_id)
        if not isinstance(result, dict):
            raise ValueError("Mem0 returned an invalid memory")
        return {"results": [result]}
    except ValueError as error:
        raise HTTPException(status_code=400, detail=str(error)) from error


@app.delete("/memories/{memory_id}", status_code=204)
def delete_memory(memory_id: str) -> Response:
    with _memory_lock:
        _get_memory().delete(memory_id=memory_id)
    return Response(status_code=204)
