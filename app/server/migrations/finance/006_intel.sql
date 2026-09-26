-- Investment event intelligence and evidence timeline, isolated from research/strategy domains.
CREATE TABLE IF NOT EXISTS finance_intel_namespaces (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  principal_key TEXT NOT NULL,
  generation BIGINT NOT NULL DEFAULT 1,
  replay_version BIGINT NOT NULL DEFAULT 0,
  branch TEXT NOT NULL DEFAULT 'main',
  step_index INTEGER NOT NULL DEFAULT 0,
  simulated_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT finance_intel_namespaces_kind_chk CHECK (kind IN ('live','demo')),
  CONSTRAINT finance_intel_namespaces_branch_chk CHECK (branch IN ('main','denial')),
  CONSTRAINT finance_intel_namespaces_step_chk CHECK (step_index >= 0)
);
CREATE UNIQUE INDEX finance_intel_namespaces_principal ON finance_intel_namespaces(kind, principal_key);

CREATE TABLE IF NOT EXISTS finance_intel_demo_sessions (
  id TEXT PRIMARY KEY,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  bootstrap_hash TEXT NOT NULL,
  idem_scope TEXT NOT NULL,
  idem_key TEXT,
  idem_body_hash TEXT,
  idem_status INTEGER,
  idem_response JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX finance_intel_demo_bootstrap ON finance_intel_demo_sessions(bootstrap_hash);
CREATE UNIQUE INDEX finance_intel_demo_idem ON finance_intel_demo_sessions(idem_scope, idem_key);

CREATE TABLE IF NOT EXISTS finance_intel_instruments (
  id TEXT PRIMARY KEY,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  code TEXT NOT NULL,
  name TEXT NOT NULL,
  exchange TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'fixture',
  verified_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(namespace_id, code)
);

CREATE TABLE IF NOT EXISTS finance_intel_watchlist (
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  principal_key TEXT NOT NULL,
  code TEXT NOT NULL,
  subscribed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  revision BIGINT NOT NULL DEFAULT 1,
  PRIMARY KEY(namespace_id, principal_key, code)
);

CREATE TABLE IF NOT EXISTS finance_intel_sources (
  id TEXT PRIMARY KEY,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  publisher TEXT NOT NULL,
  document_id TEXT NOT NULL,
  normalized_url TEXT NOT NULL DEFAULT '',
  rights TEXT NOT NULL DEFAULT 'summary',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(namespace_id, publisher, document_id)
);

CREATE TABLE IF NOT EXISTS finance_intel_source_revisions (
  id TEXT PRIMARY KEY,
  logical_id TEXT NOT NULL,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  source_id TEXT NOT NULL REFERENCES finance_intel_sources(id) ON DELETE RESTRICT,
  revision_no INTEGER NOT NULL,
  content_hash TEXT NOT NULL,
  title TEXT NOT NULL,
  allowed_excerpt TEXT NOT NULL,
  locator JSONB NOT NULL DEFAULT '{}',
  disclosed_at TIMESTAMPTZ,
  disclosed_date TEXT,
  occurred_at TIMESTAMPTZ,
  fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  source_updated_at TIMESTAMPTZ,
  access_status TEXT NOT NULL DEFAULT 'available',
  is_repost BOOLEAN NOT NULL DEFAULT false,
  supersedes_revision_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(namespace_id, source_id, revision_no),
  UNIQUE(namespace_id, source_id, content_hash),
  UNIQUE(namespace_id, logical_id),
  CONSTRAINT finance_intel_source_access_chk CHECK (access_status IN ('available','dead_link','restricted','cleaned'))
);

CREATE TABLE IF NOT EXISTS finance_intel_events (
  id TEXT PRIMARY KEY,
  logical_id TEXT NOT NULL,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  subject_code TEXT NOT NULL,
  subject_name TEXT NOT NULL,
  matter_key TEXT NOT NULL,
  title TEXT NOT NULL,
  core_claim_key TEXT NOT NULL,
  core_claim_text TEXT NOT NULL,
  current_version INTEGER NOT NULL DEFAULT 0,
  current_snapshot_id TEXT,
  latest_valid_disclosed_at TIMESTAMPTZ,
  review_status TEXT NOT NULL DEFAULT 'normal',
  redirect_event_ids JSONB NOT NULL DEFAULT '[]',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(namespace_id, event_type, subject_code, matter_key),
  UNIQUE(namespace_id, logical_id),
  CONSTRAINT finance_intel_event_type_chk CHECK (event_type IN ('acquisition','earnings_forecast','regulatory_investigation')),
  CONSTRAINT finance_intel_event_review_chk CHECK (review_status IN ('normal','pending','reassigned'))
);

CREATE TABLE IF NOT EXISTS finance_intel_evidence (
  id TEXT PRIMARY KEY,
  logical_id TEXT NOT NULL,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  event_id TEXT NOT NULL REFERENCES finance_intel_events(id) ON DELETE CASCADE,
  source_revision_id TEXT NOT NULL REFERENCES finance_intel_source_revisions(id) ON DELETE RESTRICT,
  claim_key TEXT NOT NULL,
  subject TEXT NOT NULL,
  predicate TEXT NOT NULL,
  object TEXT NOT NULL DEFAULT '',
  period TEXT NOT NULL DEFAULT '',
  currency TEXT NOT NULL DEFAULT '',
  basis TEXT NOT NULL DEFAULT '',
  modality TEXT NOT NULL,
  grade TEXT NOT NULL,
  authoritative BOOLEAN NOT NULL DEFAULT false,
  subject_unique BOOLEAN NOT NULL DEFAULT true,
  modality_explicit BOOLEAN NOT NULL DEFAULT false,
  source_weight TEXT NOT NULL,
  effective_weight TEXT NOT NULL,
  direction TEXT NOT NULL,
  value_json JSONB NOT NULL DEFAULT '{}',
  quote TEXT NOT NULL,
  locator JSONB NOT NULL DEFAULT '{}',
  source_cluster TEXT NOT NULL,
  extraction_run_id TEXT,
  active BOOLEAN NOT NULL DEFAULT true,
  membership_status TEXT NOT NULL DEFAULT 'current',
  supersedes_evidence_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(namespace_id, event_id, source_revision_id, claim_key, predicate),
  UNIQUE(namespace_id, logical_id),
  CONSTRAINT finance_intel_grade_chk CHECK (grade IN ('fact','opinion','inference','rumor')),
  CONSTRAINT finance_intel_direction_chk CHECK (direction IN ('support','refute','context')),
  CONSTRAINT finance_intel_membership_chk CHECK (membership_status IN ('current','superseded','retracted','isolated'))
);

CREATE TABLE IF NOT EXISTS finance_intel_snapshots (
  id TEXT PRIMARY KEY,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  event_id TEXT NOT NULL REFERENCES finance_intel_events(id) ON DELETE CASCADE,
  event_version INTEGER NOT NULL,
  verification TEXT NOT NULL,
  phase TEXT NOT NULL,
  freshness TEXT NOT NULL,
  support_score TEXT NOT NULL,
  support_level TEXT NOT NULL,
  current_values JSONB NOT NULL DEFAULT '[]',
  conclusion TEXT NOT NULL,
  calc_trace JSONB NOT NULL DEFAULT '{}',
  coverage JSONB NOT NULL DEFAULT '{}',
  as_of TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(namespace_id, event_id, event_version),
  CONSTRAINT finance_intel_verification_chk CHECK (verification IN ('unverified','confirmed','denied','disputed')),
  CONSTRAINT finance_intel_support_chk CHECK (support_level IN ('low','medium','high','unknown'))
);

CREATE TABLE IF NOT EXISTS finance_intel_changes (
  id TEXT PRIMARY KEY,
  logical_id TEXT NOT NULL,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  event_id TEXT NOT NULL REFERENCES finance_intel_events(id) ON DELETE CASCADE,
  event_version INTEGER NOT NULL,
  kind TEXT NOT NULL,
  summary TEXT NOT NULL,
  before_snapshot_id TEXT,
  after_snapshot_id TEXT NOT NULL REFERENCES finance_intel_snapshots(id),
  diff JSONB NOT NULL DEFAULT '{}',
  source_revision_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(namespace_id, event_id, event_version)
  ,UNIQUE(namespace_id, logical_id)
);

CREATE TABLE IF NOT EXISTS finance_intel_timeline_nodes (
  id TEXT PRIMARY KEY,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  event_id TEXT NOT NULL REFERENCES finance_intel_events(id) ON DELETE CASCADE,
  source_revision_id TEXT REFERENCES finance_intel_source_revisions(id) ON DELETE RESTRICT,
  event_version INTEGER NOT NULL DEFAULT 0,
  axis TEXT NOT NULL,
  occurred_at TIMESTAMPTZ,
  disclosed_at TIMESTAMPTZ,
  ingested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  title TEXT NOT NULL,
  excerpt TEXT NOT NULL,
  backfilled BOOLEAN NOT NULL DEFAULT false,
  UNIQUE(namespace_id, event_id, source_revision_id, axis),
  CONSTRAINT finance_intel_axis_chk CHECK (axis IN ('disclosure','ingestion'))
);

CREATE TABLE IF NOT EXISTS finance_intel_conflicts (
  id TEXT PRIMARY KEY,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  event_id TEXT NOT NULL REFERENCES finance_intel_events(id) ON DELETE CASCADE,
  claim_key TEXT NOT NULL,
  status TEXT NOT NULL,
  values JSONB NOT NULL DEFAULT '[]',
  evidence_ids JSONB NOT NULL DEFAULT '[]',
  opened_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  opened_from_version INTEGER NOT NULL DEFAULT 0,
  resolved_at TIMESTAMPTZ,
  resolved_from_version INTEGER,
  resolution_revision_id TEXT,
  UNIQUE(namespace_id, event_id, claim_key, status),
  CONSTRAINT finance_intel_conflict_status_chk CHECK (status IN ('open','resolved'))
);

CREATE TABLE IF NOT EXISTS finance_intel_outbox (
  id TEXT PRIMARY KEY,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  change_id TEXT NOT NULL REFERENCES finance_intel_changes(id) ON DELETE CASCADE,
  principal_key TEXT NOT NULL,
  target_codes JSONB NOT NULL DEFAULT '[]',
  status TEXT NOT NULL DEFAULT 'pending',
  lease_owner TEXT,
  lease_epoch BIGINT NOT NULL DEFAULT 0,
  lease_until TIMESTAMPTZ,
  attempts INTEGER NOT NULL DEFAULT 0,
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(namespace_id, change_id, principal_key),
  CONSTRAINT finance_intel_outbox_status_chk CHECK (status IN ('pending','processing','sent','cancelled','failed'))
);

CREATE TABLE IF NOT EXISTS finance_intel_notifications (
  id TEXT PRIMARY KEY,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  principal_key TEXT NOT NULL,
  event_id TEXT NOT NULL REFERENCES finance_intel_events(id) ON DELETE CASCADE,
  change_id TEXT NOT NULL REFERENCES finance_intel_changes(id) ON DELETE CASCADE,
  outbox_id TEXT NOT NULL UNIQUE REFERENCES finance_intel_outbox(id),
  kind TEXT NOT NULL DEFAULT 'event_change',
  title TEXT NOT NULL,
  reason TEXT NOT NULL,
  before JSONB NOT NULL DEFAULT '{}',
  after JSONB NOT NULL DEFAULT '{}',
  target_codes JSONB NOT NULL DEFAULT '[]',
  link_version INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'unread',
  read_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT finance_intel_notification_status_chk CHECK (status IN ('unread','read','ignored'))
);

CREATE TABLE IF NOT EXISTS finance_intel_mutes (
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  principal_key TEXT NOT NULL,
  event_id TEXT NOT NULL REFERENCES finance_intel_events(id) ON DELETE CASCADE,
  muted BOOLEAN NOT NULL,
  effective_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  revision BIGINT NOT NULL DEFAULT 1,
  PRIMARY KEY(namespace_id, principal_key, event_id)
);

CREATE TABLE IF NOT EXISTS finance_intel_jobs (
  id TEXT PRIMARY KEY,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT 'fixture',
  payload JSONB NOT NULL DEFAULT '{}',
  cursor TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'queued',
  owner TEXT,
  lease_epoch BIGINT NOT NULL DEFAULT 0,
  lease_until TIMESTAMPTZ,
  attempts INTEGER NOT NULL DEFAULT 0,
  generation BIGINT NOT NULL DEFAULT 1,
  received_count INTEGER NOT NULL DEFAULT 0,
  processed_count INTEGER NOT NULL DEFAULT 0,
  quarantined_count INTEGER NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT finance_intel_job_status_chk CHECK (status IN ('queued','running','succeeded','partial','failed','cancelled'))
);
CREATE INDEX finance_intel_jobs_claim ON finance_intel_jobs(status, lease_until, created_at);

CREATE TABLE IF NOT EXISTS finance_intel_review_items (
  id TEXT PRIMARY KEY,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  revision INTEGER NOT NULL DEFAULT 1,
  kind TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  payload JSONB NOT NULL DEFAULT '{}',
  reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ,
  UNIQUE(namespace_id, id),
  CONSTRAINT finance_intel_review_kind_chk CHECK (kind IN ('merge','extraction','conflict')),
  CONSTRAINT finance_intel_review_status_chk CHECK (status IN ('pending','resolved','rejected'))
);

CREATE TABLE IF NOT EXISTS finance_intel_idempotency (
  id TEXT PRIMARY KEY,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  principal_key TEXT NOT NULL,
  scope_key TEXT NOT NULL,
  body_hash TEXT NOT NULL,
  status INTEGER NOT NULL,
  response JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  UNIQUE(namespace_id, principal_key, scope_key)
);

CREATE TABLE IF NOT EXISTS finance_intel_extraction_runs (
  id TEXT PRIMARY KEY,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  source_revision_id TEXT NOT NULL REFERENCES finance_intel_source_revisions(id) ON DELETE RESTRICT,
  config_id TEXT NOT NULL,
  config_digest TEXT NOT NULL,
  prompt_version TEXT NOT NULL,
  input_hash TEXT NOT NULL,
  attempt INTEGER NOT NULL,
  status TEXT NOT NULL,
  output JSONB NOT NULL DEFAULT '{}',
  validation JSONB NOT NULL DEFAULT '{}',
  usage JSONB NOT NULL DEFAULT '{}',
  mode TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(namespace_id, source_revision_id, config_digest, prompt_version, input_hash, attempt),
  CONSTRAINT finance_intel_extraction_attempt_chk CHECK (attempt BETWEEN 1 AND 2),
  CONSTRAINT finance_intel_extraction_status_chk CHECK (status IN ('queued','running','succeeded','failed','quarantined')),
  CONSTRAINT finance_intel_extraction_mode_chk CHECK (mode IN ('fixture','live'))
);

CREATE TABLE IF NOT EXISTS finance_intel_event_bindings (
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  event_id TEXT NOT NULL REFERENCES finance_intel_events(id) ON DELETE CASCADE,
  code TEXT NOT NULL,
  role TEXT NOT NULL,
  evidence_ids JSONB NOT NULL DEFAULT '[]',
  PRIMARY KEY(namespace_id, event_id, code, role),
  CONSTRAINT finance_intel_binding_role_chk CHECK (role IN ('subject','counterparty'))
);

CREATE TABLE IF NOT EXISTS finance_intel_evidence_memberships (
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  event_id TEXT NOT NULL REFERENCES finance_intel_events(id) ON DELETE CASCADE,
  evidence_id TEXT NOT NULL REFERENCES finance_intel_evidence(id) ON DELETE CASCADE,
  from_version INTEGER NOT NULL,
  to_version INTEGER,
  validity TEXT NOT NULL DEFAULT 'current',
  supersedes_evidence_id TEXT,
  PRIMARY KEY(namespace_id, event_id, evidence_id, from_version),
  CONSTRAINT finance_intel_membership_validity_chk CHECK (validity IN ('current','superseded','retracted','isolated'))
);

CREATE TABLE IF NOT EXISTS finance_intel_provider_cursors (
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  scope_hash TEXT NOT NULL,
  fetch_cursor TEXT NOT NULL DEFAULT '',
  processed_cursor TEXT NOT NULL DEFAULT '',
  gaps JSONB NOT NULL DEFAULT '[]',
  last_success_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(namespace_id, provider, scope_hash)
);

CREATE TABLE IF NOT EXISTS finance_intel_usage (
  id TEXT PRIMARY KEY,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  job_id TEXT,
  attempt INTEGER NOT NULL,
  reserved_tokens BIGINT NOT NULL DEFAULT 0,
  actual_tokens BIGINT,
  status TEXT NOT NULL DEFAULT 'reserved',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  settled_at TIMESTAMPTZ,
  UNIQUE(namespace_id, job_id, attempt),
  CONSTRAINT finance_intel_usage_status_chk CHECK (status IN ('reserved','settled','usage_unknown'))
);

CREATE TABLE IF NOT EXISTS finance_intel_audit (
  id TEXT PRIMARY KEY,
  namespace_id TEXT NOT NULL REFERENCES finance_intel_namespaces(id) ON DELETE CASCADE,
  actor_key TEXT NOT NULL,
  action TEXT NOT NULL,
  resource_id TEXT NOT NULL,
  before_ref TEXT NOT NULL DEFAULT '',
  after_ref TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  request_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
