-- audit_events: JSONB-authoritative event log, keyed by (tenant_id, chain_id,
-- position). The stored `event` JSONB is the byte-for-byte marshaling of
-- audit.Event, including the nanosecond-precision OccurredAt timestamp that
-- VerifyChain's hash depends on. A typed TIMESTAMPTZ column truncates to
-- microseconds and would silently break chain verification after a restart
-- — the exact failure this schema exists to prevent. `seq` is a
-- monotonically increasing identity used only for insertion-order queries;
-- it plays no role in the hash chain itself.
CREATE TABLE audit_events (
  seq         BIGINT GENERATED ALWAYS AS IDENTITY,
  tenant_id   UUID  NOT NULL,
  chain_id    UUID  NOT NULL,
  position    INT   NOT NULL,
  event       JSONB NOT NULL,

  PRIMARY KEY (tenant_id, chain_id, position)
);

CREATE INDEX idx_audit_events_tenant ON audit_events (tenant_id);
CREATE INDEX idx_audit_events_seq ON audit_events (seq);

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

-- tenant_rules: the current trigger-rule set per tenant (ADR-0002). Unlike
-- audit_events this table is mutated in place — RuleStore.MutateRules
-- replaces a tenant's whole rule set (delete + insert) inside one
-- transaction guarded by a per-tenant advisory lock, matching the
-- in-memory backend's semantics.
CREATE TABLE tenant_rules (
  tenant_id UUID  NOT NULL,
  rule_id   UUID  NOT NULL,
  rule      JSONB NOT NULL,

  PRIMARY KEY (tenant_id, rule_id)
);
