#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../../../.." && pwd)
VENV="${ROOT}/.tmp/nanopb-venv"
STAMP="${VENV}/.gizclaw-nanopb-0.4.9.1"
PROTOC=$(command -v protoc)

if [ ! -x "${VENV}/bin/python" ] || [ ! -f "${STAMP}" ]; then
  rm -rf "${VENV}"
  python3 -m venv "${VENV}"
  "${VENV}/bin/python" -m pip install --disable-pip-version-check --quiet "protobuf>=3.20,<7" "grpcio-tools>=1.0"
fi

cd "${ROOT}/sdk/c/gizclaw"
PATH="${VENV}/bin:${PATH}" PROTOCOL_BUFFERS_PYTHON_IMPLEMENTATION=python \
  "${PROTOC}" \
    -I ../../../api/rpc \
    --plugin=protoc-gen-nanopb=../../../third_party/nanopb/upstream/generator/protoc-gen-nanopb \
    --nanopb_out=generated \
    --nanopb_opt=-I../../../api/rpc \
    google/protobuf/struct.proto \
    ../../../api/rpc/common.proto \
    ../../../api/rpc/peer.proto \
    ../../../api/rpc/payload.proto

: > "${STAMP}"
