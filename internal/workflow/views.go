package workflow

import (
	"context"
	"fmt"
	"sort"
	"time"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/policy"
)

type StageView struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	State    string `json:"state"`
	Sequence int    `json:"sequence"`
}

type ThresholdView struct {
	Metric   string  `json:"metric"`
	Label    string  `json:"label"`
	Value    float64 `json:"value"`
	Operator string  `json:"operator"`
	Unit     string  `json:"unit"`
}

type CheckpointEvidenceView struct {
	CheckpointDay int                     `json:"checkpointDay"`
	State         string                  `json:"state"`
	MeasurementID string                  `json:"measurementId,omitempty"`
	MeasuredBy    string                  `json:"measuredBy,omitempty"`
	MeasuredAt    *time.Time              `json:"measuredAt,omitempty"`
	Decisions     []domain.MetricDecision `json:"decisions"`
	Immutable     bool                    `json:"immutable"`
}

type PanelEvidenceView struct {
	PanelID          string                   `json:"panelId"`
	PanelCode        string                   `json:"panelCode"`
	FormulaID        string                   `json:"formulaId"`
	Decision         string                   `json:"decision"`
	NextCheckpoint   *int                     `json:"nextCheckpoint,omitempty"`
	Checkpoints      []CheckpointEvidenceView `json:"checkpoints"`
	DeviationTotal   int                      `json:"deviationTotal"`
	DeviationOpen    int                      `json:"deviationOpen"`
	RetestRound      int                      `json:"retestRound"`
	EvidenceComplete bool                     `json:"evidenceComplete"`
}

type FormulaPanelGroupView struct {
	FormulaID     string              `json:"formulaId"`
	ParallelCount int                 `json:"parallelCount"`
	Panels        []PanelEvidenceView `json:"panels"`
}

type CuringQueueItem struct {
	PanelID       string     `json:"panelId"`
	PanelCode     string     `json:"panelCode"`
	FormulaID     string     `json:"formulaId"`
	CheckpointDay int        `json:"checkpointDay"`
	ScheduledAt   time.Time  `json:"scheduledAt"`
	State         string     `json:"state"`
	OverdueDays   int        `json:"overdueDays"`
	MeasurementID string     `json:"measurementId,omitempty"`
	ConfirmedAt   *time.Time `json:"confirmedAt,omitempty"`
	Confirmable   bool       `json:"confirmable"`
}

type AvailableActions struct {
	RegisterPanel     bool `json:"registerPanel"`
	RecordMeasurement bool `json:"recordMeasurement"`
	ReviewDeviation   bool `json:"reviewDeviation"`
	PlanRemediation   bool `json:"planRemediation"`
	RecordRetest      bool `json:"recordRetest"`
	FreezeBatch       bool `json:"freezeBatch"`
	IssueCredential   bool `json:"issueCredential"`
}

type EvidenceSummary struct {
	FormulaCount      int `json:"formulaCount"`
	PanelCount        int `json:"panelCount"`
	ConfirmedMeasures int `json:"confirmedMeasurements"`
	DeviationCount    int `json:"deviationCount"`
	ClosedDeviations  int `json:"closedDeviations"`
	AuditCount        int `json:"auditCount"`
}

type TrialDetailView struct {
	*domain.TrialTask
	Stages             []StageView                   `json:"stages"`
	ThresholdDisplay   []ThresholdView               `json:"thresholdDisplay"`
	PanelEvidence      []PanelEvidenceView           `json:"panelEvidence"`
	FormulaPanelGroups []FormulaPanelGroupView       `json:"formulaPanelGroups"`
	CuringQueue        []CuringQueueItem             `json:"curingQueue"`
	EligibilityMatrix  []policy.EligibilityCandidate `json:"eligibilityMatrix"`
	AvailableActions   AvailableActions              `json:"availableActions"`
	EvidenceSummary    EvidenceSummary               `json:"evidenceSummary"`
	GeneratedAt        time.Time                     `json:"generatedAt"`
}

var workflowStages = []struct {
	code  domain.TaskStatus
	label string
}{
	{domain.StatusDraft, "任务草拟"},
	{domain.StatusCuring, "养护检验"},
	{domain.StatusUnderReview, "偏差复核"},
	{domain.StatusRemediation, "整改复验"},
	{domain.StatusPendingApproval, "待施工批准"},
	{domain.StatusFrozen, "批次已冻结"},
	{domain.StatusReleased, "凭据已签发"},
}

func statusRank(status domain.TaskStatus) int {
	for i, stage := range workflowStages {
		if stage.code == status {
			return i
		}
	}
	return 0
}

