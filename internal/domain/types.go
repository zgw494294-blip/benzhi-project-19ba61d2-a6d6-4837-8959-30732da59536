package domain

import "time"

type Thresholds struct {
	MaxColorDifference float64 `json:"maxColorDifference"`
	MaxShrinkagePct    float64 `json:"maxShrinkagePct"`
	MinBondStrengthMPa float64 `json:"minBondStrengthMPa"`
	MaxPowderingGrade  int     `json:"maxPowderingGrade"`
}

type FormulaComponent struct {
	Name       string  `json:"name"`
	Percentage float64 `json:"percentage"`
	BatchRef   string  `json:"batchRef"`
}

type MortarFormula struct {
	ID           string             `json:"id"`
	TaskID       string             `json:"taskId"`
	Revision     int                `json:"revision"`
	Components   []FormulaComponent `json:"components"`
	WaterRatio   float64            `json:"waterRatio"`
	MixingMethod string             `json:"mixingMethod"`
	PreparedBy   string             `json:"preparedBy"`
	PreparedAt   time.Time          `json:"preparedAt"`
	TemperatureC float64            `json:"temperatureC"`
	HumidityPct  float64            `json:"humidityPct"`
}

type MetricDecision struct {
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Operator  string  `json:"operator"`
	Passed    bool    `json:"passed"`
}

type Measurement struct {
	ID              string           `json:"id"`
	CheckpointDay   int              `json:"checkpointDay"`
	Round           int              `json:"round"`
	ColorDifference float64          `json:"colorDifference"`
	ShrinkagePct    float64          `json:"shrinkagePct"`
	BondStrengthMPa float64          `json:"bondStrengthMPa"`
	PowderingGrade  int              `json:"powderingGrade"`
	Observation     string           `json:"observation"`
	MeasuredBy      string           `json:"measuredBy"`
	MeasuredAt      time.Time        `json:"measuredAt"`
	Confirmed       bool             `json:"confirmed"`
	Decisions       []MetricDecision `json:"decisions"`
}

type Deviation struct {
	ID               string     `json:"id"`
	PanelID          string     `json:"panelId"`
	MeasurementID    string     `json:"measurementId"`
	Metric           string     `json:"metric"`
	Observed         float64    `json:"observed"`
	Requirement      string     `json:"requirement"`
	Status           string     `json:"status"`
	ReviewConclusion string     `json:"reviewConclusion,omitempty"`
	ReviewedBy       string     `json:"reviewedBy,omitempty"`
	ReviewedAt       *time.Time `json:"reviewedAt,omitempty"`
	RetestID         string     `json:"retestId,omitempty"`
	SourceRetestID   string     `json:"sourceRetestId,omitempty"`
	Round            int        `json:"round"`
}

type RemediationPlan struct {
	ID           string     `json:"id"`
	PanelID      string     `json:"panelId"`
	DeviationIDs []string   `json:"deviationIds"`
	Action       string     `json:"action"`
	DueDate      string     `json:"dueDate"`
	Responsible  string     `json:"responsible"`
	CreatedAt    time.Time  `json:"createdAt"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
	Outcome      string     `json:"outcome,omitempty"`
	RetestID     string     `json:"retestId,omitempty"`
}

type TestPanel struct {
	ID                   string            `json:"id"`
	TaskID               string            `json:"taskId"`
	FormulaID            string            `json:"formulaId"`
	PanelCode            string            `json:"panelCode"`
	CuringStartedAt      time.Time         `json:"curingStartedAt"`
	ScheduledCheckpoints []int             `json:"scheduledCheckpoints"`
	Measurements         []Measurement     `json:"measurements"`
	Deviations           []Deviation       `json:"deviations"`
	Remediations         []RemediationPlan `json:"remediations"`
	RetestRound          int               `json:"retestRound"`
	Decision             string            `json:"decision"`
}
