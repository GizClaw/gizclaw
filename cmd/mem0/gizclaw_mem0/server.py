"""GizClaw HTTP service backed by the official Mem0 OSS SDK.

The HTTP surface follows the standard self-hosted Mem0 entity contract. GizClaw
encodes its complete provider-neutral Scope into the native user_id before the
request reaches this service. Extraction, embedding, persistence, and semantic
search remain real Mem0 operations backed by configured model services and
embedded Qdrant or PostgreSQL/pgvector.
"""

from __future__ import annotations

import json
import hashlib
import math
import hmac
import re
import os
import threading
import time
from contextlib import ExitStack, asynccontextmanager, contextmanager
from importlib.metadata import version
from typing import Any

from fastapi import FastAPI, HTTPException, Query, Response
from fastapi.responses import JSONResponse
import yaml
from mem0 import Memory
from mem0.exceptions import LLMError
from openai import APIError, APIStatusError
from mem0.llms.openai import OpenAILLM
from mem0.embeddings.openai import OpenAIEmbedding
from dotenv import dotenv_values
from pydantic import BaseModel, ConfigDict, Field
from gizclaw_mem0.schema import contract
import anyio.to_thread
import anyio

_ROUTING_FIELDS = ("user_id", "agent_id", "run_id")
_CREDENTIAL_FILE = os.environ.get("MEM0_CREDENTIAL_FILE", "")
_memory: Memory | None = None
_memory_lock = threading.RLock()
_operation_state = threading.local()
_scope_guard = threading.Lock()
_scope_locks: dict[tuple, list] = {}
_diagnostic_lock = threading.Lock()
_service_api_key = ""
_request_limiter = None
_read_limiter = None


class CompatibleOpenAILLM(OpenAILLM):
    """Keep GPT-6 Luna requests compatible with the pinned Mem0 SDK."""

    def __init__(self, config, service_tier=""):
        super().__init__(config)
        self.service_tier = service_tier

    def generate_response(self, *args, **kwargs):
        _operation_state.llm_started = time.perf_counter()
        response = super().generate_response(*args, **kwargs)
        # Mem0 swallows exceptions from response_callback. Validate here before
        # its add pipeline can embed or persist the extracted candidates.
        if getattr(_operation_state, "extraction_required", False):
            _validate_extraction_response()
        return response

    def _get_supported_params(self, **kwargs: Any) -> dict[str, Any]:
        params = super()._get_supported_params(**kwargs)
        if self.service_tier:
            params["service_tier"] = self.service_tier
        if not _is_luna(self.config.model):
            return params
        params.pop("max_tokens", None)
        params["max_completion_tokens"] = self.config.max_tokens
        effort = self.config.reasoning_effort or "medium"
        params["reasoning_effort"] = effort
        if effort != "none":
            params.pop("temperature", None)
            params.pop("top_p", None)
        return params


def _is_luna(model: str) -> bool:
    return model == "gpt-6-luna" or model.startswith("gpt-6-luna-")


class ThinkingOpenAILLM(CompatibleOpenAILLM):
    """Forward an explicit thinking mode to OpenAI-compatible model APIs."""

    def __init__(self, config: Any, thinking: str, service_tier=""):
        super().__init__(config, service_tier)
        self.thinking = thinking

    def _get_supported_params(self, **kwargs: Any) -> dict[str, Any]:
        params = super()._get_supported_params(**kwargs)
        params.setdefault("extra_body", {})["thinking"] = {"type": self.thinking}
        return params


