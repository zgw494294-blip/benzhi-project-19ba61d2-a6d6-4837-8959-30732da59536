package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

func NewTrialTask(id, site, section, substrate, owner string, thresholds Thresholds, now time.Time) (*TrialTask, error) {
	values := map[string]string{"id": id, "siteName": site, "wallSection": section, "substrateCondition": substrate, "owner": owner}
	for field, value := range values {
		if strings.TrimSpace(value) == "" {
			return nil, Invalid(field, "必填字段不能为空")
		}
	}
	now = now.UTC().Truncate(time.Millisecond)
	t := &TrialTask{ID: strings.TrimSpace(id), SiteName: strings.TrimSpace(site), WallSection: strings.TrimSpace(section), SubstrateCondition: strings.TrimSpace(substrate), Owner: strings.TrimSpace(owner), AcceptanceThresholds: thresholds, Status: StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now, Formulas: []MortarFormula{}, Panels: []TestPanel{}, Audit: []AuditEntry{}}
	t.record("trial_created", owner, "创建灰浆试配任务草案", now)
	return t, nil
}

func (t *TrialTask) clone() *TrialTask {
	raw, _ := json.Marshal(t)
	var result TrialTask
	_ = json.Unmarshal(raw, &result)
	return &result
}

func (t *TrialTask) Event(kind, actor string, now time.Time) Event {
	return Event{Type: kind, TaskID: t.ID, Version: t.Version, Actor: strings.TrimSpace(actor), OccurredAt: now.UTC(), Snapshot: t.clone()}
}

func (t *TrialTask) record(kind, actor, summary string, now time.Time) {
	t.UpdatedAt = now.UTC().Truncate(time.Millisecond)
	t.Audit = append(t.Audit, AuditEntry{Sequence: len(t.Audit) + 1, Version: t.Version, Type: kind, Actor: strings.TrimSpace(actor), Summary: summary, OccurredAt: t.UpdatedAt})
}

func (t *TrialTask) mutate(kind, actor, summary string, now time.Time) {
	t.Version++
	t.record(kind, actor, summary, now)
}

func (t *TrialTask) ensureEditable() error {
	if t.Status == StatusFrozen || t.Status == StatusReleased {
		return Illegal("施工批次冻结后，配方、阈值与检验结论不可修改")
	}
	return nil
}

func (t *TrialTask) Formula(id string) *MortarFormula {
	for i := range t.Formulas {
		if t.Formulas[i].ID == id {
			return &t.Formulas[i]
		}
	}
	return nil
}

func (t *TrialTask) Panel(id string) *TestPanel {
	for i := range t.Panels {
		if t.Panels[i].ID == id {
			return &t.Panels[i]
		}
	}
	return nil
}

func (t *TrialTask) AddFormulaAndPanel(formula MortarFormula, panel TestPanel, actor string, now time.Time) error {
	return t.AddPanels(&formula, "", []TestPanel{panel}, actor, now)
}

// AddPanels 将一个新配方或任务内既有配方与一组实体试板原子关联。
// 调用方须先完成整组策略校验；本方法仍保留聚合状态和归属不变量。
func (t *TrialTask) AddPanels(newFormula *MortarFormula, formulaID string, panels []TestPanel, actor string, now time.Time) error {
	if err := t.ensureEditable(); err != nil {
		return err
	}
	if t.Status != StatusDraft && t.Status != StatusCuring {
		return Illegal("仅草拟或养护中任务可登记新试板")
	}
	if len(panels) == 0 {
		return Invalid("panels", "至少登记一块试板")
	}
	if newFormula != nil {
		formulaID = newFormula.ID
		for _, existing := range t.Formulas {
			if existing.ID == formulaID {
				return Invalid("formula.id", "配方修订身份已存在；引用既有配方时请仅提交 formulaId")
			}
		}
	} else if t.Formula(formulaID) == nil {
		return NotFound("任务内配方", formulaID)
	}
	seen := map[string]bool{}
	for _, existing := range t.Panels {
		seen[strings.ToLower(strings.TrimSpace(existing.PanelCode))] = true
	}
	for i := range panels {
		code := strings.ToLower(strings.TrimSpace(panels[i].PanelCode))
		if seen[code] {
			return Invalid(fmt.Sprintf("panels[%d].panelCode", i), "试板编号在任务内必须唯一（忽略大小写及首尾空白）")
		}
		seen[code] = true
	}
	if newFormula != nil {
		formula := *newFormula
		formula.TaskID, formula.PreparedAt = t.ID, formula.PreparedAt.UTC()
		t.Formulas = append(t.Formulas, formula)
	}
	codes := make([]string, 0, len(panels))
	for i := range panels {
		panel := panels[i]
		panel.TaskID, panel.FormulaID = t.ID, formulaID
		panel.PanelCode, panel.CuringStartedAt = strings.TrimSpace(panel.PanelCode), panel.CuringStartedAt.UTC()
		panel.Measurements, panel.Deviations, panel.Remediations = []Measurement{}, []Deviation{}, []RemediationPlan{}
		panel.Decision = "curing"
		t.Panels = append(t.Panels, panel)
		codes = append(codes, panel.PanelCode)
	}
	t.Status = StatusCuring
	t.mutate("parallel_panels_registered", actor, fmt.Sprintf("配方 %s 登记 %d 块平行试板：%s", formulaID, len(panels), strings.Join(codes, "、")), now)
	return nil
}

