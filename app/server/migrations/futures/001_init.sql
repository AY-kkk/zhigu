CREATE TABLE futures_products (
  id TEXT PRIMARY KEY,
  product_id TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  exchange TEXT NOT NULL,
  template_version TEXT NOT NULL,
  admission TEXT NOT NULL CHECK (admission IN ('blocked','admitted','suspended')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);

CREATE TABLE futures_contracts (
  id TEXT PRIMARY KEY,
  product_id TEXT NOT NULL REFERENCES futures_products(product_id),
  kind TEXT NOT NULL CHECK (kind='actual'),
  last_trading_at TIMESTAMPTZ NOT NULL,
  price_precision INTEGER NOT NULL CHECK (price_precision >= 0),
  price_unit TEXT NOT NULL,
  multiplier NUMERIC(38,12) NOT NULL CHECK (multiplier > 0),
  calendar_version TEXT NOT NULL,
  source_version TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  UNIQUE (id, source_version),
  UNIQUE (product_id, last_trading_at, source_version)
);

CREATE TABLE futures_sources (
  id TEXT PRIMARY KEY,
  source_id TEXT NOT NULL UNIQUE,
  version INTEGER NOT NULL CHECK (version >= 1),
  publisher TEXT NOT NULL,
  url TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('registered','enabled','paused','revoked')),
  credential_ref TEXT NOT NULL,
  rights JSONB NOT NULL,
  metrics JSONB NOT NULL,
  schedule_version TEXT NOT NULL,
  adapter_version TEXT NOT NULL,
  coverage_start DATE,
  coverage_end DATE,
  health TEXT NOT NULL CHECK (health IN ('unknown','healthy','degraded','failed')),
  manifest JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);

CREATE TABLE futures_source_versions (
  source_id TEXT NOT NULL,
  version INTEGER NOT NULL CHECK (version >= 1),
  manifest JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (source_id, version)
);

CREATE TABLE futures_series (
  id TEXT PRIMARY KEY,
  product_id TEXT NOT NULL REFERENCES futures_products(product_id),
  contract_id TEXT REFERENCES futures_contracts(id),
  metric TEXT NOT NULL,
  unit TEXT NOT NULL,
  currency TEXT,
  caliber TEXT NOT NULL,
  frequency TEXT NOT NULL,
  calendar_version TEXT NOT NULL,
  caliber_hash TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);

CREATE TABLE futures_observations (
  id TEXT PRIMARY KEY,
  scope TEXT NOT NULL CHECK (scope IN ('public','private')),
  owner_id BIGINT,
  mode TEXT CHECK (mode IS NULL OR mode='live'),
  source_id TEXT NOT NULL REFERENCES futures_sources(source_id),
  natural_key TEXT NOT NULL,
  source_version TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  series_id TEXT NOT NULL REFERENCES futures_series(id),
  product_id TEXT NOT NULL,
  contract_id TEXT,
  source_cluster TEXT NOT NULL,
  caliber_id TEXT NOT NULL,
  period_start DATE NOT NULL,
  period_end DATE NOT NULL,
  trading_day DATE,
  revision INTEGER NOT NULL CHECK (revision >= 1),
  supersedes_id TEXT REFERENCES futures_observations(id),
  numeric_value NUMERIC(38,12),
  text_value TEXT,
  unit TEXT NOT NULL,
  published_at TIMESTAMPTZ,
  published_precision TEXT NOT NULL CHECK (published_precision IN ('timestamp','date','unknown')),
  version_available_at TIMESTAMPTZ,
  first_observed_at TIMESTAMPTZ NOT NULL,
  retrieved_at TIMESTAMPTZ NOT NULL,
  usable_at TIMESTAMPTZ NOT NULL,
  quality JSONB NOT NULL,
  source_type TEXT NOT NULL CHECK (source_type IN ('official_release','market_data','industry_data','user_document','calculation')),
  locator JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  UNIQUE (source_id, natural_key, source_version, content_hash),
  UNIQUE (series_id, period_start, revision, content_hash)
);

CREATE TABLE futures_manifests (
  id TEXT PRIMARY KEY,
  owner_id BIGINT,
  mode TEXT CHECK (mode IS NULL OR mode='live'),
  product_id TEXT NOT NULL,
  as_of TIMESTAMPTZ NOT NULL,
  manifest_hash TEXT NOT NULL,
  versions JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);

CREATE TABLE futures_manifest_records (
  manifest_id TEXT NOT NULL REFERENCES futures_manifests(id),
  record_id TEXT NOT NULL REFERENCES futures_observations(id),
  position INTEGER NOT NULL,
  PRIMARY KEY (manifest_id, record_id)
);

CREATE TABLE futures_documents (
  id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  filename TEXT NOT NULL,
  media_type TEXT NOT NULL,
  byte_size BIGINT NOT NULL CHECK (byte_size BETWEEN 1 AND 20971520),
  content_hash TEXT NOT NULL,
  storage_key TEXT NOT NULL,
  extraction_status TEXT NOT NULL,
  page_count INTEGER,
  word_count INTEGER,
  failure_code TEXT,
  extracted_text TEXT NOT NULL DEFAULT '',
  purge_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  UNIQUE (owner_id, mode, id)
);

