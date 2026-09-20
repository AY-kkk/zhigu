ALTER TABLE finance_model_cache ADD COLUMN IF NOT EXISTS run_id TEXT NOT NULL DEFAULT '';
ALTER TABLE finance_model_cache ADD COLUMN IF NOT EXISTS task_id TEXT NOT NULL DEFAULT '';
ALTER TABLE finance_model_cache ADD COLUMN IF NOT EXISTS owner_id INTEGER;
ALTER TABLE finance_model_cache ADD COLUMN IF NOT EXISTS purpose TEXT NOT NULL DEFAULT 'research';
ALTER TABLE finance_model_cache DROP CONSTRAINT IF EXISTS finance_model_cache_pkey;
ALTER TABLE finance_model_cache ADD PRIMARY KEY (run_id, task_id, request_id);
