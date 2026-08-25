package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/ledger"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/policy"
)

func extensionService(t *testing.T, now *time.Time) *Service {
	t.Helper()
	store, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := NewService(store)
	service.now = func() time.Time { return *now }
	return service
}

func createExtensionTrial(t *testing.T, service *Service, id string) *ActionResult {
	t.Helper()
	result, err := service.CreateTrial(context.Background(), CreateTrialCommand{WriteMeta: WriteMeta{IdempotencyKey: "create-" + id}, ID: id, SiteName: "壁画", WallSection: "东壁", SubstrateCondition: "夯土", Owner: "负责人", AcceptanceThresholds: domain.Thresholds{MaxColorDifference: 2, MaxShrinkagePct: 1, MinBondStrengthMPa: .3, MaxPowderingGrade: 1}})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func extensionFormula(id string, prepared time.Time) domain.MortarFormula {
	return domain.MortarFormula{ID: id, Revision: 1, Components: []domain.FormulaComponent{{Name: "石灰", Percentage: 70, BatchRef: "L-1"}, {Name: "砂", Percentage: 30, BatchRef: "S-1"}}, WaterRatio: .4, MixingMethod: "低速湿拌", PreparedBy: "研究员", PreparedAt: prepared, TemperatureC: 20, HumidityPct: 55}
}

func goodMeasurement(day int, by string) domain.Measurement {
	return domain.Measurement{CheckpointDay: day, ColorDifference: 1, ShrinkagePct: .5, BondStrengthMPa: .5, PowderingGrade: 0, MeasuredBy: by, Observation: "表面稳定"}
}

func TestParallelPanelRegistrationIsAtomicAndIdempotent(t *testing.T) {
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	service := extensionService(t, &now)
	result := createExtensionTrial(t, service, "GROUP-1")
	panels := []domain.TestPanel{
		{PanelCode: "P-101", CuringStartedAt: now.AddDate(0, 0, -8), ScheduledCheckpoints: []int{7}},
		{PanelCode: "P-102", CuringStartedAt: now.AddDate(0, 0, -8), ScheduledCheckpoints: []int{7}},
		{PanelCode: "P-103", CuringStartedAt: now.AddDate(0, 0, -8), ScheduledCheckpoints: []int{7}},
	}
	cmd := RegisterPanelCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "parallel-group-0001"}, Formula: extensionFormula("F-01", now.Add(-time.Hour)), Panels: panels, Actor: "研究员"}
	result, err := service.RegisterPanel(context.Background(), result.Task.ID, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if result.Task.Version != 2 || len(result.Task.Formulas) != 1 || len(result.Task.Panels) != 3 {
		t.Fatalf("整组登记未以单版本提交: v%d formulas=%d panels=%d", result.Task.Version, len(result.Task.Formulas), len(result.Task.Panels))
	}
	replayed, err := service.RegisterPanel(context.Background(), result.Task.ID, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Task.Version != 2 || len(replayed.Task.Panels) != 3 || len(replayed.Task.Audit) != len(result.Task.Audit) {
		t.Fatal("幂等重试产生了重复试板或审计")
	}
	invalid := RegisterPanelCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "parallel-group-0002"}, FormulaID: "F-01", Panels: []domain.TestPanel{{PanelCode: " p-102 ", CuringStartedAt: now, ScheduledCheckpoints: []int{7}}, {PanelCode: "P-104", CuringStartedAt: now, ScheduledCheckpoints: []int{14, 7}}}, Actor: "研究员"}
	_, err = service.RegisterPanel(context.Background(), result.Task.ID, invalid)
	var validation *policy.ValidationError
	if !errors.As(err, &validation) || len(validation.Violations) < 2 {
		t.Fatalf("未返回逐行错误: %v", err)
	}
	unchanged, _ := service.GetTrial(context.Background(), result.Task.ID)
	if unchanged.Version != 2 || len(unchanged.Panels) != 3 || len(unchanged.Audit) != len(result.Task.Audit) {
		t.Fatal("无效整组请求改变了聚合")
	}
}