func buildStages(status domain.TaskStatus) []StageView {
	current := statusRank(status)
	result := make([]StageView, 0, len(workflowStages))
	for i, stage := range workflowStages {
		state := "pending"
		if i < current {
			state = "complete"
		} else if i == current {
			state = "current"
		}
		// under_review 与 remediation 是同一偏差分支，恢复待批准后都视作完成。
		if statusRank(status) >= statusRank(domain.StatusPendingApproval) && (stage.code == domain.StatusUnderReview || stage.code == domain.StatusRemediation) {
			state = "complete"
		}
		result = append(result, StageView{Code: string(stage.code), Label: stage.label, State: state, Sequence: i + 1})
	}
	return result
}

func thresholdDisplay(thresholds domain.Thresholds) []ThresholdView {
	return []ThresholdView{
		{Metric: "colorDifference", Label: "综合色差", Value: thresholds.MaxColorDifference, Operator: "<=", Unit: "ΔE"},
		{Metric: "shrinkagePct", Label: "收缩率", Value: thresholds.MaxShrinkagePct, Operator: "<=", Unit: "%"},
		{Metric: "bondStrengthMPa", Label: "粘结强度", Value: thresholds.MinBondStrengthMPa, Operator: ">=", Unit: "MPa"},
		{Metric: "powderingGrade", Label: "表面粉化", Value: float64(thresholds.MaxPowderingGrade), Operator: "<=", Unit: "级"},
	}
}

func buildPanelEvidence(panel domain.TestPanel) PanelEvidenceView {
	view := PanelEvidenceView{PanelID: panel.ID, PanelCode: panel.PanelCode, FormulaID: panel.FormulaID, Decision: panel.Decision, DeviationTotal: len(panel.Deviations), RetestRound: panel.RetestRound, Checkpoints: []CheckpointEvidenceView{}}
	measurements := map[int]domain.Measurement{}
	for _, measurement := range panel.Measurements {
		if measurement.Round == 0 {
			measurements[measurement.CheckpointDay] = measurement
		}
	}
	for _, day := range panel.ScheduledCheckpoints {
		checkpoint := CheckpointEvidenceView{CheckpointDay: day, State: "pending", Decisions: []domain.MetricDecision{}}
		if measurement, ok := measurements[day]; ok {
			stamp := measurement.MeasuredAt
			checkpoint.State, checkpoint.MeasurementID, checkpoint.MeasuredBy, checkpoint.MeasuredAt = "confirmed", measurement.ID, measurement.MeasuredBy, &stamp
			checkpoint.Decisions, checkpoint.Immutable = append([]domain.MetricDecision(nil), measurement.Decisions...), measurement.Confirmed
		} else if view.NextCheckpoint == nil {
			next := day
			view.NextCheckpoint = &next
			checkpoint.State = "next"
		}
		view.Checkpoints = append(view.Checkpoints, checkpoint)
	}
	for _, deviation := range panel.Deviations {
		if deviation.Status == "pending_review" || deviation.Status == "remediation_required" {
			view.DeviationOpen++
		}
	}
	view.EvidenceComplete = view.NextCheckpoint == nil && view.DeviationOpen == 0 && panel.Decision != "curing"
	return view
}

func buildCuringQueue(task *domain.TrialTask, now time.Time) []CuringQueueItem {
	queue := []CuringQueueItem{}
	today := now.UTC().Truncate(24 * time.Hour)
	for i := range task.Panels {
		panel := &task.Panels[i]
		confirmed := map[int]domain.Measurement{}
		for _, measurement := range panel.Measurements {
			if measurement.Round == 0 && measurement.Confirmed {
				confirmed[measurement.CheckpointDay] = measurement
			}
		}
		pendingAdded := false
		for _, day := range panel.ScheduledCheckpoints {
			scheduled := policy.CheckpointDueAt(panel, day)
			item := CuringQueueItem{PanelID: panel.ID, PanelCode: panel.PanelCode, FormulaID: panel.FormulaID, CheckpointDay: day, ScheduledAt: scheduled, State: "not_due"}
			if measurement, ok := confirmed[day]; ok {
				stamp := measurement.MeasuredAt
				item.State, item.MeasurementID, item.ConfirmedAt = "confirmed", measurement.ID, &stamp
			} else {
				if pendingAdded {
					continue
				}
				pendingAdded = true
				dueDay := scheduled.UTC().Truncate(24 * time.Hour)
				if dueDay.Equal(today) {
					item.State = "due_today"
				} else if dueDay.Before(today) {
					item.State = "overdue"
					item.OverdueDays = int(today.Sub(dueDay) / (24 * time.Hour))
				}
				item.Confirmable = !now.UTC().Before(scheduled)
			}
			queue = append(queue, item)
		}
	}
	rank := map[string]int{"overdue": 0, "due_today": 1, "not_due": 2, "confirmed": 3}
	sort.SliceStable(queue, func(i, j int) bool {
		if rank[queue[i].State] != rank[queue[j].State] {
			return rank[queue[i].State] < rank[queue[j].State]
		}
		if queue[i].State == "overdue" && queue[i].OverdueDays != queue[j].OverdueDays {
			return queue[i].OverdueDays > queue[j].OverdueDays
		}
		if queue[i].PanelCode != queue[j].PanelCode {
			return queue[i].PanelCode < queue[j].PanelCode
		}
		return queue[i].CheckpointDay < queue[j].CheckpointDay
	})
	return queue
}

