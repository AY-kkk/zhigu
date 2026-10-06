package futuresmodels

import "time"

type Capabilities struct {
	SchemaVersion string   `json:"schema_version"`
	Enabled       bool     `json:"enabled"`
	Ready         bool     `json:"ready"`
	Mode          string   `json:"mode"`
	Reason        string   `json:"reason"`
	Products      []string `json:"products"`
	Features      struct {
		Research bool `json:"research"`
		Tracking bool `json:"tracking"`
		Export   bool `json:"export"`
	} `json:"features"`
	Limits PublicLimits `json:"limits"`
}

type PublicLimits struct {
	DailyRuns               int    `json:"daily_runs"`
	ActiveRuns              int    `json:"active_runs"`
	ActiveHypotheses        int    `json:"active_hypotheses"`
	ModelCalls              int    `json:"model_calls"`
	ToolCalls               int    `json:"tool_calls"`
	Tokens                  int    `json:"tokens"`
	UserDailyCNY            string `json:"user_daily_cny"`
	ModuleDailyCNY          string `json:"module_daily_cny"`
	QueueTimeoutSeconds     int    `json:"queue_timeout_seconds"`
	ExecutionTimeoutSeconds int    `json:"execution_timeout_seconds"`
}

type Product struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Exchange        string `json:"exchange"`
	TemplateVersion string `json:"template_version"`
	Admission       string `json:"admission"`
}

type Contract struct {
	ID              string    `json:"id"`
	ProductID       string    `json:"product_id"`
	Kind            string    `json:"kind"`
	LastTradingAt   time.Time `json:"last_trading_at"`
	PricePrecision  int       `json:"price_precision"`
	PriceUnit       string    `json:"price_unit"`
	Multiplier      string    `json:"multiplier"`
	CalendarVersion string    `json:"calendar_version"`
	SourceVersion   string    `json:"source_version"`
}

type ClaimsEnvelope struct {
	Items      []map[string]any `json:"items"`
	NextCursor *string          `json:"next_cursor"`
}
