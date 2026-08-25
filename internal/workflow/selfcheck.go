package workflow

import (
	"context"
	"fmt"
	"time"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
)

func RunBoundedSelfcheck(ctx context.Context, service *Service) (*CredentialView, error) {
	result, err := service.CreateTrial(ctx, CreateTrialCommand{WriteMeta: WriteMeta{ExpectedVersion: 0, IdempotencyKey: "selfcheck-create-0001"}, ID: "SELF-CHECK-TRIAL", SiteName: "自检古建", WallSection: "东壁画心", SubstrateCondition: "夯土基底、局部酥碱", Owner: "材料研究员", AcceptanceThresholds: domain.Thresholds{MaxColorDifference: 2, MaxShrinkagePct: 1, MinBondStrengthMPa: .3, MaxPowderingGrade: 1}})
	if err != nil {
		return nil, err
	}
	prepared := time.Now().UTC().AddDate(0, 0, -8)
	result, err = service.RegisterPanel(ctx, result.Task.ID, RegisterPanelCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "selfcheck-panel-0001"}, Formula: domain.MortarFormula{ID: "SELF-F-1", Revision: 1, Components: []domain.FormulaComponent{{Name: "熟石灰", Percentage: 70, BatchRef: "LIME-01"}, {Name: "细砂", Percentage: 30, BatchRef: "SAND-01"}}, WaterRatio: .42, MixingMethod: "低速湿拌 5 分钟", PreparedBy: "材料研究员", PreparedAt: prepared, TemperatureC: 21, HumidityPct: 55}, Panel: domain.TestPanel{ID: "SELF-P-1", PanelCode: "SC-P01", CuringStartedAt: prepared, ScheduledCheckpoints: []int{7}}, Actor: "材料研究员"})
	if err != nil {
		return nil, err
	}
	result, err = service.RecordMeasurement(ctx, result.Task.ID, RecordMeasurementCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "selfcheck-measure-01"}, PanelID: "SELF-P-1", Measurement: domain.Measurement{ID: "SELF-M-1", CheckpointDay: 7, ColorDifference: 2.6, ShrinkagePct: .8, BondStrengthMPa: .4, PowderingGrade: 1, Observation: "色差偏高，其余稳定", MeasuredBy: "检验员"}})
	if err != nil {
		return nil, err
	}
	deviation := result.Task.Panels[0].Deviations[0]
	result, err = service.ReviewDeviation(ctx, result.Task.ID, ReviewDeviationCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "selfcheck-review-001"}, PanelID: "SELF-P-1", DeviationID: deviation.ID, Conclusion: "remediation_required", Reviewer: "保护责任人", Note: "调整砂源综合色相后复验"})
	if err != nil {
		return nil, err
	}
	due := time.Now().UTC().AddDate(0, 0, 3).Format("2006-01-02")
	result, err = service.AddRemediation(ctx, result.Task.ID, RemediationCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "selfcheck-plan-00001"}, PanelID: "SELF-P-1", Plan: domain.RemediationPlan{ID: "SELF-R-1", DeviationIDs: []string{deviation.ID}, Action: "更换同产地低色度细砂并制作复验面", DueDate: due, Responsible: "材料研究员"}, Actor: "保护责任人"})
	if err != nil {
		return nil, err
	}
	result, err = service.RecordRetest(ctx, result.Task.ID, RetestCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "selfcheck-retest-001"}, PanelID: "SELF-P-1", PlanID: "SELF-R-1", Measurement: domain.Measurement{ID: "SELF-M-R1", CheckpointDay: 7, ColorDifference: 1.4, ShrinkagePct: .7, BondStrengthMPa: .42, PowderingGrade: 0, Observation: "整改后综合色差与表面状态合格", MeasuredBy: "复验员"}})
	if err != nil {
		return nil, err
	}
	eligibility, err := service.PreviewEligibility(ctx, result.Task.ID, "SELF-F-1", "SELF-P-1", "现场施工负责人")
	if err != nil {
		return nil, err
	}
	result, err = service.Freeze(ctx, result.Task.ID, FreezeCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "selfcheck-freeze-01"}, FormulaID: "SELF-F-1", PanelID: "SELF-P-1", ApprovedBy: "现场施工负责人", EligibilityDigest: eligibility.Digest})
	if err != nil {
		return nil, err
	}
	result, err = service.Release(ctx, result.Task.ID, ReleaseCommand{WriteMeta: WriteMeta{ExpectedVersion: result.Task.Version, IdempotencyKey: "selfcheck-release-1"}, ApprovedBy: "现场施工负责人"})
	if err != nil {
		return nil, err
	}
	if result.Credential == nil {
		return nil, fmt.Errorf("自检签发响应缺少凭据")
	}
	view, err := service.VerifyCredential(ctx, result.Credential.CredentialNo)
	if err != nil {
		return nil, err
	}
	if !view.DigestValid || view.TaskStatus != domain.StatusReleased {
		return nil, fmt.Errorf("凭据摘要或任务状态核验失败")
	}
	return view, nil
}