class ArkMultimodalEmbedding(OpenAIEmbedding):
    """Embed each memory/query as one text sample using Ark's multimodal API."""

    protocol = "ark_multimodal"
    corpus_instructions = "Instruction:Compress the text into one word.\nQuery:"
    query_instructions = (
        "Target_modality: text.\n"
        "Instruction:Retrieve personal memory facts that answer the question, preserving people, dates and negation.\n"
        "Query:"
    )

    def __init__(self, config: Any):
        if config.embedding_dims not in (1024, 2048):
            raise RuntimeError("Ark multimodal embedding dimensions must be 1024 or 2048")
        super().__init__(config)
        self._errors = threading.local()

    @property
    def last_error(self) -> str | None:
        return getattr(self._errors, "value", None)

    @last_error.setter
    def last_error(self, value: str | None):
        self._errors.value = value

    def embed(self, text: str, memory_action: str | None = None) -> list[float]:
        try:
            if not isinstance(text, str) or not text.strip():
                raise RuntimeError("Ark embedding requires nonempty text")
            response = self.client.post(
                "/embeddings/multimodal",
                cast_to=dict,
                body={
                    "model": self.config.model,
                    "input": [{"type": "text", "text": text}],
                    "dimensions": self.config.embedding_dims,
                    "encoding_format": "float",
                    "instructions": self.query_instructions if memory_action == "search" else self.corpus_instructions,
                },
            )
            vector = response.get("data", {}).get("embedding")
            if not isinstance(vector, list) or len(vector) != self.config.embedding_dims or not all(
                isinstance(value, (int, float)) and not isinstance(value, bool) and math.isfinite(value)
                for value in vector
            ):
                raise RuntimeError("Ark returned an invalid embedding vector")
            return [float(value) for value in vector]
        except Exception as error:
            self.last_error = type(error).__name__
            raise

    def embed_batch(self, texts: list[str], memory_action: str = "add") -> list[list[float]]:
        # input[] is one multimodal sample, not a batch of independent texts.
        return [self.embed(text, memory_action) for text in texts]

    def policy_fingerprint(self) -> str:
        return hashlib.sha256((self.corpus_instructions + "\x00" + self.query_instructions).encode()).hexdigest()


class Message(BaseModel):
    role: str
    content: str
    name: str | None = None


class MemoryCreate(BaseModel):
    model_config = ConfigDict(extra="forbid")

    messages: list[Message] = Field(min_length=1)
    metadata: dict[str, Any] | None = None
    infer: bool = True
    prompt: str | None = None
    user_id: str | None = None
    agent_id: str | None = None
    run_id: str | None = None


class MemoryUpdate(BaseModel):
    text: str = Field(min_length=1)


class SearchRequest(BaseModel):
    query: str = Field(min_length=1)
    filters: dict[str, Any]
    top_k: int = Field(default=10, ge=1, le=1000)


class MemoryRecord(BaseModel):
    model_config = ConfigDict(extra="allow")

    id: str
    memory: str | None = None
    score: float | None = None
    event: str | None = None
    metadata: dict[str, Any] | None = None
    user_id: str | None = None
    agent_id: str | None = None
    run_id: str | None = None
    created_at: str | None = None
    updated_at: str | None = None


class MemoryResults(BaseModel):
    results: list[MemoryRecord]


class StatusResponse(BaseModel):
    message: str


class HealthStatus(BaseModel):
    status: str
    vector_store: str
    llm_max_tokens: int
    llm_model: str
    llm_provider: str
    llm_thinking: str
    llm_service_tier: str
    sdk_version: str
    extraction_policy_fingerprint: str
    embedding_model: str
    embedding_dimensions: int
    embedding_protocol: str
    embedding_policy_fingerprint: str


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


@contextmanager
def _write_lock(routing: dict[str, str]):
    # Local Qdrant shares mutable in-process vector state. PostgreSQL isolates
    # writes by native entity; independent Workspace/Peer scopes can overlap.
    if _get_memory().config.vector_store.provider != "pgvector":
        with _memory_lock:
            yield
        return
    # A broader purge must share locks with every matching compound entity.
    # Acquire in one order so requests with multiple entities cannot deadlock.
    keys = sorted(routing.items())
    with _scope_guard:
        entries = [_scope_locks.setdefault(key, [threading.RLock(), 0]) for key in keys]
        for entry in entries:
            entry[1] += 1
    try:
        with ExitStack() as held:
            for entry in entries:
                held.enter_context(entry[0])
            yield
    finally:
        with _scope_guard:
            for key, entry in zip(keys, entries):
                entry[1] -= 1
                if entry[1] == 0:
                    del _scope_locks[key]


@contextmanager
def _read_lock():
    if _get_memory().config.vector_store.provider == "pgvector":
        yield
    else:
        with _memory_lock:
            yield


