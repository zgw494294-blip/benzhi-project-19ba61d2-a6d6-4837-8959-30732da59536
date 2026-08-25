package policy

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
)

func finitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}

func ValidateThresholds(t domain.Thresholds) error {
	var failures []FieldViolation
	if !finitePositive(t.MaxColorDifference) || t.MaxColorDifference > 20 {
		failures = append(failures, FieldViolation{"acceptanceThresholds.maxColorDifference", "必须大于 0 且不超过 20"})
	}
	if !finitePositive(t.MaxShrinkagePct) || t.MaxShrinkagePct > 10 {
		failures = append(failures, FieldViolation{"acceptanceThresholds.maxShrinkagePct", "必须大于 0 且不超过 10"})
	}
	if !finitePositive(t.MinBondStrengthMPa) || t.MinBondStrengthMPa > 20 {
		failures = append(failures, FieldViolation{"acceptanceThresholds.minBondStrengthMPa", "必须大于 0 且不超过 20"})
	}
	if t.MaxPowderingGrade < 0 || t.MaxPowderingGrade > 5 {
		failures = append(failures, FieldViolation{"acceptanceThresholds.maxPowderingGrade", "必须为 0 到 5 的等级"})
	}
	if len(failures) > 0 {
		return &ValidationError{Violations: failures}
	}
	return nil
}

func ValidateFormula(formula domain.MortarFormula) error {
	var failures []FieldViolation
	if strings.TrimSpace(formula.ID) == "" {
		failures = append(failures, FieldViolation{"formula.id", "配方编号不能为空"})
	}
	if formula.Revision < 1 {
		failures = append(failures, FieldViolation{"formula.revision", "修订号必须从 1 开始"})
	}
	if len(formula.Components) < 2 {
		failures = append(failures, FieldViolation{"formula.components", "配方至少包含两种组分"})
	}
	seen, total := map[string]bool{}, 0.0
	for i, component := range formula.Components {
		name := strings.ToLower(strings.TrimSpace(component.Name))
		field := fmt.Sprintf("formula.components[%d]", i)
		if name == "" {
			failures = append(failures, FieldViolation{field + ".name", "组分名称不能为空"})
		}
		if seen[name] {
			failures = append(failures, FieldViolation{field + ".name", "组分名称重复"})
		}
		seen[name] = true
		if !finitePositive(component.Percentage) || component.Percentage > 100 {
			failures = append(failures, FieldViolation{field + ".percentage", "比例必须大于 0 且不超过 100"})
		}
		if strings.TrimSpace(component.BatchRef) == "" {
			failures = append(failures, FieldViolation{field + ".batchRef", "原料批次引用不能为空"})
		}
		total += component.Percentage
	}
	if math.Abs(total-100) > 0.01 {
		failures = append(failures, FieldViolation{"formula.components", fmt.Sprintf("组分比例合计必须为 100，当前为 %.3f", total)})
	}
	if !finitePositive(formula.WaterRatio) || formula.WaterRatio > 5 {
		failures = append(failures, FieldViolation{"formula.waterRatio", "水料比必须大于 0 且不超过 5"})
	}
	if strings.TrimSpace(formula.MixingMethod) == "" {
		failures = append(failures, FieldViolation{"formula.mixingMethod", "搅拌方法不能为空"})
	}
	if strings.TrimSpace(formula.PreparedBy) == "" {
		failures = append(failures, FieldViolation{"formula.preparedBy", "制备人不能为空"})
	}
	if formula.PreparedAt.IsZero() {
		failures = append(failures, FieldViolation{"formula.preparedAt", "制备时间不能为空"})
	}
	if formula.TemperatureC < 0 || formula.TemperatureC > 50 {
		failures = append(failures, FieldViolation{"formula.temperatureC", "制备温度必须在 0 到 50 摄氏度"})
	}
	if formula.HumidityPct <= 0 || formula.HumidityPct > 100 {
		failures = append(failures, FieldViolation{"formula.humidityPct", "相对湿度必须大于 0 且不超过 100"})
	}
	if len(failures) > 0 {
		return &ValidationError{Violations: failures}
	}
	return nil
}

