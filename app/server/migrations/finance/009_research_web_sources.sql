ALTER TABLE finance_research_documents DROP CONSTRAINT IF EXISTS finance_research_documents_media_type_check;
ALTER TABLE finance_research_documents
  ADD CONSTRAINT finance_research_documents_media_type_check
  CHECK (media_type IN ('application/pdf','application/vnd.openxmlformats-officedocument.wordprocessingml.document','text/plain','text/html'));

ALTER TABLE finance_research_documents ADD COLUMN IF NOT EXISTS origin_type TEXT NOT NULL DEFAULT 'upload';
ALTER TABLE finance_research_documents ADD COLUMN IF NOT EXISTS source_url TEXT NOT NULL DEFAULT '';
ALTER TABLE finance_research_documents ADD COLUMN IF NOT EXISTS canonical_url TEXT NOT NULL DEFAULT '';
ALTER TABLE finance_research_documents ADD COLUMN IF NOT EXISTS source_domain TEXT NOT NULL DEFAULT '';
ALTER TABLE finance_research_documents ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT '';
ALTER TABLE finance_research_documents ADD COLUMN IF NOT EXISTS fetch_status TEXT NOT NULL DEFAULT 'succeeded';
ALTER TABLE finance_research_documents ADD COLUMN IF NOT EXISTS http_status INTEGER;
ALTER TABLE finance_research_documents ADD COLUMN IF NOT EXISTS fetched_at TIMESTAMPTZ;

ALTER TABLE finance_research_documents
  ADD CONSTRAINT finance_research_documents_origin_type_check
  CHECK (origin_type IN ('upload','url'));

ALTER TABLE finance_research_documents
  ADD CONSTRAINT finance_research_documents_fetch_status_check
  CHECK (fetch_status IN ('queued','succeeded','failed'));

ALTER TABLE finance_evidence DROP CONSTRAINT IF EXISTS finance_evidence_source_kind_check;
ALTER TABLE finance_evidence
  ADD CONSTRAINT finance_evidence_source_kind_check
  CHECK (source_kind IN ('filing','financials','market','news','fixture','user_report','web_article'));

ALTER TABLE finance_evidence DROP CONSTRAINT IF EXISTS finance_evidence_source_grade_check;
ALTER TABLE finance_evidence
  ADD CONSTRAINT finance_evidence_source_grade_check
  CHECK (source_grade IN ('official_filing','structured_data','user_report','external_web','fixture'));