def _validate_extraction_response():
    extraction = getattr(_operation_state, "extraction_response", None)
    if extraction is None or not extraction["valid_json"] or extraction["finish_reason"] != "stop":
        raise HTTPException(status_code=502, detail="Mem0 extraction failed or returned incomplete/invalid memory JSON")


def _report_llm_response(_: Any, response: Any, __: Any) -> None:
    choice = response.choices[0]
    content = choice.message.content or ""
    try:
        decoded = json.loads(content)
        memories = decoded.get("memory")
        valid_json = isinstance(memories, list) and all(
            isinstance(m, dict) and isinstance(m.get("text"), str) and m["text"].strip()
            for m in memories
        )
        candidates = len(memories) if isinstance(memories, list) else None
    except (ValueError, TypeError, AttributeError):
        candidates = None
        valid_json = False
    metadata = {
        "event": "mem0_extraction_response",
        "finish_reason": choice.finish_reason,
        "content_characters": len(content),
        "completion_tokens": response.usage.completion_tokens if response.usage else None,
        "prompt_tokens": getattr(response.usage, "prompt_tokens", None) if response.usage else None,
        "valid_json": valid_json,
        "candidates": candidates,
        "llm_duration_ms": (time.perf_counter()-getattr(_operation_state,"llm_started",time.perf_counter()))*1000,
        "service_tier": getattr(response,"service_tier","") or "",
    }
    _operation_state.extraction_response = metadata
    with _diagnostic_lock:
        print(json.dumps(metadata), flush=True)


