-- Disposable project-local databases. Application roles never own each other's data.
CREATE ROLE gizclaw_server LOGIN PASSWORD 'gizclaw_server';
CREATE ROLE gizclaw_mem0 LOGIN PASSWORD 'gizclaw_mem0';
CREATE DATABASE gizclaw_server OWNER gizclaw_server;
CREATE DATABASE gizclaw_mem0 OWNER gizclaw_mem0;
REVOKE CONNECT ON DATABASE gizclaw_server FROM PUBLIC;
REVOKE CONNECT ON DATABASE gizclaw_mem0 FROM PUBLIC;
\connect gizclaw_mem0
CREATE EXTENSION vector;