func ValidatePanel(panel domain.TestPanel) error {
	var failures []FieldViolation
	if strings.TrimSpace(panel.ID) == "" {
		failures = append(failures, FieldViolation{"panel.id", "试板 ID 不能为空"})
	}
	if strings.TrimSpace(panel.PanelCode) == "" {
		failures = append(failures, FieldViolation{"panel.panelCode", "试板编号不能为空"})
	}
	if panel.CuringStartedAt.IsZero() {
		failures = append(failures, FieldViolation{"panel.curingStartedAt", "养护开始时间不能为空"})
	}
	if len(panel.ScheduledCheckpoints) == 0 {
		failures = append(failures, FieldViolation{"panel.scheduledCheckpoints", "至少设置一个养护检验节点"})
	}
	last := 0
	for i, day := range panel.ScheduledCheckpoints {
		if day <= 0 || day > 365 {
			failures = append(failures, FieldViolation{fmt.Sprintf("panel.scheduledCheckpoints[%d]", i), "节点日必须在 1 到 365 之间"})
		}
		if day <= last {
			failures = append(failures, FieldViolation{"panel.scheduledCheckpoints", "养护节点必须严格递增且不重复"})
		}
		last = day
	}
	if len(failures) > 0 {
		return &ValidationError{Violations: failures}
	}
	return nil
}

const MaxPanelsPerGroup = 20
const MaxMeasurementsPerBatch = 50