def _build_memory() -> Memory:
    global _service_api_key
    _operation_state.configured_concurrency = 160
    _service_api_key = os.environ.get("MEM0_API_KEY", "")
    config_file = os.environ.get("MEM0_CONFIG", "").strip()
    if config_file:
        with open(config_file, encoding="utf-8") as source:
            document = yaml.safe_load(source)
        if not isinstance(document, dict) or set(document) - {"memory", "service"}:
            raise RuntimeError("Mem0 config requires memory and optional service objects")
        document = _expand_environment(document)
        config = document.get("memory")
        service = document.get("service", {})
        if not isinstance(config, dict) or not isinstance(service, dict) or set(service) - {"api_key", "thinking", "embedding_protocol", "max_concurrency"}:
            raise RuntimeError("Invalid Mem0 memory/service configuration")
        _service_api_key = service.get("api_key", _service_api_key)
        if not isinstance(_service_api_key, str):
            raise RuntimeError("Mem0 service api_key must be a string")
        _operation_state.configured_concurrency = _validate_concurrency(service.get("max_concurrency", 160))
        config.setdefault("version", "v1.1")
        config.setdefault("history_db_path", os.environ.get("MEM0_HISTORY_DB_PATH", "/tmp/gizclaw-memory-history.db"))
        return _configured_memory(config, service.get("thinking", ""), service.get("embedding_protocol", "openai"))
    _operation_state.configured_concurrency = _validate_concurrency(int(os.environ.get("MEM0_MAX_CONCURRENCY", "160")))
    shared_api_key = os.environ.get("OPENAI_API_KEY", "").strip()
    if not shared_api_key and os.path.isfile(_CREDENTIAL_FILE):
        shared_api_key = str(
            dotenv_values(_CREDENTIAL_FILE).get(
                "GIZCLAW_E2E_OPENAI_API_KEY", ""
            )
        ).strip()
    llm_api_key = os.environ.get("MEM0_LLM_API_KEY", "").strip() or shared_api_key
    embedding_api_key = (
        os.environ.get("MEM0_EMBEDDING_API_KEY", "").strip()
        or shared_api_key
    )
    if not llm_api_key:
        raise RuntimeError(
            "MEM0_LLM_API_KEY, OPENAI_API_KEY, or a configured credential file is required"
        )
    if not embedding_api_key:
        raise RuntimeError(
            "MEM0_EMBEDDING_API_KEY, OPENAI_API_KEY, or a configured credential file is required"
        )
    llm_model = os.environ.get("MEM0_LLM_MODEL", "").strip() or "gpt-4o-mini"
    embedding_model = (
        os.environ.get("MEM0_EMBEDDING_MODEL", "").strip()
        or "text-embedding-3-small"
    )
    raw_embedding_dimensions = os.environ.get(
        "MEM0_EMBEDDING_DIMENSIONS", "1536"
    ).strip()
    try:
        embedding_dimensions = int(raw_embedding_dimensions)
    except ValueError as error:
        raise RuntimeError(
            "MEM0_EMBEDDING_DIMENSIONS must be a positive integer"
        ) from error
    if embedding_dimensions <= 0:
        raise RuntimeError(
            "MEM0_EMBEDDING_DIMENSIONS must be a positive integer"
        )
    try:
        max_tokens = int(os.environ.get("MEM0_LLM_MAX_TOKENS", "2000"))
    except ValueError as error:
        raise RuntimeError("MEM0_LLM_MAX_TOKENS must be a positive integer") from error
    if max_tokens <= 0:
        raise RuntimeError("MEM0_LLM_MAX_TOKENS must be a positive integer")
    thinking = os.environ.get("MEM0_LLM_THINKING", "").strip()
    if thinking not in ("", "enabled", "disabled"):
        raise RuntimeError("MEM0_LLM_THINKING must be enabled, disabled, or empty")
    embedding_protocol = os.environ.get("MEM0_EMBEDDING_PROTOCOL", "openai").strip()
    if embedding_protocol not in ("openai", "ark_multimodal"):
        raise RuntimeError("MEM0_EMBEDDING_PROTOCOL must be openai or ark_multimodal")
    if embedding_protocol == "ark_multimodal" and embedding_dimensions not in (1024, 2048):
        raise RuntimeError("Ark multimodal embedding dimensions must be 1024 or 2048")
    if embedding_protocol == "ark_multimodal" and not os.environ.get("MEM0_EMBEDDING_BASE_URL", "").strip():
        raise RuntimeError("MEM0_EMBEDDING_BASE_URL is required for ark_multimodal")
    config = (
        {
            "version": "v1.1",
            "vector_store": _vector_store_config(embedding_dimensions),
            "llm": {
                "provider": "openai",
                "config": {
                    "api_key": llm_api_key,
                    "model": llm_model,
                    "openai_base_url": os.environ.get(
                        "MEM0_LLM_BASE_URL", ""
                    ).strip()
                    or None,
                    "temperature": 0.1,
                    "max_tokens": max_tokens,
                    "response_callback": _report_llm_response,
                },
            },
            "embedder": {
                "provider": "openai",
                "config": {
                    "api_key": embedding_api_key,
                    "model": embedding_model,
                    "embedding_dims": embedding_dimensions,
                    "openai_base_url": os.environ.get(
                        "MEM0_EMBEDDING_BASE_URL", ""
                    ).strip()
                    or None,
                },
            },
            "history_db_path": os.environ.get("MEM0_HISTORY_DB_PATH", "/tmp/gizclaw-memory-history.db"),
        }
    )
    service_tier = os.environ.get("MEM0_LLM_SERVICE_TIER", "").strip()
    if service_tier:
        config["llm"]["config"]["service_tier"] = service_tier
    return _configured_memory(config, thinking, embedding_protocol)


def _expand_environment(value: Any) -> Any:
    if isinstance(value, dict):
        return {key: _expand_environment(item) for key, item in value.items()}
    if isinstance(value, list):
        return [_expand_environment(item) for item in value]
    if isinstance(value, str):
        def replace(match: re.Match) -> str:
            name = match.group(1)
            if not os.environ.get(name):
                raise RuntimeError("Missing Mem0 config environment variable: " + name)
            return os.environ[name]
        return re.sub(r"\$\{([A-Z_][A-Z0-9_]*)\}", replace, value)
    return value


