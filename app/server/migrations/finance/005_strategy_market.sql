-- Stage B §12.2: strategy market content domain + strategy authoring extensions.
-- App-side ID prefixes: smi_ (items), smv_ (versions), sme_ (evidence), sma_ (audit).
-- Market content is append-only where required; withdrawn items are never physically deleted.

-- 1. Items first (current_version FK added after versions exist).
CREATE TABLE IF NOT EXISTS finance_strategy_market_items (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL,
  current_version_id TEXT,
  revision INTEGER NOT NULL DEFAULT 1,
  created_by INTEGER NOT NULL REFERENCES finance_users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  withdrawn_at TIMESTAMPTZ,
  CONSTRAINT finance_strategy_market_items_status_chk CHECK (status IN ('draft', 'published', 'withdrawn')),
  CONSTRAINT finance_strategy_market_items_revision_chk CHECK (revision >= 1),
  CONSTRAINT finance_strategy_market_items_published_chk CHECK (status <> 'published' OR current_version_id IS NOT NULL)
);
CREATE INDEX finance_strategy_market_items_list ON finance_strategy_market_items (status, updated_at DESC, id DESC);

-- 2. Immutable content versions. Item lookups are served by UNIQUE (item_id, version_no).
CREATE TABLE IF NOT EXISTS finance_strategy_market_versions (
  id TEXT PRIMARY KEY,
  item_id TEXT NOT NULL REFERENCES finance_strategy_market_items(id) ON DELETE RESTRICT,
  version_no INTEGER NOT NULL,
  name TEXT NOT NULL,
  summary TEXT NOT NULL,
  category TEXT NOT NULL,
  tags JSONB NOT NULL DEFAULT '[]',
  markets JSONB NOT NULL DEFAULT '[]',
  signal_period TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  hypothesis TEXT NOT NULL DEFAULT '',
  failure_cases TEXT NOT NULL DEFAULT '',
  sources JSONB NOT NULL DEFAULT '[]',
  rights_note TEXT NOT NULL DEFAULT '',
  editor_schema_version TEXT NOT NULL,
  rule_template JSONB NOT NULL,
  backtest_defaults JSONB NOT NULL DEFAULT '{}',
  content_hash TEXT NOT NULL,
  validation_status TEXT NOT NULL DEFAULT 'pending',
  validation_report JSONB NOT NULL DEFAULT '{}',
  created_by INTEGER NOT NULL REFERENCES finance_users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  validated_at TIMESTAMPTZ,
  published_at TIMESTAMPTZ,
  CONSTRAINT finance_strategy_market_versions_no_uniq UNIQUE (item_id, version_no),
  CONSTRAINT finance_strategy_market_versions_item_uniq UNIQUE (item_id, id),
  CONSTRAINT finance_strategy_market_versions_validation_chk CHECK (validation_status IN ('pending', 'passed', 'failed')),
  CONSTRAINT finance_strategy_market_versions_name_chk CHECK (char_length(name) BETWEEN 1 AND 80),
  CONSTRAINT finance_strategy_market_versions_summary_chk CHECK (char_length(summary) BETWEEN 1 AND 160),
  CONSTRAINT finance_strategy_market_versions_tags_chk CHECK (jsonb_typeof(tags) = 'array'),
  CONSTRAINT finance_strategy_market_versions_markets_chk CHECK (jsonb_typeof(markets) = 'array'),
  CONSTRAINT finance_strategy_market_versions_sources_chk CHECK (jsonb_typeof(sources) = 'array'),
  CONSTRAINT finance_strategy_market_versions_rule_chk CHECK (jsonb_typeof(rule_template) = 'object'),
  CONSTRAINT finance_strategy_market_versions_defaults_chk CHECK (jsonb_typeof(backtest_defaults) = 'object'),
  CONSTRAINT finance_strategy_market_versions_report_chk CHECK (jsonb_typeof(validation_report) = 'object')
);
CREATE INDEX finance_strategy_market_versions_category ON finance_strategy_market_versions (category);
CREATE INDEX finance_strategy_market_versions_period ON finance_strategy_market_versions (signal_period);