CREATE TABLE futures_drafts (
  id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  revision BIGINT NOT NULL CHECK (revision >= 1),
  input JSONB NOT NULL,
  parse_state TEXT NOT NULL CHECK (parse_state IN ('not_started','queued','running','succeeded','failed')),
  parsed_revision BIGINT,
  claims JSONB NOT NULL DEFAULT '[]'::jsonb,
  claims_overflow BOOLEAN NOT NULL DEFAULT false,
  consumed_model_calls INTEGER NOT NULL DEFAULT 0 CHECK (consumed_model_calls >= 0),
  consumed_tokens INTEGER NOT NULL DEFAULT 0 CHECK (consumed_tokens >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  UNIQUE (owner_id, mode, id),
  UNIQUE (owner_id, mode, id, revision)
);

CREATE TABLE futures_draft_versions (
  draft_id TEXT NOT NULL,
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  revision BIGINT NOT NULL,
  input JSONB NOT NULL,
  claims JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (draft_id, revision),
  FOREIGN KEY (draft_id, owner_id, mode) REFERENCES futures_drafts(id, owner_id, mode)
);

CREATE TABLE futures_runs (
  id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  draft_id TEXT NOT NULL,
  draft_revision BIGINT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('queued','running','verifying','succeeded','failed','canceling','canceled')),
  stage TEXT NOT NULL CHECK (stage IN ('queued','evidence','support','challenge','verify','complete')),
  as_of TIMESTAMPTZ NOT NULL,
  horizon_end TIMESTAMPTZ NOT NULL,
  report JSONB,
  failure_code TEXT,
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  manifest_id TEXT NOT NULL DEFAULT 'pending',
  claims_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb,
  versions JSONB NOT NULL DEFAULT '{}'::jsonb,
  task_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  generation BIGINT NOT NULL DEFAULT 1,
  cancel_requested BOOLEAN NOT NULL DEFAULT false,
  lease_until TIMESTAMPTZ,
  lease_owner TEXT,
  deadline_at TIMESTAMPTZ,
  version BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  UNIQUE (owner_id, mode, id),
  UNIQUE (owner_id, mode, idempotency_key)
);

CREATE TABLE futures_reports (
  id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  run_id TEXT NOT NULL,
  report_version INTEGER NOT NULL,
  body JSONB NOT NULL,
  verified BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  UNIQUE (owner_id, mode, run_id, report_version),
  UNIQUE (owner_id, mode, id)
);