func expectedCheckpoint(panel *TestPanel) (int, bool) {
	seen := map[int]bool{}
	for _, m := range panel.Measurements {
		if m.Round == 0 {
			seen[m.CheckpointDay] = true
		}
	}
	for _, day := range panel.ScheduledCheckpoints {
		if !seen[day] {
			return day, true
		}
	}
	return 0, false
}

func (t *TrialTask) RecordMeasurement(panelID string, measurement Measurement, decisions []MetricDecision, actor string, now time.Time) error {
	return t.RecordMeasurements([]PanelMeasurement{{PanelID: panelID, Measurement: measurement, Decisions: decisions}}, actor, now)
}

type PanelMeasurement struct {
	PanelID     string
	Measurement Measurement
	Decisions   []MetricDecision
}

func (t *TrialTask) recomputeWorkflowStatus() {
	hasPendingReview := false
	hasRemediation := false
	hasPassed := false
	for i := range t.Panels {
		panel := &t.Panels[i]
		if _, remains := expectedCheckpoint(panel); remains {
			panel.Decision = "curing"
			continue
		}
		if len(panel.Deviations) == 0 {
			panel.Decision = "passed"
			hasPassed = true
			continue
		}
		pendingReview, remediation := false, false
		for _, deviation := range panel.Deviations {
			pendingReview = pendingReview || deviation.Status == "pending_review"
			remediation = remediation || deviation.Status == "remediation_required"
		}
		if remediation {
			panel.Decision = "remediation_required"
			hasRemediation = true
		} else if pendingReview {
			panel.Decision = "deviation_review"
			hasPendingReview = true
		} else {
			panel.Decision = "passed_by_review"
			for _, measurement := range panel.Measurements {
				if measurement.Round > 0 {
					panel.Decision = "retest_passed"
				}
			}
			hasPassed = true
		}
	}
	if hasRemediation {
		t.Status = StatusRemediation
	} else if hasPendingReview {
		t.Status = StatusUnderReview
	} else if hasPassed {
		t.Status = StatusPendingApproval
	} else {
		t.Status = StatusCuring
	}
}

// RecordMeasurements 只在全部记录已通过策略预检后一次追加整批原始证据。
func (t *TrialTask) RecordMeasurements(records []PanelMeasurement, actor string, now time.Time) error {
	if err := t.ensureEditable(); err != nil {
		return err
	}
	if t.Status != StatusCuring && t.Status != StatusPendingApproval {
		return Illegal("仅养护中或已有候选待批准的任务可记录原始养护检验")
	}
	if len(records) == 0 {
		return Invalid("measurements", "至少提交一条测量")
	}
	stamp := now.UTC().Truncate(time.Millisecond)
	for _, record := range records {
		panel := t.Panel(record.PanelID)
		if panel == nil {
			return NotFound("试板", record.PanelID)
		}
		measurement := record.Measurement
		measurement.Round, measurement.Confirmed, measurement.Decisions = 0, true, append([]MetricDecision(nil), record.Decisions...)
		measurement.MeasuredAt = stamp
		panel.Measurements = append(panel.Measurements, measurement)
		for _, d := range record.Decisions {
			if d.Passed {
				continue
			}
			id := fmt.Sprintf("DEV-%s-%02d", measurement.ID, len(panel.Deviations)+1)
			panel.Deviations = append(panel.Deviations, Deviation{ID: id, PanelID: panel.ID, MeasurementID: measurement.ID, Metric: d.Metric, Observed: d.Value, Requirement: d.Operator + fmt.Sprintf(" %.4g", d.Threshold), Status: "pending_review", Round: 0})
		}
	}
	t.recomputeWorkflowStatus()
	t.mutate("measurements_batch_confirmed", actor, fmt.Sprintf("原子确认 %d 块试板的当前养护节点测量", len(records)), now)
	return nil
}