-- 3. Reviewed backtest evidence snapshots. No placeholder rows: absence of a row means "not tested".
CREATE TABLE IF NOT EXISTS finance_strategy_market_evidence (
  id TEXT PRIMARY KEY,
  item_id TEXT NOT NULL,
  market_version_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  validation_instrument_id TEXT NOT NULL,
  bound_dsl_hash TEXT NOT NULL,
  config JSONB NOT NULL DEFAULT '{}',
  manifest JSONB NOT NULL DEFAULT '{}',
  metrics JSONB NOT NULL DEFAULT '{}',
  equity JSONB NOT NULL DEFAULT '[]',
  trades JSONB NOT NULL DEFAULT '[]',
  limitations JSONB NOT NULL DEFAULT '[]',
  result_hash TEXT NOT NULL,
  evidence_hash TEXT NOT NULL,
  review_note TEXT NOT NULL DEFAULT '',
  created_by INTEGER NOT NULL REFERENCES finance_users(id),
  reviewed_by INTEGER REFERENCES finance_users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  reviewed_at TIMESTAMPTZ,
  CONSTRAINT finance_strategy_market_evidence_version_fk FOREIGN KEY (item_id, market_version_id) REFERENCES finance_strategy_market_versions (item_id, id) ON DELETE RESTRICT,
  CONSTRAINT finance_strategy_market_evidence_status_chk CHECK (status IN ('pending', 'approved', 'rejected', 'revoked')),
  CONSTRAINT finance_strategy_market_evidence_config_chk CHECK (jsonb_typeof(config) = 'object'),
  CONSTRAINT finance_strategy_market_evidence_manifest_chk CHECK (jsonb_typeof(manifest) = 'object'),
  CONSTRAINT finance_strategy_market_evidence_metrics_chk CHECK (jsonb_typeof(metrics) = 'object'),
  CONSTRAINT finance_strategy_market_evidence_equity_chk CHECK (jsonb_typeof(equity) = 'array'),
  CONSTRAINT finance_strategy_market_evidence_trades_chk CHECK (jsonb_typeof(trades) = 'array'),
  CONSTRAINT finance_strategy_market_evidence_limits_chk CHECK (jsonb_typeof(limitations) = 'array')
);
CREATE INDEX finance_strategy_market_evidence_list ON finance_strategy_market_evidence (market_version_id, status, created_at DESC, id DESC);

-- 4. Append-only audit trail (application must never UPDATE or DELETE rows).
CREATE TABLE IF NOT EXISTS finance_strategy_market_audit (
  id TEXT PRIMARY KEY,
  item_id TEXT NOT NULL REFERENCES finance_strategy_market_items(id) ON DELETE RESTRICT,
  market_version_id TEXT,
  actor_id INTEGER NOT NULL REFERENCES finance_users(id),
  action TEXT NOT NULL,
  request_id TEXT NOT NULL,
  before_revision INTEGER NOT NULL,
  after_revision INTEGER NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  payload_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT finance_strategy_market_audit_version_fk FOREIGN KEY (item_id, market_version_id) REFERENCES finance_strategy_market_versions (item_id, id) ON DELETE RESTRICT,
  CONSTRAINT finance_strategy_market_audit_action_chk CHECK (action IN ('create_version', 'validate', 'publish', 'withdraw', 'import_evidence', 'approve_evidence', 'reject_evidence', 'revoke_evidence'))
);
CREATE INDEX finance_strategy_market_audit_item ON finance_strategy_market_audit (item_id, created_at DESC, id DESC);

CREATE FUNCTION finance_strategy_market_audit_guard() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'finance_strategy_market_audit is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER finance_strategy_market_audit_guard
  BEFORE UPDATE OR DELETE ON finance_strategy_market_audit
  FOR EACH ROW EXECUTE FUNCTION finance_strategy_market_audit_guard();

-- 5. Composite FK keeps items.current_version_id pointing at one of the item's own versions.
ALTER TABLE finance_strategy_market_items
  ADD CONSTRAINT finance_strategy_market_items_current_fk FOREIGN KEY (id, current_version_id) REFERENCES finance_strategy_market_versions (item_id, id);

