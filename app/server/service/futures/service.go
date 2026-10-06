// Package futures owns commodity research. It must not create finance ResearchRuns.
package futures

// Capabilities is the data payload of GET /api/v1/futures/capabilities.
// The scaffold deliberately fails closed, even when enabled in configuration.
type Capabilities struct {
	SchemaVersion string   `json:"schema_version"`
	Enabled       bool     `json:"enabled"`
	Ready         bool     `json:"ready"`
	Mode          string   `json:"mode"`
	Reason        string   `json:"reason"`
	Products      []string `json:"products"`
	Features      Features `json:"features"`
	Limits        Limits   `json:"limits"`
}

type Limits struct {
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

type Features struct {
	Research bool `json:"research"`
	Tracking bool `json:"tracking"`
	Export   bool `json:"export"`
}

type Service struct {
	enabled bool
	state   RuntimeState
	domain  *Domain
}

func (s *Service) WithDomain(domain *Domain) *Service { s.domain = domain; return s }
func (s *Service) Domain() *Domain                    { return s.domain }

// NewService performs no migration, network request, goroutine or model call.
func NewService(enabled bool) *Service { return &Service{enabled: enabled} }

// WithRuntimeState injects the bootstrap result without making NewService perform I/O.
func (s *Service) WithRuntimeState(state RuntimeState) *Service {
	s.state = state
	if state.Enabled != s.enabled {
		s.enabled = state.Enabled
	}
	return s
}

func (s *Service) RuntimeState() RuntimeState {
	if s.state.Mode == "" {
		return RuntimeState{Enabled: s.enabled, Ready: false, Mode: "off", Reason: "FUTURES_NOT_IMPLEMENTED"}
	}
	return s.state
}

func (s *Service) Capabilities() Capabilities {
	state := s.RuntimeState()
	return Capabilities{
		SchemaVersion: "futures.capabilities.v1", Enabled: state.Enabled,
		Ready: state.Ready, Mode: state.Mode, Reason: state.Reason,
		Products: []string{}, Features: Features{},
		Limits: Limits{DailyRuns: 5, ActiveRuns: 1, ActiveHypotheses: 20,
			ModelCalls: 8, ToolCalls: 24, Tokens: 48000, UserDailyCNY: "5", ModuleDailyCNY: "50",
			QueueTimeoutSeconds: 60, ExecutionTimeoutSeconds: 180},
	}
}
