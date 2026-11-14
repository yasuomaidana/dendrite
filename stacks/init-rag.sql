-- dendrite-rag shared tier: the dendrite_rag role and database with pgvector.
-- Runs on a fresh volume through /docker-entrypoint-initdb.d; idempotent, so an existing
-- stack can apply it too:
--   docker exec -i dendrite-postgres psql -U dendrite -d postgres < stacks/init-rag.sql
SELECT 'CREATE ROLE dendrite_rag WITH LOGIN PASSWORD ''dendrite_rag'''
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'dendrite_rag')\gexec
SELECT 'CREATE DATABASE dendrite_rag OWNER dendrite_rag'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'dendrite_rag')\gexec
\connect dendrite_rag
CREATE EXTENSION IF NOT EXISTS vector;
GRANT ALL ON SCHEMA public TO dendrite_rag;