def _configured_memory(config: dict[str, Any], thinking: str, embedding_protocol: str) -> Memory:
    if config.get("custom_instructions"):
        raise RuntimeError("Business instructions belong to MemoryLayout and request.prompt, not the service config")
    if thinking not in ("", "enabled", "disabled") or embedding_protocol not in ("openai", "ark_multimodal"):
        raise RuntimeError("Invalid Mem0 thinking or embedding protocol")
    for name in ("llm", "embedder", "vector_store"):
        provider = config.get(name)
        if not isinstance(provider, dict) or not isinstance(provider.get("config"), dict):
            raise RuntimeError("Mem0 requires " + name + " provider/config objects")
    if config["llm"].get("provider") != "openai" or config["embedder"].get("provider") != "openai":
        raise RuntimeError("This Mem0 service requires OpenAI-compatible LLM and embedder providers")
    if config["vector_store"].get("provider") not in ("qdrant", "pgvector"):
        raise RuntimeError("This Mem0 service supports qdrant or pgvector")
    # The pinned native Mem0 config has no service_tier field. Keep the option
    # with its LLM, then consume it before constructing the native SDK config.
    config = {**config, "llm": {**config["llm"], "config": dict(config["llm"]["config"])}}
    service_tier = config["llm"]["config"].pop("service_tier", "")
    if service_tier not in ("", "auto", "default", "fast", "flex", "priority"):
        raise RuntimeError("Mem0 llm.config.service_tier must be auto, default, fast, flex, priority, or empty")
    config["llm"]["config"]["response_callback"] = _report_llm_response
    luna = _is_luna(config["llm"]["config"].get("model", ""))
    if thinking and luna:
        raise RuntimeError("GPT-6 Luna uses reasoning_effort; omit the Ark thinking setting")
    if embedding_protocol == "ark_multimodal":
        embedding = config.get("embedder", {}).get("config", {})
        if embedding.get("embedding_dims") not in (1024, 2048) or not embedding.get("openai_base_url"):
            raise RuntimeError("Ark embedding requires its explicit base URL and 1024 or 2048 dimensions")
        if config["vector_store"]["config"].get("embedding_model_dims") != embedding["embedding_dims"]:
            raise RuntimeError("Mem0 vector store and Ark embedding dimensions must match")
    memory = Memory.from_config(config)
    memory.llm.client.close()
    memory.llm = ThinkingOpenAILLM(memory.llm.config, thinking, service_tier) if thinking else CompatibleOpenAILLM(memory.llm.config, service_tier)
    if embedding_protocol == "ark_multimodal":
        memory.embedding_model.client.close()
        memory.embedding_model = ArkMultimodalEmbedding(memory.embedding_model.config)
    return memory


def _validate_concurrency(value: Any) -> int:
    if not isinstance(value, int) or isinstance(value, bool) or not 1 <= value <= 1024:
        raise RuntimeError("Mem0 max_concurrency must be an integer between 1 and 1024")
    return value


def _vector_store_config(embedding_dimensions: int) -> dict[str, Any]:
    provider = os.environ.get("MEM0_VECTOR_STORE", "qdrant").strip()
    config: dict[str, Any] = {
        "collection_name": os.environ.get("MEM0_COLLECTION_NAME", "gizclaw_memory").strip(),
        "embedding_model_dims": embedding_dimensions,
    }
    if provider == "qdrant":
        config.update(path=os.environ.get("MEM0_QDRANT_PATH", "/tmp/gizclaw-qdrant"), on_disk=True)
    elif provider == "pgvector":
        dsn = os.environ.get("MEM0_POSTGRES_DSN", "").strip()
        if not dsn:
            raise RuntimeError("MEM0_POSTGRES_DSN is required for pgvector")
        minimum = int(os.environ.get("MEM0_POSTGRES_MIN_CONNECTIONS", "4"))
        maximum = int(os.environ.get("MEM0_POSTGRES_MAX_CONNECTIONS", "32"))
        if not 0 <= minimum <= maximum or maximum < 1 or maximum > 256:
            raise RuntimeError("Mem0 PostgreSQL connections require 0 <= min <= max <= 256 and max >= 1")
        config.update(connection_string=dsn, hnsw=True, minconn=minimum, maxconn=maximum)
    else:
        raise RuntimeError("MEM0_VECTOR_STORE must be qdrant or pgvector")
    return {"provider": provider, "config": config}