func normalizedPanelCode(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func appendPrefixedViolations(target []FieldViolation, err error, prefix string) []FieldViolation {
	validation, ok := err.(*ValidationError)
	if !ok {
		return append(target, FieldViolation{prefix, err.Error()})
	}
	for _, violation := range validation.Violations {
		field := violation.Field
		if dot := strings.Index(field, "."); dot >= 0 {
			field = field[dot+1:]
		}
		target = append(target, FieldViolation{prefix + "." + field, violation.Message})
	}
	return target
}

// ValidatePanelGroup 在聚合改变前定位整组逐行错误，包括请求内和任务内的规范化编号冲突。
func ValidatePanelGroup(task *domain.TrialTask, formulaID string, requireExistingFormula bool, panels []domain.TestPanel) error {
	var failures []FieldViolation
	if task.Status != domain.StatusDraft && task.Status != domain.StatusCuring {
		failures = append(failures, FieldViolation{"panels", "仅草拟或养护中任务可登记新试板"})
	}
	if strings.TrimSpace(formulaID) == "" || requireExistingFormula && task.Formula(formulaID) == nil {
		failures = append(failures, FieldViolation{"formulaId", "被引用配方必须属于当前任务"})
	}
	if len(panels) == 0 || len(panels) > MaxPanelsPerGroup {
		failures = append(failures, FieldViolation{"panels", fmt.Sprintf("平行试板数量必须为 1 到 %d", MaxPanelsPerGroup)})
	}
	seen := map[string]string{}
	seenIDs := map[string]bool{}
	for _, panel := range task.Panels {
		seen[normalizedPanelCode(panel.PanelCode)] = panel.PanelCode
		seenIDs[panel.ID] = true
	}
	for i, panel := range panels {
		prefix := fmt.Sprintf("panels[%d]", i)
		if err := ValidatePanel(panel); err != nil {
			failures = appendPrefixedViolations(failures, err, prefix)
		}
		if panel.TaskID != "" && panel.TaskID != task.ID {
			failures = append(failures, FieldViolation{prefix + ".taskId", "试板不得关联其他任务"})
		}
		if panel.FormulaID != "" && panel.FormulaID != formulaID {
			failures = append(failures, FieldViolation{prefix + ".formulaId", "试板不得绕过所选配方归属"})
		}
		code := normalizedPanelCode(panel.PanelCode)
		if existing, ok := seen[code]; code != "" && ok {
			failures = append(failures, FieldViolation{prefix + ".panelCode", "试板编号与 " + existing + " 重复（忽略大小写及首尾空白）"})
		} else if code != "" {
			seen[code] = strings.TrimSpace(panel.PanelCode)
		}
		if seenIDs[panel.ID] {
			failures = append(failures, FieldViolation{prefix + ".id", "试板稳定身份在任务内重复"})
		} else if panel.ID != "" {
			seenIDs[panel.ID] = true
		}
	}
	if len(failures) > 0 {
		return &ValidationError{Violations: failures}
	}
	return nil
}

func EvaluateMeasurement(t domain.Thresholds, m domain.Measurement) ([]domain.MetricDecision, error) {
	var failures []FieldViolation
	values := []struct {
		field string
		value float64
		max   float64
	}{{"colorDifference", m.ColorDifference, 100}, {"shrinkagePct", m.ShrinkagePct, 100}, {"bondStrengthMPa", m.BondStrengthMPa, 100}}
	for _, item := range values {
		if math.IsNaN(item.value) || math.IsInf(item.value, 0) || item.value < 0 || item.value > item.max {
			failures = append(failures, FieldViolation{"measurement." + item.field, fmt.Sprintf("测量值必须为 0 到 %.0f 的有限数", item.max)})
		}
	}
	if m.PowderingGrade < 0 || m.PowderingGrade > 5 {
		failures = append(failures, FieldViolation{"measurement.powderingGrade", "粉化等级必须为 0 到 5"})
	}
	if strings.TrimSpace(m.MeasuredBy) == "" {
		failures = append(failures, FieldViolation{"measurement.measuredBy", "测量人不能为空"})
	}
	if strings.TrimSpace(m.Observation) == "" {
		failures = append(failures, FieldViolation{"measurement.observation", "观察备注不能为空"})
	}
	if len(failures) > 0 {
		return nil, &ValidationError{Violations: failures}
	}
	return []domain.MetricDecision{
		{Metric: "colorDifference", Value: m.ColorDifference, Threshold: t.MaxColorDifference, Operator: "<=", Passed: m.ColorDifference <= t.MaxColorDifference},
		{Metric: "shrinkagePct", Value: m.ShrinkagePct, Threshold: t.MaxShrinkagePct, Operator: "<=", Passed: m.ShrinkagePct <= t.MaxShrinkagePct},
		{Metric: "bondStrengthMPa", Value: m.BondStrengthMPa, Threshold: t.MinBondStrengthMPa, Operator: ">=", Passed: m.BondStrengthMPa >= t.MinBondStrengthMPa},
		{Metric: "powderingGrade", Value: float64(m.PowderingGrade), Threshold: float64(t.MaxPowderingGrade), Operator: "<=", Passed: m.PowderingGrade <= t.MaxPowderingGrade},
	}, nil
}

type MeasurementInput struct {
	PanelID     string
	Measurement domain.Measurement
}

func nextCheckpoint(panel *domain.TestPanel) (int, bool) {
	confirmed := map[int]bool{}
	for _, measurement := range panel.Measurements {
		if measurement.Round == 0 && measurement.Confirmed {
			confirmed[measurement.CheckpointDay] = true
		}
	}
	for _, day := range panel.ScheduledCheckpoints {
		if !confirmed[day] {
			return day, true
		}
	}
	return 0, false
}

func CheckpointDueAt(panel *domain.TestPanel, day int) time.Time {
	return panel.CuringStartedAt.UTC().Add(time.Duration(day) * 24 * time.Hour)
}

// ValidateMeasurementBatch 先验证所有行，再返回与输入顺序一致的逐指标判定。
func ValidateMeasurementBatch(task *domain.TrialTask, inputs []MeasurementInput, now time.Time) ([][]domain.MetricDecision, error) {
	var failures []FieldViolation
	decisions := make([][]domain.MetricDecision, len(inputs))
	if task.Status != domain.StatusCuring && task.Status != domain.StatusPendingApproval {
		failures = append(failures, FieldViolation{"measurements", "仅养护中或已有候选待批准的任务可确认原始测量"})
	}
	if len(inputs) == 0 || len(inputs) > MaxMeasurementsPerBatch {
		failures = append(failures, FieldViolation{"measurements", fmt.Sprintf("批量测量数量必须为 1 到 %d", MaxMeasurementsPerBatch)})
	}
	seenPanels, seenIDs := map[string]bool{}, map[string]bool{}
	for i, input := range inputs {
		prefix := fmt.Sprintf("measurements[%d]", i)
		panelLabel := "试板 " + strings.TrimSpace(input.PanelID) + "："
		panel := task.Panel(strings.TrimSpace(input.PanelID))
		if panel == nil {
			failures = append(failures, FieldViolation{prefix + ".panelId", panelLabel + "不属于当前任务"})
		} else {
			panelLabel = "试板 " + panel.PanelCode + "："
			if seenPanels[panel.ID] {
				failures = append(failures, FieldViolation{prefix + ".panelId", panelLabel + "同一批次不能重复提交"})
			}
			seenPanels[panel.ID] = true
			next, ok := nextCheckpoint(panel)
			if !ok {
				failures = append(failures, FieldViolation{prefix + ".checkpointDay", panelLabel + "所有预定养护节点均已确认"})
			} else {
				if input.Measurement.CheckpointDay != next {
					failures = append(failures, FieldViolation{prefix + ".checkpointDay", panelLabel + fmt.Sprintf("必须提交下一节点第 %d 天", next)})
				}
				if now.UTC().Before(CheckpointDueAt(panel, next)) {
					failures = append(failures, FieldViolation{prefix + ".checkpointDay", panelLabel + "该养护节点尚未到期"})
				}
			}
			for _, old := range panel.Measurements {
				if old.ID == input.Measurement.ID || old.Round == 0 && old.CheckpointDay == input.Measurement.CheckpointDay {
					failures = append(failures, FieldViolation{prefix + ".measurement", panelLabel + "已确认原始测量不可覆盖或篡改"})
					break
				}
			}
		}
		if id := strings.TrimSpace(input.Measurement.ID); id != "" {
			if seenIDs[id] {
				failures = append(failures, FieldViolation{prefix + ".id", "测量 ID 在批次内重复"})
			}
			seenIDs[id] = true
		}
		rowDecisions, err := EvaluateMeasurement(task.AcceptanceThresholds, input.Measurement)
		if err != nil {
			before := len(failures)
			failures = appendPrefixedViolations(failures, err, prefix+".measurement")
			for j := before; j < len(failures); j++ {
				failures[j].Message = panelLabel + failures[j].Message
			}
		} else {
			decisions[i] = rowDecisions
		}
	}
	if len(failures) > 0 {
		return nil, &ValidationError{Violations: failures}
	}
	return decisions, nil
}

func ValidateRemediationDates(plan domain.RemediationPlan, now time.Time) error {
	due, err := time.Parse("2006-01-02", strings.TrimSpace(plan.DueDate))
	if err != nil {
		return &ValidationError{Violations: []FieldViolation{{"remediation.dueDate", "期限必须为 YYYY-MM-DD"}}}
	}
	if due.Before(now.UTC().Truncate(24 * time.Hour)) {
		return &ValidationError{Violations: []FieldViolation{{"remediation.dueDate", "整改期限不得早于当前日期"}}}
	}
	return nil
}

type EligibilityReport struct {
	Eligible bool                      `json:"eligible"`
	Problems []string                  `json:"problems"`
	Checks   []domain.EligibilityCheck `json:"checks"`
	Digest   string                    `json:"digest,omitempty"`
}

type EligibilityCandidate struct {
	FormulaID         string                    `json:"formulaId"`
	PanelID           string                    `json:"panelId"`
	PanelCode         string                    `json:"panelCode"`
	Status            string                    `json:"status"`
	Eligible          bool                      `json:"eligible"`
	Checks            []domain.EligibilityCheck `json:"checks"`
	Reasons           []domain.EligibilityCheck `json:"reasons"`
	EligibilityDigest string                    `json:"eligibilityDigest,omitempty"`
}

func eligibilityCheck(code string, passed bool, message, evidence string) domain.EligibilityCheck {
	return domain.EligibilityCheck{Code: code, Passed: passed, Message: message, EvidenceRef: evidence}
}

func buildEligibilityCandidate(task *domain.TrialTask, formula *domain.MortarFormula, panel *domain.TestPanel, approver string) EligibilityCandidate {
	candidate := EligibilityCandidate{FormulaID: formula.ID, PanelID: panel.ID, PanelCode: panel.PanelCode, Checks: []domain.EligibilityCheck{}, Reasons: []domain.EligibilityCheck{}}
	add := func(check domain.EligibilityCheck) {
		candidate.Checks = append(candidate.Checks, check)
		if !check.Passed {
			candidate.Reasons = append(candidate.Reasons, check)
		}
	}
	add(eligibilityCheck("TASK_PENDING_APPROVAL", task.Status == domain.StatusPendingApproval, "任务应处于待批准状态", "task:"+task.ID))
	confirmed := map[int]domain.Measurement{}
	for _, measurement := range panel.Measurements {
		if measurement.Round == 0 && measurement.Confirmed {
			confirmed[measurement.CheckpointDay] = measurement
		}
	}
	for _, day := range panel.ScheduledCheckpoints {
		measurement, ok := confirmed[day]
		add(eligibilityCheck("CHECKPOINT_CONFIRMED", ok, fmt.Sprintf("第 %d 天节点证据完整", day), fmt.Sprintf("panel:%s/checkpoint:%d", panel.ID, day)))
		add(eligibilityCheck("FOUR_METRIC_DECISIONS", ok && len(measurement.Decisions) == 4, fmt.Sprintf("第 %d 天包含四项指标判定", day), fmt.Sprintf("measurement:%s", measurement.ID)))
	}
	for _, deviation := range panel.Deviations {
		closed := deviation.Status != "pending_review" && deviation.Status != "remediation_required"
		add(eligibilityCheck("DEVIATION_CLOSED", closed, "偏差 "+deviation.ID+" 已闭环", "deviation:"+deviation.ID))
	}
	for _, plan := range panel.Remediations {
		complete := plan.CompletedAt != nil && plan.RetestID != "" && (plan.Outcome == "passed" || plan.Outcome == "failed")
		add(eligibilityCheck("RETEST_ROUND_RECORDED", complete, "整改方案 "+plan.ID+" 的复验轮次留痕完整", "remediation:"+plan.ID))
	}
	passed := panel.Decision == "passed" || panel.Decision == "passed_by_review" || panel.Decision == "retest_passed"
	add(eligibilityCheck("PANEL_PASSED", passed, "试板已形成通过结论", "panel:"+panel.ID))
	approver = strings.TrimSpace(approver)
	add(eligibilityCheck("APPROVER_PRESENT", approver != "", "施工批准人不能为空", "task:"+task.ID+"/approver"))
	add(eligibilityCheck("DUTY_SEPARATION", approver != "" && approver != strings.TrimSpace(task.Owner), "施工批准人与任务负责人职责分离", "task:"+task.ID+"/owner"))
	candidate.Eligible = len(candidate.Reasons) == 0
	candidate.Status = "available"
	for _, reason := range candidate.Reasons {
		if reason.Code == "APPROVER_PRESENT" || reason.Code == "DUTY_SEPARATION" {
			candidate.Status = "duty_conflict"
			continue
		}
		candidate.Status = "missing_evidence"
		break
	}
	if candidate.Eligible {
		payload := struct {
			TaskID     string                    `json:"taskId"`
			Version    int                       `json:"version"`
			FormulaID  string                    `json:"formulaId"`
			PanelID    string                    `json:"panelId"`
			ApprovedBy string                    `json:"approvedBy"`
			Checks     []domain.EligibilityCheck `json:"checks"`
		}{task.ID, task.Version, formula.ID, panel.ID, approver, candidate.Checks}
		candidate.EligibilityDigest, _ = domain.Digest(payload)
	}
	return candidate
}

func BuildEligibilityMatrix(task *domain.TrialTask, approver string) []EligibilityCandidate {
	result := []EligibilityCandidate{}
	for i := range task.Formulas {
		formula := &task.Formulas[i]
		for j := range task.Panels {
			panel := &task.Panels[j]
			if panel.FormulaID == formula.ID {
				result = append(result, buildEligibilityCandidate(task, formula, panel, approver))
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].FormulaID != result[j].FormulaID {
			return result[i].FormulaID < result[j].FormulaID
		}
		if result[i].PanelCode != result[j].PanelCode {
			return result[i].PanelCode < result[j].PanelCode
		}
		return result[i].PanelID < result[j].PanelID
	})
	return result
}

func CheckReleaseEligibility(task *domain.TrialTask, formulaID, panelID, approver string) EligibilityReport {
	report := EligibilityReport{Problems: []string{}, Checks: []domain.EligibilityCheck{}}
	formula, panel := task.Formula(formulaID), task.Panel(panelID)
	if formula == nil || panel == nil || panel.FormulaID != formulaID {
		report.Problems = append(report.Problems, "所选配方与试板不存在或归属不匹配")
		return report
	}
	candidate := buildEligibilityCandidate(task, formula, panel, approver)
	report.Eligible, report.Checks, report.Digest = candidate.Eligible, candidate.Checks, candidate.EligibilityDigest
	for _, reason := range candidate.Reasons {
		report.Problems = append(report.Problems, reason.Code+"："+reason.Message)
	}
	return report
}
