package finance

import "time"

type FixtureConfig struct{}

func NewFixtureConfig() *FixtureConfig { return &FixtureConfig{} }

type FrozenVersions struct {
	Model  string
	Source string
	Prompt string
	Policy string
	Budget string
}

func (FixtureConfig) Active() FrozenVersions {
	return FrozenVersions{
		Model:  "model_fixture_v1",
		Source: "source_fixture_v1",
		Prompt: "prompt_v1",
		Policy: "policy_v1",
		Budget: "budget_v1",
	}
}

func DefaultAsOf(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func SuggestedHorizon(asOf time.Time) string {
	end := asOf.AddDate(1, 0, 0)
	return asOf.Format("2006-01-02") + "/" + end.Format("2006-01-02")
}

func HorizonDates(asOf time.Time) (string, string) {
	end := asOf.AddDate(1, 0, 0)
	return asOf.Format("2006-01-02"), end.Format("2006-01-02")
}