@asynccontextmanager
async def _lifespan(_: FastAPI):
    global _memory, _request_limiter, _read_limiter
    _memory = _build_memory()
    concurrency = getattr(_operation_state, "configured_concurrency", None)
    if concurrency is None:
        concurrency = _validate_concurrency(int(os.environ.get("MEM0_MAX_CONCURRENCY", "160")))
    read_concurrency = max(16,concurrency//4)
    anyio.to_thread.current_default_thread_limiter().total_tokens = concurrency+read_concurrency
    _request_limiter = anyio.CapacityLimiter(concurrency)
    _read_limiter = anyio.CapacityLimiter(read_concurrency)
    # Bound individual provider requests beneath the caller's observation
    # deadline. The SDK otherwise permits a single HTTP wait of ten minutes.
    _memory.llm.client = _memory.llm.client.with_options(timeout=30, max_retries=1)
    _memory.embedding_model.client = _memory.embedding_model.client.with_options(timeout=30, max_retries=1)
    try:
        yield
    finally:
        for client in (_memory.llm.client, _memory.embedding_model.client):
            client.close()
        pool = getattr(_memory.vector_store, "connection_pool", None)
        if pool is not None:
            pool.close()
        elif hasattr(_memory.vector_store, "client"):
            _memory.vector_store.client.close()
        _memory.close()
        _memory = None
        _request_limiter = None
        _read_limiter = None


app = FastAPI(title="GizClaw Mem0", lifespan=_lifespan)
app.openapi = contract


@app.exception_handler(LLMError)
@app.exception_handler(APIError)
async def provider_failure(_, error):
    cause = error.__cause__ if isinstance(error, LLMError) else error
    if isinstance(cause, APIStatusError) and cause.status_code == 429:
        return JSONResponse(status_code=429, content={"detail": "Mem0 model provider rate limit exceeded"})
    return JSONResponse(status_code=502, content={"detail": "Mem0 model provider request failed"})


@app.middleware("http")
async def authenticate(request, call_next):
    if request.url.path != "/health" and _service_api_key and not hmac.compare_digest(
        request.headers.get("X-API-Key", "").encode(), _service_api_key.encode()
    ):
        return JSONResponse(status_code=401, content={"detail": "Invalid Mem0 API key"})
    readonly = request.method == "GET" or request.url.path == "/search"
    limiter = (_read_limiter if readonly else _request_limiter) if request.url.path != "/health" else None
    if limiter is None:
        return await call_next(request)
    try:
        limiter.acquire_nowait()
    except anyio.WouldBlock:
        return JSONResponse(status_code=503, content={"detail": "Mem0 concurrency limit reached"})
    try:
        return await call_next(request)
    finally:
        limiter.release()


@app.get("/health", operation_id="health", response_model=HealthStatus)
async def health() -> dict[str, Any]:
    memory = _get_memory()
    return {
        "status": "ready",
        "vector_store": memory.config.vector_store.provider,
        "llm_max_tokens": memory.llm.config.max_tokens,
        "llm_model": memory.llm.config.model,
        "llm_provider": memory.config.llm.provider,
        "llm_thinking": getattr(memory.llm, "thinking", ""),
        "llm_service_tier": getattr(memory.llm,"service_tier", ""),
        "sdk_version": version("mem0ai"),
        "extraction_policy_fingerprint": hashlib.sha256((memory.config.custom_instructions or "").encode()).hexdigest(),
        "embedding_model": memory.embedding_model.config.model,
        "embedding_dimensions": memory.embedding_model.config.embedding_dims,
        "embedding_protocol": getattr(memory.embedding_model, "protocol", "openai"),
        "embedding_policy_fingerprint": memory.embedding_model.policy_fingerprint() if isinstance(memory.embedding_model, ArkMultimodalEmbedding) else "",
    }


@app.post("/memories", operation_id="addMemory", response_model=MemoryResults, response_model_exclude_unset=True)
def add_memory(request: MemoryCreate) -> dict[str, Any]:
    try:
        routing = _routing_kwargs(request.model_dump())
        with _write_lock(routing):
            _operation_state.extraction_response = None
            embedding = _get_memory().embedding_model
            if isinstance(embedding, ArkMultimodalEmbedding):
                embedding.last_error = None
            _operation_state.extraction_required = request.infer
            try:
                result = _get_memory().add(
                    messages=[message.model_dump(exclude_none=True) for message in request.messages],
                    metadata=request.metadata,
                    infer=request.infer,
                    prompt=request.prompt if request.infer else None,
                    **routing,
                )
            finally:
                _operation_state.extraction_required = False
            if isinstance(embedding, ArkMultimodalEmbedding) and embedding.last_error is not None:
                raise HTTPException(status_code=502, detail="Mem0 embedding failed")
            entries = _result_entries(result)
            if request.infer and not entries:
                # An empty result has no committed facts; still distinguish a
                # valid empty extraction from an SDK path that skipped the LLM.
                _validate_extraction_response()
            for entry in entries:
                if entry.get("event") == "ADD" and not isinstance(_get_memory().get(entry["id"]), dict):
                    raise HTTPException(status_code=502, detail="Mem0 reported a fact that was not persisted")
        return {"results": entries}
    except ValueError as error:
        raise HTTPException(status_code=400, detail=str(error)) from error


@app.post("/search", operation_id="searchMemories", response_model=MemoryResults, response_model_exclude_unset=True)
def search_memories(request: SearchRequest) -> dict[str, Any]:
    try:
        with _read_lock():
            result = _get_memory().search(
                query=request.query,
                filters=request.filters,
                top_k=request.top_k,
            )
        return {"results": _result_entries(result)}
    except ValueError as error:
        raise HTTPException(status_code=400, detail=str(error)) from error


@app.get("/memories", operation_id="listMemories", response_model=MemoryResults, response_model_exclude_unset=True)
def list_memories(
    user_id: str | None = None,
    agent_id: str | None = None,
    run_id: str | None = None,
    top_k: int = Query(default=100, ge=1, le=1000),
) -> dict[str, Any]:
    try:
        routing = _routing_kwargs(
            {"user_id": user_id, "agent_id": agent_id, "run_id": run_id}
        )
        with _read_lock():
            result = _get_memory().get_all(filters=routing, top_k=top_k)
        return {"results": _result_entries(result)}
    except ValueError as error:
        raise HTTPException(status_code=400, detail=str(error)) from error


@app.delete("/memories", operation_id="deleteMemories", response_model=StatusResponse)
def delete_memories(
    user_id: str | None = None,
    agent_id: str | None = None,
    run_id: str | None = None,
) -> dict[str, str]:
    try:
        routing = _routing_kwargs(
            {"user_id": user_id, "agent_id": agent_id, "run_id": run_id}
        )
        with _write_lock(routing):
            _get_memory().delete_all(**routing)
        return {"message": "Memories deleted successfully!"}
    except ValueError as error:
        raise HTTPException(status_code=400, detail=str(error)) from error


@app.get("/memories/{memory_id}", operation_id="getMemory", response_model=MemoryRecord, response_model_exclude_unset=True)
def get_memory(memory_id: str) -> dict[str, Any]:
    try:
        with _read_lock():
            result = _get_memory().get(memory_id)
        if not isinstance(result, dict):
            raise ValueError("Mem0 returned an invalid memory")
        return result
    except ValueError as error:
        raise HTTPException(status_code=400, detail=str(error)) from error


@app.put("/memories/{memory_id}", operation_id="updateMemory", response_model=MemoryResults, response_model_exclude_unset=True)
def update_memory(memory_id: str, request: MemoryUpdate) -> dict[str, Any]:
    try:
        with _read_lock():
            existing = _get_memory().get(memory_id)
        if not isinstance(existing, dict):
            raise ValueError("Mem0 returned an invalid memory")
        routing = _routing_kwargs(existing)
        with _write_lock(routing):
            _get_memory().update(memory_id=memory_id, data=request.text)
            result = _get_memory().get(memory_id)
        if not isinstance(result, dict):
            raise ValueError("Mem0 returned an invalid memory")
        return {"results": [result]}
    except ValueError as error:
        raise HTTPException(status_code=400, detail=str(error)) from error


@app.delete("/memories/{memory_id}", operation_id="deleteMemory", status_code=204)
def delete_memory(memory_id: str) -> Response:
    try:
        with _read_lock():
            existing = _get_memory().get(memory_id)
        if not isinstance(existing, dict):
            raise ValueError("Mem0 returned an invalid memory")
        with _write_lock(_routing_kwargs(existing)):
            _get_memory().delete(memory_id=memory_id)
    except ValueError as error:
        raise HTTPException(status_code=400, detail=str(error)) from error
    return Response(status_code=204)
