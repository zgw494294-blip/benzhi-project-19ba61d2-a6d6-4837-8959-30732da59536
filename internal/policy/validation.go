package policy

// FieldViolation 将业务校验错误定位到稳定的请求字段。
type FieldViolation struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationError struct {
	Violations []FieldViolation `json:"violations"`
}

func (e *ValidationError) Error() string {
	if len(e.Violations) == 0 {
		return "业务输入不符合规则"
	}
	return e.Violations[0].Field + ": " + e.Violations[0].Message
}
