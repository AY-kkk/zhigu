-- Strategy workbench / market domain. Do not alter research report schema.

CREATE TABLE IF NOT EXISTS finance_market_catalog_releases (
  catalog_version TEXT PRIMARY KEY,
  catalog_as_of DATE NOT NULL,
  freshness_status TEXT NOT NULL,
  source_id TEXT NOT NULL,
  source_contract_version TEXT NOT NULL,
  instrument_count INTEGER NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}',
  published_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS finance_market_pointers (
  name TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS finance_market_instruments (
  instrument_id TEXT PRIMARY KEY,
  security_id TEXT NOT NULL,
  exchange TEXT NOT NULL,
  board TEXT NOT NULL,
  asset_type TEXT NOT NULL,
  code TEXT NOT NULL,
  name TEXT NOT NULL,
  name_en TEXT NOT NULL DEFAULT '',
  aliases JSONB NOT NULL DEFAULT '[]',
  pinyin TEXT NOT NULL DEFAULT '',
  pinyin_abbr TEXT NOT NULL DEFAULT '',
  currency TEXT NOT NULL,
  calendar_id TEXT NOT NULL,
  listing_date DATE,
  delisting_date DATE,
  trading_status TEXT NOT NULL,
  lot_size INTEGER NOT NULL DEFAULT 100,
  lot_rule_id TEXT NOT NULL,
  tick_rule_id TEXT NOT NULL,
  valid_from DATE NOT NULL,
  valid_to DATE,
  catalog_version TEXT NOT NULL,
  source_payload JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS finance_market_instruments_security ON finance_market_instruments (security_id);
CREATE INDEX IF NOT EXISTS finance_market_instruments_code ON finance_market_instruments (code);
CREATE INDEX IF NOT EXISTS finance_market_instruments_exchange ON finance_market_instruments (exchange, asset_type);
CREATE INDEX IF NOT EXISTS finance_market_instruments_pinyin ON finance_market_instruments (pinyin_abbr);
CREATE INDEX IF NOT EXISTS finance_market_instruments_name ON finance_market_instruments (name);

CREATE TABLE IF NOT EXISTS finance_market_instrument_aliases (
  alias TEXT NOT NULL,
  normalized TEXT NOT NULL,
  instrument_id TEXT NOT NULL REFERENCES finance_market_instruments(instrument_id),
  kind TEXT NOT NULL,
  valid_from DATE NOT NULL,
  valid_to DATE,
  catalog_version TEXT NOT NULL,
  PRIMARY KEY (normalized, instrument_id, kind, valid_from)
);
CREATE INDEX IF NOT EXISTS finance_market_aliases_norm ON finance_market_instrument_aliases (normalized);

CREATE TABLE IF NOT EXISTS finance_market_snapshots (
  id TEXT PRIMARY KEY,
  instrument_id TEXT NOT NULL,
  period TEXT NOT NULL,
  adjust TEXT NOT NULL,
  source_id TEXT NOT NULL,
  source_contract_version TEXT NOT NULL,
  catalog_version TEXT,
  adjustment_version TEXT,
  calendar_version TEXT,
  earliest_date DATE,
  latest_complete_date DATE,
  quality JSONB NOT NULL DEFAULT '{}',
  bars_hash TEXT NOT NULL,
  immutable BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS finance_market_snapshots_inst ON finance_market_snapshots (instrument_id, period, adjust, created_at DESC);

CREATE TABLE IF NOT EXISTS finance_market_bars (
  snapshot_id TEXT NOT NULL REFERENCES finance_market_snapshots(id),
  instrument_id TEXT NOT NULL,
  period TEXT NOT NULL,
  trade_date DATE NOT NULL,
  period_start DATE,
  period_end DATE,
  open_px TEXT NOT NULL,
  high_px TEXT NOT NULL,
  low_px TEXT NOT NULL,
  close_px TEXT NOT NULL,
  volume TEXT NOT NULL,
  is_final BOOLEAN NOT NULL DEFAULT TRUE,
  PRIMARY KEY (snapshot_id, instrument_id, period, trade_date)
);
CREATE INDEX IF NOT EXISTS finance_market_bars_date ON finance_market_bars (instrument_id, period, trade_date);

CREATE TABLE IF NOT EXISTS finance_market_actions (
  id TEXT PRIMARY KEY,
  instrument_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  effective_at DATE NOT NULL,
  available_at DATE,
  record_date DATE,
  pay_date DATE,
  factor TEXT,
  cash_amount TEXT,
  ratio TEXT,
  source_id TEXT NOT NULL,
  version TEXT NOT NULL,
  evidence_level TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS finance_market_actions_inst ON finance_market_actions (instrument_id, effective_at);

CREATE TABLE IF NOT EXISTS finance_market_calendars (
  calendar_id TEXT NOT NULL,
  calendar_version TEXT NOT NULL,
  trade_date DATE NOT NULL,
  session TEXT NOT NULL DEFAULT 'regular',
  is_half_day BOOLEAN NOT NULL DEFAULT FALSE,
  PRIMARY KEY (calendar_id, calendar_version, trade_date)
);

CREATE TABLE IF NOT EXISTS finance_market_rules (
  id TEXT PRIMARY KEY,
  exchange TEXT NOT NULL,
  board TEXT NOT NULL,
  rule_kind TEXT NOT NULL,
  effective_from DATE NOT NULL,
  effective_to DATE,
  version TEXT NOT NULL,
  source TEXT NOT NULL,
  payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS finance_market_rules_lookup ON finance_market_rules (exchange, board, rule_kind, effective_from);

CREATE TABLE IF NOT EXISTS finance_strategy_workspace (
  owner_id INTEGER PRIMARY KEY REFERENCES finance_users(id),
  revision INTEGER NOT NULL DEFAULT 1,
  watchlist JSONB NOT NULL DEFAULT '[]',
  last_instrument_id TEXT,
  layout JSONB NOT NULL DEFAULT '{}',
  chart_indicators JSONB NOT NULL DEFAULT '[]',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS finance_strategy_drafts (
  id TEXT PRIMARY KEY,
  owner_id INTEGER NOT NULL REFERENCES finance_users(id),
  text TEXT NOT NULL,
  instrument_id TEXT,
  status TEXT NOT NULL,
  revision INTEGER NOT NULL DEFAULT 1,
  generation_id TEXT,
  dsl JSONB,
  assumptions JSONB NOT NULL DEFAULT '[]',
  capability_errors JSONB NOT NULL DEFAULT '[]',
  clarification JSONB,
  compiled JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS finance_strategy_drafts_owner ON finance_strategy_drafts (owner_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS finance_strategy_generations (
  id TEXT PRIMARY KEY,
  owner_id INTEGER NOT NULL REFERENCES finance_users(id),
  draft_id TEXT NOT NULL REFERENCES finance_strategy_drafts(id),
  status TEXT NOT NULL,
  text TEXT NOT NULL,
  instrument_id TEXT,
  base_revision INTEGER,
  model_config_version TEXT,
  prompt_version TEXT,
  protocol TEXT,
  usage JSONB,
  error_code TEXT,
  error_message TEXT,
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  lease_owner TEXT,
  lease_until TIMESTAMPTZ,
  execution_epoch INTEGER NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (owner_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS finance_strategy_generations_draft ON finance_strategy_generations (draft_id, created_at DESC);

CREATE TABLE IF NOT EXISTS finance_strategies (
  id TEXT PRIMARY KEY,
  owner_id INTEGER NOT NULL REFERENCES finance_users(id),
  name TEXT NOT NULL,
  current_version_id TEXT,
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS finance_strategies_owner ON finance_strategies (owner_id, created_at DESC);

CREATE TABLE IF NOT EXISTS finance_strategy_versions (
  id TEXT PRIMARY KEY,
  strategy_id TEXT NOT NULL REFERENCES finance_strategies(id),
  owner_id INTEGER NOT NULL REFERENCES finance_users(id),
  revision INTEGER NOT NULL,
  name TEXT NOT NULL,
  dsl JSONB NOT NULL,
  dsl_hash TEXT NOT NULL,
  compiled JSONB NOT NULL,
  compiler_version TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (strategy_id, revision)
);

CREATE TABLE IF NOT EXISTS finance_backtest_runs (
  id TEXT PRIMARY KEY,
  owner_id INTEGER NOT NULL REFERENCES finance_users(id),
  strategy_id TEXT NOT NULL,
  strategy_version_id TEXT NOT NULL,
  status TEXT NOT NULL,
  progress JSONB NOT NULL DEFAULT '{}',
  config JSONB NOT NULL,
  config_hash TEXT NOT NULL,
  manifest JSONB,
  result_summary JSONB,
  result_hash TEXT,
  error_code TEXT,
  error_message TEXT,
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  lease_owner TEXT,
  lease_until TIMESTAMPTZ,
  execution_epoch INTEGER NOT NULL DEFAULT 1,
  heartbeat_at TIMESTAMPTZ,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (owner_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS finance_backtest_runs_owner ON finance_backtest_runs (owner_id, created_at DESC);
CREATE INDEX IF NOT EXISTS finance_backtest_runs_status ON finance_backtest_runs (status, lease_until);

CREATE TABLE IF NOT EXISTS finance_backtest_orders (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES finance_backtest_runs(id),
  signal_id TEXT,
  side TEXT NOT NULL,
  status TEXT NOT NULL,
  qty TEXT NOT NULL,
  submitted_date DATE,
  fill_date DATE,
  reason TEXT,
  payload JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS finance_backtest_orders_run ON finance_backtest_orders (run_id, submitted_date);

CREATE TABLE IF NOT EXISTS finance_backtest_fills (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES finance_backtest_runs(id),
  order_id TEXT NOT NULL REFERENCES finance_backtest_orders(id),
  fill_date DATE NOT NULL,
  qty TEXT NOT NULL,
  price TEXT NOT NULL,
  fees JSONB NOT NULL DEFAULT '[]',
  cash_delta TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS finance_backtest_fills_run ON finance_backtest_fills (run_id, fill_date);

CREATE TABLE IF NOT EXISTS finance_backtest_equity (
  run_id TEXT NOT NULL REFERENCES finance_backtest_runs(id),
  trade_date DATE NOT NULL,
  cash TEXT NOT NULL,
  position_qty TEXT NOT NULL,
  market_value TEXT NOT NULL,
  equity TEXT NOT NULL,
  drawdown TEXT,
  PRIMARY KEY (run_id, trade_date)
);

CREATE TABLE IF NOT EXISTS finance_backtest_results (
  run_id TEXT PRIMARY KEY REFERENCES finance_backtest_runs(id),
  metrics JSONB NOT NULL,
  signals JSONB NOT NULL DEFAULT '[]',
  assumptions JSONB NOT NULL DEFAULT '[]',
  quality JSONB NOT NULL DEFAULT '{}',
  result_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS finance_strategy_idempotency (
  owner_id INTEGER NOT NULL,
  operation TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  object_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (owner_id, operation, idempotency_key)
);
