CREATE TABLE futures_source_probes (
  id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL CHECK (owner_id > 0),
  source_id TEXT NOT NULL REFERENCES futures_sources(source_id),
  source_version BIGINT NOT NULL CHECK (source_version >= 1),
  adapter_version TEXT NOT NULL,
  config_digest TEXT NOT NULL CHECK (length(config_digest) = 64),
  secret_revision BIGINT NOT NULL CHECK (secret_revision >= 1),
  secret_fingerprint TEXT NOT NULL CHECK (length(secret_fingerprint) BETWEEN 32 AND 128),
  result TEXT NOT NULL CHECK (result IN ('PASS','FAIL','NOT_RUN','BLOCKED')),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 1 AND 500),
  evidence_hash TEXT NOT NULL CHECK (length(evidence_hash) = 64),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (owner_id, id)
);

CREATE TABLE futures_release_gates (
  id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL CHECK (owner_id > 0),
  kind TEXT NOT NULL CHECK (kind IN ('source')),
  probe_id TEXT NOT NULL REFERENCES futures_source_probes(id),
  config_digest TEXT NOT NULL CHECK (length(config_digest) = 64),
  secret_revision BIGINT NOT NULL CHECK (secret_revision >= 1),
  secret_fingerprint TEXT NOT NULL CHECK (length(secret_fingerprint) BETWEEN 32 AND 128),
  status TEXT NOT NULL CHECK (status IN ('active','consumed','invalidated')),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 1 AND 500),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  consumed_operations_version BIGINT,
  invalidated_at TIMESTAMPTZ,
  invalidated_reason TEXT,
  CHECK (expires_at > created_at AND expires_at <= created_at + interval '24 hours'),
  CHECK ((status = 'active' AND consumed_at IS NULL AND invalidated_at IS NULL) OR
         (status = 'consumed' AND consumed_at IS NOT NULL AND invalidated_at IS NULL) OR
         (status = 'invalidated' AND invalidated_at IS NOT NULL)),
  UNIQUE (owner_id, id),
  UNIQUE (probe_id)
);

CREATE INDEX futures_source_probes_owner_created_idx ON futures_source_probes(owner_id, created_at DESC, id DESC);
CREATE INDEX futures_release_gates_owner_status_expiry_idx ON futures_release_gates(owner_id, status, expires_at);