func TestMeasurementBatchRejectsAllRowsAndThenCommitsOnce(t *testing.T) {
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	service := extensionService(t, &now)
	result := createExtensionTrial(t, service, "BATCH-1")
	result, err := service.RegisterPanel(context.Background(), result.Task.ID, RegisterPanelCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "batch-panels-0001"}, Formula: extensionFormula("F-01", now), Panels: []domain.TestPanel{{ID: "P1", PanelCode: "P-1", CuringStartedAt: now.AddDate(0, 0, -8), ScheduledCheckpoints: []int{7}}, {ID: "P2", PanelCode: "P-2", CuringStartedAt: now, ScheduledCheckpoints: []int{7, 14}}}, Actor: "研究员"})
	if err != nil {
		t.Fatal(err)
	}
	bad := RecordMeasurementCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "batch-measure-0001"}, Measurements: []MeasurementRecord{{PanelID: "P1", Measurement: goodMeasurement(7, "甲")}, {PanelID: "P2", Measurement: goodMeasurement(14, "乙")}}}
	_, err = service.RecordMeasurement(context.Background(), result.Task.ID, bad)
	var validation *policy.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("批量无效记录未返回定位错误: %v", err)
	}
	unchanged, _ := service.GetTrial(context.Background(), result.Task.ID)
	if unchanged.Version != result.Task.Version || len(unchanged.Panels[0].Measurements) != 0 || len(unchanged.Panels[1].Measurements) != 0 {
		t.Fatal("无效批次形成了部分测量")
	}
	now = now.AddDate(0, 0, 8)
	good := RecordMeasurementCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "batch-measure-0002"}, Measurements: []MeasurementRecord{{PanelID: "P1", Measurement: goodMeasurement(7, "甲")}, {PanelID: "P2", Measurement: goodMeasurement(7, "乙")}}}
	result, err = service.RecordMeasurement(context.Background(), result.Task.ID, good)
	if err != nil {
		t.Fatal(err)
	}
	if result.Task.Version != 3 || len(result.Task.Panels[0].Measurements) != 1 || len(result.Task.Panels[1].Measurements) != 1 || result.Task.Status != domain.StatusPendingApproval {
		t.Fatalf("合法批次提交异常: v%d status=%s", result.Task.Version, result.Task.Status)
	}
	view, err := service.GetTrialViewForApprover(context.Background(), result.Task.ID, "施工负责人")
	if err != nil || len(view.EligibilityMatrix) != 2 || !view.EligibilityMatrix[0].Eligible || view.EligibilityMatrix[1].Eligible || len(view.EligibilityMatrix[1].Reasons) == 0 {
		t.Fatalf("冻结候选矩阵未区分完整与缺证据试板: %+v", view.EligibilityMatrix)
	}
}

