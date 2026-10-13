#!/usr/bin/env bash
set -euo pipefail
psql -v ON_ERROR_STOP=1 --username postgres --dbname postgres \
  --set="app_password=$POSTGRES_PASSWORD" <<'SQL'
CREATE USER sfu_test WITH PASSWORD :'app_password';
CREATE DATABASE server_a OWNER sfu_test;
CREATE DATABASE server_b OWNER sfu_test;
SQL
