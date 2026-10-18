CREATE TABLE finance_backup_epochs (
  id TEXT PRIMARY KEY,
  epoch TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL CHECK (status IN ('active','succeeded','failed')),
  started_at TIMESTAMPTZ NOT NULL,
  ended_at TIMESTAMPTZ,
  CHECK ((status = 'active' AND ended_at IS NULL) OR (status <> 'active' AND ended_at IS NOT NULL))
);

CREATE INDEX finance_backup_epochs_status_started_idx ON finance_backup_epochs(status, started_at DESC);