func TestFailedRetestCreatesNextRoundAndFreezeBindsDigest(t *testing.T) {
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	service := extensionService(t, &now)
	result := createExtensionTrial(t, service, "RETEST-1")
	result, _ = service.RegisterPanel(context.Background(), result.Task.ID, RegisterPanelCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "retest-panel-0001"}, Formula: extensionFormula("F-01", now), Panel: domain.TestPanel{ID: "P1", PanelCode: "P-1", CuringStartedAt: now.AddDate(0, 0, -8), ScheduledCheckpoints: []int{7}}, Actor: "研究员"})
	failed := goodMeasurement(7, "检验员")
	failed.ColorDifference = 3
	result, _ = service.RecordMeasurement(context.Background(), result.Task.ID, RecordMeasurementCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "retest-measure-001"}, PanelID: "P1", Measurement: failed})
	deviationID := result.Task.Panels[0].Deviations[0].ID
	result, _ = service.ReviewDeviation(context.Background(), result.Task.ID, ReviewDeviationCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "retest-review-0001"}, PanelID: "P1", DeviationID: deviationID, Conclusion: "remediation_required", Reviewer: "保护人", Note: "调整综合色相"})
	due := now.AddDate(0, 0, 2).Format("2006-01-02")
	result, _ = service.AddRemediation(context.Background(), result.Task.ID, RemediationCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "retest-plan-000001"}, PanelID: "P1", Plan: domain.RemediationPlan{ID: "R1", DeviationIDs: []string{deviationID}, Action: "更换砂源", DueDate: due, Responsible: "研究员"}, Actor: "保护人"})
	retestFailed := goodMeasurement(0, "复验员")
	retestFailed.ColorDifference = 2.8
	result, err := service.RecordRetest(context.Background(), result.Task.ID, RetestCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "retest-failed-0001"}, PanelID: "P1", PlanID: "R1", Measurement: retestFailed})
	if err != nil {
		t.Fatal(err)
	}
	panel := result.Task.Panels[0]
	if result.RetestOutcome == nil || result.RetestOutcome.Passed || panel.RetestRound != 1 || panel.Remediations[0].Outcome != "failed" || len(result.RetestOutcome.NewDeviations) != 1 || result.Task.Status != domain.StatusRemediation {
		t.Fatalf("失败复验未形成连续轮次: %+v", result.RetestOutcome)
	}
	newDeviation := result.RetestOutcome.NewDeviations[0]
	result, _ = service.AddRemediation(context.Background(), result.Task.ID, RemediationCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "retest-plan-000002"}, PanelID: "P1", Plan: domain.RemediationPlan{ID: "R2", DeviationIDs: []string{newDeviation.ID}, Action: "再次调整砂源", DueDate: due, Responsible: "研究员"}, Actor: "保护人"})
	result, err = service.RecordRetest(context.Background(), result.Task.ID, RetestCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "retest-passed-0001"}, PanelID: "P1", PlanID: "R2", Measurement: goodMeasurement(0, "复验员")})
	if err != nil || result.Task.Status != domain.StatusPendingApproval || len(result.Task.Panels[0].Measurements) != 3 {
		t.Fatalf("第二轮合格未闭环或历史证据丢失: %v", err)
	}
	report, _ := service.PreviewEligibility(context.Background(), result.Task.ID, "F-01", "P1", "施工负责人甲")
	if !report.Eligible || report.Digest == "" {
		t.Fatalf("未生成可冻结摘要: %+v", report)
	}
	conflictView, _ := service.GetTrialViewForApprover(context.Background(), result.Task.ID, "负责人")
	if len(conflictView.EligibilityMatrix) != 1 || conflictView.EligibilityMatrix[0].Status != "duty_conflict" || conflictView.EligibilityMatrix[0].Eligible {
		t.Fatalf("职责冲突未在预检矩阵中定位: %+v", conflictView.EligibilityMatrix)
	}
	before := result.Task.Version
	_, err = service.Freeze(context.Background(), result.Task.ID, FreezeCommand{WriteMeta: WriteMeta{ExpectedVersion: before, IdempotencyKey: "freeze-stale-0001"}, FormulaID: "F-01", PanelID: "P1", ApprovedBy: "施工负责人乙", EligibilityDigest: report.Digest})
	if err == nil {
		t.Fatal("不同批准人复用了旧预检摘要")
	}
	unchanged, _ := service.GetTrial(context.Background(), result.Task.ID)
	if unchanged.Version != before || unchanged.FrozenBatch != nil {
		t.Fatal("过期预检摘要改变了任务")
	}
	result, err = service.Freeze(context.Background(), result.Task.ID, FreezeCommand{WriteMeta: WriteMeta{ExpectedVersion: before, IdempotencyKey: "freeze-valid-0001"}, FormulaID: "F-01", PanelID: "P1", ApprovedBy: "施工负责人甲", EligibilityDigest: report.Digest})
	if err != nil || result.Task.FrozenBatch == nil || result.Task.FrozenBatch.EligibilityDigest != report.Digest || len(result.Task.FrozenBatch.EligibilityChecks) == 0 {
		t.Fatalf("冻结快照未固化预检证据: %v", err)
	}
}
