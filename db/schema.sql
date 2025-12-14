CREATE EXTENSION IF NOT EXISTS pgcrypto;


CREATE TABLE audit_events (
  id            BIGSERIAL PRIMARY KEY,
  tenant_id     UUID        NOT NULL,
  occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

  actor         JSONB       NOT NULL,
  action        TEXT        NOT NULL,
  resource      JSONB       NOT NULL,
  metadata      JSONB,

  prev_hash     BYTEA       NOT NULL,
  event_hash    BYTEA       NOT NULL
);

CREATE OR REPLACE FUNCTION forbid_mutation()
RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'audit_events are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER no_update
BEFORE UPDATE ON audit_events
FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TRIGGER no_delete
BEFORE DELETE ON audit_events
FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE INDEX idx_audit_tenant_time
ON audit_events (tenant_id, occurred_at);

CREATE INDEX idx_audit_action
ON audit_events (action);

CREATE INDEX idx_audit_actor
ON audit_events
USING GIN ((actor->'id'));

CREATE INDEX idx_audit_resource
ON audit_events
USING GIN ((resource->'id'));