CREATE TABLE futures_evidence_links (
  id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  run_id TEXT NOT NULL,
  record_id TEXT NOT NULL REFERENCES futures_observations(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  UNIQUE (owner_id, mode, run_id, record_id)
);

CREATE TABLE futures_calculations (
  id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  run_id TEXT NOT NULL,
  formula TEXT NOT NULL,
  formula_version TEXT NOT NULL,
  input_record_ids JSONB NOT NULL,
  value NUMERIC(38,12),
  unit TEXT NOT NULL,
  unavailable_reason TEXT,
  computed_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  UNIQUE (owner_id, mode, id)
);

CREATE TABLE futures_hypotheses (
  id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  version BIGINT NOT NULL CHECK (version >= 1),
  run_id TEXT NOT NULL,
  report_id TEXT NOT NULL,
  claim_ids JSONB NOT NULL,
  proposition TEXT NOT NULL,
  product_id TEXT NOT NULL,
  contract_id TEXT,
  expires_at TIMESTAMPTZ NOT NULL,
  retention_deadline TIMESTAMPTZ NOT NULL,
  lifecycle TEXT NOT NULL CHECK (lifecycle IN ('draft','active','paused','expired','closed','deleted')),
  user_view TEXT NOT NULL CHECK (user_view IN ('undetermined','retained','revised','rejected')),
  view_reason TEXT,
  reviewed_check_version BIGINT,
  needs_review BOOLEAN NOT NULL DEFAULT false,
  conditions JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  UNIQUE (owner_id, mode, id),
  UNIQUE (owner_id, mode, id, version)
);

CREATE TABLE futures_hypothesis_versions (
  hypothesis_id TEXT NOT NULL,
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  version BIGINT NOT NULL,
  body JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (hypothesis_id, version),
  FOREIGN KEY (hypothesis_id, owner_id, mode) REFERENCES futures_hypotheses(id, owner_id, mode)
);

CREATE TABLE futures_checks (
  id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  hypothesis_id TEXT NOT NULL,
  hypothesis_version BIGINT NOT NULL,
  condition_id TEXT NOT NULL,
  version BIGINT NOT NULL,
  result TEXT NOT NULL CHECK (result IN ('pending','met','not_met','unknown')),
  evidence_ids JSONB NOT NULL,
  reason TEXT NOT NULL,
  checked_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (owner_id, mode, hypothesis_id, hypothesis_version, condition_id, version)
);

CREATE TABLE futures_recaps (
  hypothesis_id TEXT NOT NULL,
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  version BIGINT NOT NULL,
  facts TEXT NOT NULL,
  transmission TEXT NOT NULL,
  contract_performance TEXT NOT NULL,
  data_sufficiency TEXT NOT NULL,
  reason TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (hypothesis_id, version)
);

CREATE TABLE futures_change_events (
  id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  object_id TEXT NOT NULL,
  object_version BIGINT NOT NULL,
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE futures_outbox (
  id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  dedupe_key TEXT NOT NULL,
  payload JSONB NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('pending','delivered','dead')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (dedupe_key)
);

CREATE TABLE futures_notifications (
  id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  type TEXT NOT NULL,
  object_id TEXT NOT NULL,
  object_version BIGINT NOT NULL,
  dedupe_key TEXT NOT NULL UNIQUE,
  read_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);

CREATE TABLE futures_watchlists (
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  product_ids JSONB NOT NULL,
  version BIGINT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (owner_id, mode)
);

CREATE TABLE futures_idempotency_keys (
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  operation TEXT NOT NULL,
  key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  response JSONB,
  status TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (owner_id, mode, operation, key)
);

CREATE TABLE futures_budget_accounts (
  owner_id BIGINT NOT NULL,
  scope TEXT NOT NULL CHECK (scope IN ('user','module')),
  budget_date DATE NOT NULL,
  limit_cny NUMERIC(38,12) NOT NULL CHECK (limit_cny >= 0),
  reserved_cny NUMERIC(38,12) NOT NULL DEFAULT 0 CHECK (reserved_cny >= 0),
  spent_cny NUMERIC(38,12) NOT NULL DEFAULT 0 CHECK (spent_cny >= 0),
  model_calls INTEGER NOT NULL DEFAULT 0,
  tool_calls INTEGER NOT NULL DEFAULT 0,
  tokens INTEGER NOT NULL DEFAULT 0,
  model_call_limit INTEGER NOT NULL CHECK (model_call_limit >= 0),
  tool_call_limit INTEGER NOT NULL CHECK (tool_call_limit >= 0),
  token_limit INTEGER NOT NULL CHECK (token_limit >= 0),
  PRIMARY KEY (scope, owner_id, budget_date)
);

CREATE TABLE futures_budget_reservations (
  attempt_id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  run_id TEXT NOT NULL,
  budget_date DATE NOT NULL,
  amount_cny NUMERIC(38,12) NOT NULL CHECK (amount_cny >= 0),
  tokens INTEGER NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('model','tool')),
  state TEXT NOT NULL CHECK (state IN ('reserved','settled','unknown')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE futures_usage (
  attempt_id TEXT PRIMARY KEY REFERENCES futures_budget_reservations(attempt_id),
  owner_id BIGINT NOT NULL,
  run_id TEXT NOT NULL,
  input_tokens INTEGER,
  output_tokens INTEGER,
  usage_state TEXT NOT NULL CHECK (usage_state IN ('known','unknown')),
  recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE futures_operations (
  version BIGINT PRIMARY KEY,
  mode TEXT NOT NULL CHECK (mode IN ('off','read_only','live')),
  reason TEXT NOT NULL,
  allowed_user_ids JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE futures_admission (
  product_id TEXT NOT NULL,
  version BIGINT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('blocked','admitted','suspended')),
  evidence_manifest_id TEXT NOT NULL,
  reason TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (product_id, version)
);

CREATE TABLE futures_deletion_jobs (
  id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  mode TEXT NOT NULL CHECK (mode='live'),
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL,
  impact_version TEXT NOT NULL,
  purge_due_at TIMESTAMPTZ NOT NULL,
  eligible_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  retry_count INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  state TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (owner_id, mode, target_type, target_id)
);

CREATE TABLE futures_tombstones (
  request_id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  object_type TEXT NOT NULL,
  object_id TEXT NOT NULL,
  hidden_at TIMESTAMPTZ NOT NULL,
  purge_due_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE futures_audit (
  id TEXT PRIMARY KEY,
  actor_id BIGINT,
  action TEXT NOT NULL,
  object_type TEXT NOT NULL,
  object_id TEXT NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE futures_grants (
  nonce TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  run_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  generation BIGINT NOT NULL,
  manifest_id TEXT NOT NULL,
  stage TEXT NOT NULL,
  tool_name TEXT NOT NULL,
  args_hash TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ
);

CREATE INDEX futures_runs_owner_created ON futures_runs(owner_id, mode, created_at DESC, id DESC);
CREATE INDEX futures_runs_claimable ON futures_runs(status, lease_until) WHERE deleted_at IS NULL;
CREATE INDEX futures_observations_usable ON futures_observations(series_id, usable_at, revision);
CREATE INDEX futures_notifications_owner_created ON futures_notifications(owner_id, mode, created_at DESC, id DESC);
CREATE INDEX futures_outbox_pending ON futures_outbox(state, next_attempt_at) WHERE state='pending';

INSERT INTO futures_products(id, product_id, name, exchange, template_version, admission)
VALUES ('SHFE.CU', 'SHFE.CU', '铜', 'SHFE', 'futures.cu.template.v1', 'blocked')
ON CONFLICT (product_id) DO NOTHING;
