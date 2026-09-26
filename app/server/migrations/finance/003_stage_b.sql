-- Stage B: provider record handles, report checks, draft scope, execution epoch.

ALTER TABLE finance_claim_drafts ADD COLUMN IF NOT EXISTS instrument_id TEXT;
ALTER TABLE finance_claim_drafts ADD COLUMN IF NOT EXISTS horizon_start DATE;
ALTER TABLE finance_claim_drafts ADD COLUMN IF NOT EXISTS horizon_end DATE;
ALTER TABLE finance_claim_drafts ADD COLUMN IF NOT EXISTS parse_status TEXT NOT NULL DEFAULT 'succeeded';
ALTER TABLE finance_claim_drafts ADD COLUMN IF NOT EXISTS model_config_version TEXT;
ALTER TABLE finance_claim_drafts ADD COLUMN IF NOT EXISTS protocol TEXT;
ALTER TABLE finance_claim_drafts ADD COLUMN IF NOT EXISTS scope_origin TEXT;

ALTER TABLE finance_research_runs ADD COLUMN IF NOT EXISTS execution_epoch BIGINT NOT NULL DEFAULT 1;
ALTER TABLE finance_research_runs ADD COLUMN IF NOT EXISTS claim_results JSONB;

CREATE TABLE IF NOT EXISTS finance_provider_records (
  id TEXT PRIMARY KEY,
  grant_id TEXT NOT NULL REFERENCES finance_tool_grants(id),
  record_hash TEXT NOT NULL,
  payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (grant_id, record_hash)
);
CREATE INDEX IF NOT EXISTS finance_provider_records_grant ON finance_provider_records (grant_id);

CREATE TABLE IF NOT EXISTS finance_report_checks (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES finance_research_runs(id),
  attempt INTEGER NOT NULL,
  candidate_hash TEXT NOT NULL,
  pointer TEXT NOT NULL,
  claim_type TEXT,
  evidence_ids JSONB,
  numeric_bindings JSONB,
  rule_version TEXT NOT NULL,
  program_result TEXT NOT NULL,
  semantic_result TEXT,
  failure_reason TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS finance_report_checks_run ON finance_report_checks (run_id, attempt);
