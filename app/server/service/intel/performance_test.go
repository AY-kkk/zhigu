package intel

import (
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestTwoHundredEvidenceRecalculationWithin200ms(t *testing.T) {
	evidence := make([]EvidenceRule, 0, 200)
	for i := 0; i < 200; i++ {
		evidence = append(evidence, EvidenceRule{
			ID: fmt.Sprintf("ev-%03d", i), ClaimKey: "transaction_amount",
			Value: fmt.Sprintf("%d", i+1), SourceCluster: fmt.Sprintf("root-%03d", i),
			SourceWeight: decimal.RequireFromString("1.0"), Grade: GradeFact,
			Stance: StanceSupport, Status: EvidenceActive,
		})
	}
	start := time.Now()
	for i := 0; i < 10; i++ {
		_ = EvaluateClaim(evidence)
	}
	elapsed := time.Since(start) / 10
	if elapsed > 200*time.Millisecond {
		t.Fatalf("200-evidence recalculation took %s", elapsed)
	}
}