-- 6. Draft authoring extensions (editor state + market provenance).
ALTER TABLE finance_strategy_drafts ADD COLUMN IF NOT EXISTS editor_schema_version TEXT;
ALTER TABLE finance_strategy_drafts ADD COLUMN IF NOT EXISTS editor_state JSONB;
ALTER TABLE finance_strategy_drafts ADD COLUMN IF NOT EXISTS field_sources JSONB NOT NULL DEFAULT '{}';
ALTER TABLE finance_strategy_drafts ADD COLUMN IF NOT EXISTS backtest_config_draft JSONB NOT NULL DEFAULT '{}';
ALTER TABLE finance_strategy_drafts ADD COLUMN IF NOT EXISTS origin_market_item_id TEXT;
ALTER TABLE finance_strategy_drafts ADD COLUMN IF NOT EXISTS origin_market_version_id TEXT;
ALTER TABLE finance_strategy_drafts ADD CONSTRAINT finance_strategy_drafts_origin_pair_chk CHECK ((origin_market_item_id IS NULL) = (origin_market_version_id IS NULL));
ALTER TABLE finance_strategy_drafts ADD CONSTRAINT finance_strategy_drafts_editor_state_chk CHECK (editor_state IS NULL OR jsonb_typeof(editor_state) = 'object');
ALTER TABLE finance_strategy_drafts ADD CONSTRAINT finance_strategy_drafts_field_sources_chk CHECK (jsonb_typeof(field_sources) = 'object');
ALTER TABLE finance_strategy_drafts ADD CONSTRAINT finance_strategy_drafts_backtest_cfg_chk CHECK (jsonb_typeof(backtest_config_draft) = 'object');
ALTER TABLE finance_strategy_drafts ADD CONSTRAINT finance_strategy_drafts_origin_fk FOREIGN KEY (origin_market_item_id, origin_market_version_id) REFERENCES finance_strategy_market_versions (item_id, id);

-- 7. Saved versions freeze the same provenance at save time; legacy rows keep NULLs.
ALTER TABLE finance_strategy_versions ADD COLUMN IF NOT EXISTS editor_schema_version TEXT;
ALTER TABLE finance_strategy_versions ADD COLUMN IF NOT EXISTS editor_state JSONB;
ALTER TABLE finance_strategy_versions ADD COLUMN IF NOT EXISTS field_sources JSONB NOT NULL DEFAULT '{}';
ALTER TABLE finance_strategy_versions ADD COLUMN IF NOT EXISTS backtest_config_draft JSONB NOT NULL DEFAULT '{}';
ALTER TABLE finance_strategy_versions ADD COLUMN IF NOT EXISTS origin_market_item_id TEXT;
ALTER TABLE finance_strategy_versions ADD COLUMN IF NOT EXISTS origin_market_version_id TEXT;
ALTER TABLE finance_strategy_versions ADD CONSTRAINT finance_strategy_versions_origin_pair_chk CHECK ((origin_market_item_id IS NULL) = (origin_market_version_id IS NULL));
ALTER TABLE finance_strategy_versions ADD CONSTRAINT finance_strategy_versions_editor_state_chk CHECK (editor_state IS NULL OR jsonb_typeof(editor_state) = 'object');
ALTER TABLE finance_strategy_versions ADD CONSTRAINT finance_strategy_versions_field_sources_chk CHECK (jsonb_typeof(field_sources) = 'object');
ALTER TABLE finance_strategy_versions ADD CONSTRAINT finance_strategy_versions_backtest_cfg_chk CHECK (jsonb_typeof(backtest_config_draft) = 'object');
ALTER TABLE finance_strategy_versions ADD CONSTRAINT finance_strategy_versions_origin_fk FOREIGN KEY (origin_market_item_id, origin_market_version_id) REFERENCES finance_strategy_market_versions (item_id, id);

-- 8. Generation mode (generate / modify / explain) and explain text.
ALTER TABLE finance_strategy_generations ADD COLUMN IF NOT EXISTS mode TEXT NOT NULL DEFAULT 'generate';
ALTER TABLE finance_strategy_generations ADD COLUMN IF NOT EXISTS explanation TEXT;
ALTER TABLE finance_strategy_generations ADD CONSTRAINT finance_strategy_generations_mode_chk CHECK (mode IN ('generate', 'modify', 'explain'));

-- 9. Idempotent replay snapshot for new operations (market_copy etc.).
ALTER TABLE finance_strategy_idempotency ADD COLUMN IF NOT EXISTS response_snapshot JSONB;
ALTER TABLE finance_strategy_idempotency ADD CONSTRAINT finance_strategy_idempotency_snapshot_chk CHECK (response_snapshot IS NULL OR jsonb_typeof(response_snapshot) = 'object');