func BuildTrialDetailForApprover(task *domain.TrialTask, approver string, now time.Time) *TrialDetailView {
	view := &TrialDetailView{TrialTask: task, Stages: buildStages(task.Status), ThresholdDisplay: thresholdDisplay(task.AcceptanceThresholds), PanelEvidence: []PanelEvidenceView{}, FormulaPanelGroups: []FormulaPanelGroupView{}, CuringQueue: buildCuringQueue(task, now), EligibilityMatrix: policy.BuildEligibilityMatrix(task, approver), GeneratedAt: now.UTC()}
	view.EvidenceSummary.FormulaCount, view.EvidenceSummary.PanelCount, view.EvidenceSummary.AuditCount = len(task.Formulas), len(task.Panels), len(task.Audit)
	for _, panel := range task.Panels {
		view.PanelEvidence = append(view.PanelEvidence, buildPanelEvidence(panel))
		for _, measurement := range panel.Measurements {
			if measurement.Confirmed {
				view.EvidenceSummary.ConfirmedMeasures++
			}
		}
		view.EvidenceSummary.DeviationCount += len(panel.Deviations)
		for _, deviation := range panel.Deviations {
			if deviation.Status != "pending_review" && deviation.Status != "remediation_required" {
				view.EvidenceSummary.ClosedDeviations++
			}
		}
	}
	sort.Slice(view.PanelEvidence, func(i, j int) bool { return view.PanelEvidence[i].PanelCode < view.PanelEvidence[j].PanelCode })
	groups := map[string][]PanelEvidenceView{}
	for _, panel := range view.PanelEvidence {
		groups[panel.FormulaID] = append(groups[panel.FormulaID], panel)
	}
	formulaIDs := make([]string, 0, len(groups))
	for formulaID := range groups {
		formulaIDs = append(formulaIDs, formulaID)
	}
	sort.Strings(formulaIDs)
	for _, formulaID := range formulaIDs {
		view.FormulaPanelGroups = append(view.FormulaPanelGroups, FormulaPanelGroupView{FormulaID: formulaID, ParallelCount: len(groups[formulaID]), Panels: groups[formulaID]})
	}
	view.AvailableActions = AvailableActions{
		RegisterPanel:     task.Status == domain.StatusDraft || task.Status == domain.StatusCuring,
		RecordMeasurement: task.Status == domain.StatusCuring || task.Status == domain.StatusPendingApproval,
		ReviewDeviation:   task.Status == domain.StatusUnderReview || task.Status == domain.StatusRemediation,
		PlanRemediation:   task.Status == domain.StatusRemediation,
		RecordRetest:      task.Status == domain.StatusRemediation,
		FreezeBatch:       task.Status == domain.StatusPendingApproval,
		IssueCredential:   task.Status == domain.StatusFrozen,
	}
	return view
}

func BuildTrialDetail(task *domain.TrialTask, now time.Time) *TrialDetailView {
	return BuildTrialDetailForApprover(task, "", now)
}

func (s *Service) GetTrialView(ctx context.Context, taskID string) (*TrialDetailView, error) {
	return s.GetTrialViewForApprover(ctx, taskID, "")
}

func (s *Service) GetTrialViewForApprover(ctx context.Context, taskID, approver string) (*TrialDetailView, error) {
	task, err := s.GetTrial(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return BuildTrialDetailForApprover(task, approver, s.now()), nil
}

func (s *Service) PreviewEligibility(ctx context.Context, taskID, formulaID, panelID, approver string) (policy.EligibilityReport, error) {
	task, err := s.GetTrial(ctx, taskID)
	if err != nil {
		return policy.EligibilityReport{}, err
	}
	report := policy.CheckReleaseEligibility(task, formulaID, panelID, approver)
	if !report.Eligible && len(report.Problems) == 0 {
		report.Problems = []string{fmt.Sprintf("任务 %s 未满足冻结资格", taskID)}
	}
	return report, nil
}
