ARG BASE_IMAGE=gizclaw-mem0:linux-amd64-base
FROM ${BASE_IMAGE} AS base
WORKDIR /app
COPY cmd/mem0/gizclaw_mem0 /app/gizclaw_mem0
COPY api/http/mem0.json /app/gizclaw_mem0/openapi.json
RUN mkdir /data && chown 10001:10001 /data
ENV MEM0_DIR=/data/.mem0 MEM0_TELEMETRY=false PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 MEM0_HISTORY_DB_PATH=/data/memory-history.db MEM0_QDRANT_PATH=/data/qdrant
EXPOSE 8000
ENTRYPOINT ["python", "-m", "gizclaw_mem0"]
CMD ["--host", "0.0.0.0", "--port", "8000"]

FROM base AS test
COPY tests/gizclaw-e2e/docker/mem0_server_test.py /app/mem0_server_test.py
COPY tests/gizclaw-e2e/docker/mem0_pgvector_regression_test.py /app/mem0_pgvector_regression_test.py
COPY tests/gizclaw-e2e/docker/mem0_direct_integration_test.py /app/mem0_direct_integration_test.py
RUN python -m unittest -v mem0_server_test.py

FROM base AS runtime
ARG VERSION=dev
ARG SOURCE_COMMIT=dev
ENV GIZCLAW_MEM0_VERSION=${VERSION}
LABEL org.opencontainers.image.title="GizClaw Mem0" org.opencontainers.image.version=${VERSION} org.opencontainers.image.revision=${SOURCE_COMMIT} org.opencontainers.image.source="https://github.com/GizClaw/gizclaw"
USER 10001:10001
VOLUME ["/data"]
