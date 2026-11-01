ARG BASE_IMAGE=gizcr.gizclaw.com/docker.io/pgvector/pgvector:pg17@sha256:cf134a767f474095eeba57e0117be8e568e011a63f33fbf252f14c9b760f8e6f
FROM ${BASE_IMAGE} AS extensions
ARG DEBIAN_MIRROR=""
ADD --checksum=sha256:a3f100ae871677f0012579f58542c174900297f664a4ad2be0256e7ee5e33502 https://codeload.github.com/pgpartman/pg_partman/tar.gz/refs/tags/v5.5.0 /tmp/pg_partman.tar.gz
RUN set -eu; \
    if [ -n "$DEBIAN_MIRROR" ]; then \
        case "$DEBIAN_MIRROR" in http://*|https://*) ;; *) echo "invalid Debian mirror URL" >&2; exit 1 ;; esac; \
        case "$DEBIAN_MIRROR" in *[!a-zA-Z0-9:/._-]*) echo "invalid Debian mirror URL" >&2; exit 1 ;; esac; \
        for source in /etc/apt/sources.list /etc/apt/sources.list.d/*.sources /etc/apt/sources.list.d/*.list; do \
            if [ -f "$source" ]; then \
                sed -i "s|http://deb.debian.org|${DEBIAN_MIRROR%/}|g;s|https://deb.debian.org|${DEBIAN_MIRROR%/}|g" "$source"; \
            fi; \
        done; \
    fi; \
    apt-get update && apt-get install -y --no-install-recommends make gcc postgresql-server-dev-17 \
    && tar -xzf /tmp/pg_partman.tar.gz -C /tmp \
    && make -C /tmp/pg_partman-5.5.0 install

FROM ${BASE_IMAGE}
COPY --from=extensions /usr/share/postgresql/17/extension/pg_partman* /usr/share/postgresql/17/extension/
COPY --from=extensions /usr/lib/postgresql/17/lib/pg_partman_bgw.so /usr/lib/postgresql/17/lib/
COPY build/postgres/bootstrap-partman.sh /usr/local/bin/bootstrap-partman
