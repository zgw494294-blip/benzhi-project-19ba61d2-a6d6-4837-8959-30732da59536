package policy

import (
	"testing"
	"time"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
)

func TestFormulaAndMeasurementPolicy(t *testing.T) {
	formula := domain.MortarFormula{ID: "F-1", Revision: 1, Components: []domain.FormulaComponent{{Name: "石灰", Percentage: 70, BatchRef: "L-1"}, {Name: "砂", Percentage: 30, BatchRef: "S-1"}}, WaterRatio: .4, MixingMethod: "低速搅拌", PreparedBy: "研究员", PreparedAt: time.Now(), TemperatureC: 20, HumidityPct: 55}
	if err := ValidateFormula(formula); err != nil {
		t.Fatal(err)
	}
	decisions, err := EvaluateMeasurement(domain.Thresholds{MaxColorDifference: 2, MaxShrinkagePct: 1, MinBondStrengthMPa: .3, MaxPowderingGrade: 1}, domain.Measurement{ColorDifference: 2.5, ShrinkagePct: .5, BondStrengthMPa: .4, PowderingGrade: 0, MeasuredBy: "检测员", Observation: "稳定"})
	if err != nil {
		t.Fatal(err)
	}
	if decisions[0].Passed || !decisions[1].Passed {
		t.Fatalf("指标判定异常: %+v", decisions)
	}
}
