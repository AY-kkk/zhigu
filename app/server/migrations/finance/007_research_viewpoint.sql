ALTER TABLE finance_claim_drafts ADD COLUMN IF NOT EXISTS source_mode TEXT NOT NULL DEFAULT 'claim_only';
ALTER TABLE finance_claim_drafts ADD COLUMN IF NOT EXISTS document_id TEXT;
ALTER TABLE finance_claim_drafts ADD COLUMN IF NOT EXISTS focus_text TEXT;

ALTER TABLE finance_claim_drafts
  ADD CONSTRAINT finance_claim_drafts_source_mode_check
  CHECK (source_mode IN ('claim_only','report_only','claim_and_report'));

ALTER TABLE finance_research_runs ADD COLUMN IF NOT EXISTS document_id TEXT;
ALTER TABLE finance_research_runs ADD COLUMN IF NOT EXISTS input_mode TEXT NOT NULL DEFAULT 'claim_only';

ALTER TABLE finance_research_runs
  ADD CONSTRAINT finance_research_runs_input_mode_check
  CHECK (input_mode IN ('claim_only','report_only','claim_and_report'));

CREATE TABLE IF NOT EXISTS finance_research_documents (
  id TEXT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  draft_id TEXT,
  run_id TEXT,
  filename TEXT NOT NULL,
  media_type TEXT NOT NULL CHECK (media_type IN ('application/pdf','application/vnd.openxmlformats-officedocument.wordprocessingml.document','text/plain')),
  byte_size BIGINT NOT NULL CHECK (byte_size > 0 AND byte_size <= 20971520),
  content_hash TEXT NOT NULL,
  storage_key TEXT NOT NULL,
  extraction_status TEXT NOT NULL CHECK (extraction_status IN ('queued','succeeded','failed')),
  extracted_text TEXT,
  extraction_error TEXT,
  page_count INTEGER,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  deleted_at TIMESTAMPTZ,
  UNIQUE(owner_id, content_hash)
);

CREATE INDEX finance_research_documents_owner_created
  ON finance_research_documents(owner_id, created_at DESC);

CREATE TABLE IF NOT EXISTS finance_research_document_spans (
  id TEXT PRIMARY KEY,
  document_id TEXT NOT NULL REFERENCES finance_research_documents(id) ON DELETE CASCADE,
  page_number INTEGER,
  paragraph_index INTEGER,
  start_offset INTEGER NOT NULL,
  end_offset INTEGER NOT NULL,
  text TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  CHECK (start_offset >= 0 AND end_offset > start_offset)
);

CREATE INDEX finance_research_document_spans_document
  ON finance_research_document_spans(document_id, page_number, paragraph_index);

CREATE TABLE IF NOT EXISTS finance_claim_fact_checks (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES finance_research_runs(id) ON DELETE CASCADE,
  claim_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('supported','prerequisite_missing','contradicted','uncertain')),
  reason TEXT NOT NULL,
  evidence_ids JSONB NOT NULL DEFAULT '[]',
  created_at TIMESTAMPTZ NOT NULL,
  UNIQUE(run_id, claim_id)
);
