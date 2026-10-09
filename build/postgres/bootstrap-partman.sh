#!/usr/bin/env bash
set -euo pipefail
# This one-shot Compose service also runs against an existing data volume.
: "${PG_PARTMAN_DATABASES:?set comma-separated application database names}"
IFS=',' read -r -a databases <<< "$PG_PARTMAN_DATABASES"
for database in "${databases[@]}"; do
  psql --dbname="$database" --no-psqlrc --set=ON_ERROR_STOP=1 <<'SQL'
CREATE EXTENSION IF NOT EXISTS vector;
CREATE SCHEMA IF NOT EXISTS partman;
CREATE EXTENSION IF NOT EXISTS pg_partman WITH SCHEMA partman;
ALTER EXTENSION pg_partman UPDATE TO '5.5.0';
DO $$
DECLARE application_owner name;
BEGIN
  SELECT pg_get_userbyid(datdba) INTO application_owner FROM pg_database WHERE datname = current_database();
  EXECUTE format('GRANT USAGE, CREATE ON SCHEMA partman TO %I', application_owner);
  EXECUTE format('GRANT ALL ON ALL TABLES IN SCHEMA partman TO %I', application_owner);
  EXECUTE format('GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA partman TO %I', application_owner);
  EXECUTE format('GRANT EXECUTE ON ALL PROCEDURES IN SCHEMA partman TO %I', application_owner);
END $$;
SQL
done