func (t *TrialTask) ReviewDeviation(panelID, deviationID, conclusion, reviewer, note string, now time.Time) error {
	if t.Status != StatusUnderReview && t.Status != StatusRemediation {
		return Illegal("当前状态不接受专业偏差复核")
	}
	panel := t.Panel(panelID)
	if panel == nil {
		return NotFound("试板", panelID)
	}
	var target *Deviation
	for i := range panel.Deviations {
		if panel.Deviations[i].ID == deviationID {
			target = &panel.Deviations[i]
			break
		}
	}
	if target == nil {
		return NotFound("偏差", deviationID)
	}
	if target.Status != "pending_review" {
		return Illegal("偏差已有专业裁决，不可覆盖")
	}
	if strings.TrimSpace(reviewer) == "" || strings.TrimSpace(note) == "" {
		return Invalid("review", "复核人和专业结论说明不能为空")
	}
	if conclusion != "accepted" && conclusion != "remediation_required" {
		return Invalid("conclusion", "结论必须为 accepted 或 remediation_required")
	}
	stamp := now.UTC().Truncate(time.Millisecond)
	target.Status, target.ReviewConclusion, target.ReviewedBy, target.ReviewedAt = conclusion, strings.TrimSpace(note), strings.TrimSpace(reviewer), &stamp
	t.recomputeWorkflowStatus()
	t.mutate("deviation_reviewed", reviewer, "保护责任人完成偏差 "+deviationID+" 的专业裁决", now)
	return nil
}

func (t *TrialTask) AddRemediation(panelID string, plan RemediationPlan, actor string, now time.Time) error {
	if t.Status != StatusRemediation {
		return Illegal("仅整改中任务可登记整改方案")
	}
	panel := t.Panel(panelID)
	if panel == nil {
		return NotFound("试板", panelID)
	}
	if strings.TrimSpace(plan.Action) == "" || strings.TrimSpace(plan.DueDate) == "" || strings.TrimSpace(plan.Responsible) == "" {
		return Invalid("remediation", "整改措施、期限和责任人不能为空")
	}
	if len(plan.DeviationIDs) == 0 {
		return Invalid("deviationIds", "整改方案必须关联偏差")
	}
	for _, existing := range panel.Remediations {
		if existing.ID == plan.ID {
			return Invalid("plan.id", "整改方案身份已存在")
		}
	}
	for _, id := range plan.DeviationIDs {
		found := false
		for _, d := range panel.Deviations {
			if d.ID == id && d.Status == "remediation_required" {
				found = true
			}
		}
		if !found {
			return Invalid("deviationIds", "整改方案只能关联当前轮待整改偏差 "+id)
		}
		for _, existing := range panel.Remediations {
			if existing.CompletedAt != nil {
				continue
			}
			for _, covered := range existing.DeviationIDs {
				if covered == id {
					return Invalid("deviationIds", "偏差 "+id+" 已由进行中的整改方案覆盖")
				}
			}
		}
	}
	plan.PanelID, plan.CreatedAt = panel.ID, now.UTC().Truncate(time.Millisecond)
	panel.Remediations = append(panel.Remediations, plan)
	t.mutate("remediation_planned", actor, "登记限期整改方案 "+plan.ID, now)
	return nil
}

func (t *TrialTask) RecordRetest(panelID, planID string, result Measurement, decisions []MetricDecision, actor string, now time.Time) error {
	if t.Status != StatusRemediation {
		return Illegal("仅整改中任务可提交复验")
	}
	panel := t.Panel(panelID)
	if panel == nil {
		return NotFound("试板", panelID)
	}
	var plan *RemediationPlan
	for i := range panel.Remediations {
		if panel.Remediations[i].ID == planID {
			plan = &panel.Remediations[i]
			break
		}
	}
	if plan == nil {
		return NotFound("整改方案", planID)
	}
	if plan.CompletedAt != nil {
		return Illegal("该整改方案已完成复验，历史结果不可覆盖")
	}
	current := map[string]*Deviation{}
	for i := range panel.Deviations {
		current[panel.Deviations[i].ID] = &panel.Deviations[i]
	}
	for _, id := range plan.DeviationIDs {
		deviation := current[id]
		if deviation == nil || deviation.PanelID != panel.ID || deviation.Status != "remediation_required" {
			return Invalid("planId", "整改方案未覆盖当前试板本轮待整改偏差 "+id)
		}
	}
	panel.RetestRound++
	result.Round, result.Confirmed, result.Decisions, result.MeasuredAt = panel.RetestRound, true, append([]MetricDecision(nil), decisions...), now.UTC().Truncate(time.Millisecond)
	panel.Measurements = append(panel.Measurements, result)
	stamp := result.MeasuredAt
	plan.CompletedAt, plan.RetestID = &stamp, result.ID
	decisionByMetric := map[string]MetricDecision{}
	allPassed := true
	for _, decision := range decisions {
		decisionByMetric[decision.Metric] = decision
		if !decision.Passed {
			allPassed = false
		}
	}
	if allPassed {
		plan.Outcome = "passed"
	} else {
		plan.Outcome = "failed"
	}
	for _, id := range plan.DeviationIDs {
		deviation := current[id]
		if decision, ok := decisionByMetric[deviation.Metric]; ok && decision.Passed {
			deviation.Status = "closed"
		} else {
			deviation.Status = "retest_failed"
		}
		deviation.RetestID = result.ID
	}
	if !allPassed {
		for _, decision := range decisions {
			if decision.Passed {
				continue
			}
			id := fmt.Sprintf("DEV-%s-%02d", result.ID, len(panel.Deviations)+1)
			panel.Deviations = append(panel.Deviations, Deviation{ID: id, PanelID: panel.ID, MeasurementID: result.ID, Metric: decision.Metric, Observed: decision.Value, Requirement: decision.Operator + fmt.Sprintf(" %.4g", decision.Threshold), Status: "remediation_required", SourceRetestID: result.ID, Round: panel.RetestRound})
		}
	}
	t.recomputeWorkflowStatus()
	summary := fmt.Sprintf("完成试板 %s 第 %d 轮整改复验", panel.PanelCode, panel.RetestRound)
	if !allPassed {
		summary += "，结果未通过并生成下一轮待整改偏差"
	}
	t.mutate("retest_confirmed", actor, summary, now)
	return nil
}

