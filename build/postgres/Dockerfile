ARG BASE_IMAGE=gizcr.gizclaw.com/docker.io/pgvector/pgvector:pg17@sha256:cf134a767f474095eeba57e0117be8e568e011a63f33fbf252f14c9b760f8e6f
FROM ${BASE_IMAGE} AS extensions
ADD --checksum=sha256:a3f100ae871677f0012579f58542c174900297f664a4ad2be0256e7ee5e33502 https://codeload.github.com/pgpartman/pg_partman/tar.gz/refs/tags/v5.5.0 /tmp/pg_partman.tar.gz
RUN apt-get update && apt-get install -y --no-install-recommends make gcc postgresql-server-dev-17 \
    && tar -xzf /tmp/pg_partman.tar.gz -C /tmp \
    && make -C /tmp/pg_partman-5.5.0 install

FROM ${BASE_IMAGE}
COPY --from=extensions /usr/share/postgresql/17/extension/pg_partman* /usr/share/postgresql/17/extension/
COPY --from=extensions /usr/lib/postgresql/17/lib/pg_partman_bgw.so /usr/lib/postgresql/17/lib/
COPY build/postgres/bootstrap-partman.sh /usr/local/bin/bootstrap-partman
