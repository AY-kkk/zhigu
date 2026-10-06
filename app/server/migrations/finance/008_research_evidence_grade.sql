ALTER TABLE finance_evidence ADD COLUMN IF NOT EXISTS source_grade TEXT NOT NULL DEFAULT 'structured_data';
ALTER TABLE finance_evidence ADD COLUMN IF NOT EXISTS verification_status TEXT NOT NULL DEFAULT 'independent_verified';

ALTER TABLE finance_evidence DROP CONSTRAINT IF EXISTS finance_evidence_source_kind_check;
ALTER TABLE finance_evidence
  ADD CONSTRAINT finance_evidence_source_kind_check
  CHECK (source_kind IN ('filing','financials','market','news','fixture','user_report'));

ALTER TABLE finance_evidence
  ADD CONSTRAINT finance_evidence_source_grade_check
  CHECK (source_grade IN ('official_filing','structured_data','user_report','fixture'));

ALTER TABLE finance_evidence
  ADD CONSTRAINT finance_evidence_verification_status_check
  CHECK (verification_status IN ('independent_verified','reported_only'));