func (t *TrialTask) Freeze(formulaID, panelID, approvedBy, eligibilityDigest string, checks []EligibilityCheck, now time.Time) error {
	if t.Status != StatusPendingApproval {
		return Illegal("只有证据闭环的待批准任务可冻结")
	}
	formula, panel := t.Formula(formulaID), t.Panel(panelID)
	if formula == nil || panel == nil || panel.FormulaID != formulaID {
		return Invalid("selection", "所选配方与试板不存在或不匹配")
	}
	if panel.Decision != "passed" && panel.Decision != "passed_by_review" && panel.Decision != "retest_passed" {
		return Illegal("所选试板尚未通过")
	}
	if strings.TrimSpace(approvedBy) == "" || approvedBy == t.Owner {
		return Invalid("approvedBy", "施工批准人不能为空且应与任务负责人分离")
	}
	stamp := now.UTC().Truncate(time.Millisecond)
	t.FrozenBatch = &BatchSnapshot{TaskID: t.ID, SiteName: t.SiteName, WallSection: t.WallSection, Thresholds: t.AcceptanceThresholds, Formula: *formula, Panel: *panel, ApprovedBy: strings.TrimSpace(approvedBy), ApprovedAt: stamp, TaskVersion: t.Version + 1, EligibilityDigest: eligibilityDigest, EligibilityChecks: append([]EligibilityCheck(nil), checks...)}
	t.Status = StatusFrozen
	t.mutate("batch_frozen", approvedBy, "施工负责人批准并冻结配方与试板证据快照", now)
	return nil
}

func Digest(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func (t *TrialTask) IssueCredential(sequence uint64, previousDigest, actor string, now time.Time) (*ReleaseCredential, error) {
	if t.Status != StatusFrozen || t.FrozenBatch == nil {
		return nil, Illegal("仅冻结且批准的施工批次可签发凭据")
	}
	if t.Credential != nil {
		return nil, Illegal("施工批次已签发凭据")
	}
	if strings.TrimSpace(actor) != t.FrozenBatch.ApprovedBy {
		return nil, Invalid("approvedBy", "凭据签发人必须是冻结批准人")
	}
	stamp := now.UTC().Truncate(time.Millisecond)
	credential := &ReleaseCredential{CredentialNo: fmt.Sprintf("MMG-%08d", sequence), TaskID: t.ID, BatchSnapshot: *t.FrozenBatch, ApprovedBy: actor, IssuedAt: stamp, Sequence: sequence, PreviousDigest: previousDigest}
	digest, err := CredentialDigest(credential)
	if err != nil {
		return nil, err
	}
	credential.ContentDigest = digest
	t.Credential, t.Status = credential, StatusReleased
	t.mutate("credential_issued", actor, "签发施工放行凭据 "+credential.CredentialNo, now)
	return credential, nil
}

func CredentialDigest(c *ReleaseCredential) (string, error) {
	copy := *c
	copy.ContentDigest = ""
	return Digest(copy)
}

func (t *TrialTask) StableAudit() []AuditEntry {
	entries := append([]AuditEntry(nil), t.Audit...)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Sequence < entries[j].Sequence })
	return entries
}
