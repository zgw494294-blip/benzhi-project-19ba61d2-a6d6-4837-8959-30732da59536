package webui

import (
	"encoding/json"
	"errors"
	"net/http"

	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/domain"
	"benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536/internal/policy"
)

type envelope struct {
	Data  any       `json:"data,omitempty"`
	Error *apiError `json:"error,omitempty"`
}

type apiError struct {
	Code       string                  `json:"code"`
	Message    string                  `json:"message"`
	Field      string                  `json:"field,omitempty"`
	Violations []policy.FieldViolation `json:"violations,omitempty"`
}

func jsonDecoder(r *http.Request) *json.Decoder { return json.NewDecoder(r.Body) }

func domainInvalidJSON(err error) error {
	return domain.Invalid("body", "JSON 请求无效："+err.Error())
}

func writeData(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Data: data})
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	output := &apiError{Code: "internal_error", Message: "服务处理失败"}
	var business *domain.BusinessError
	var validation *policy.ValidationError
	switch {
	case errors.Is(err, errUnsupportedMedia):
		status = http.StatusUnsupportedMediaType
		output = &apiError{Code: "unsupported_media_type", Message: err.Error()}
	case errors.As(err, &validation):
		status = http.StatusUnprocessableEntity
		output = &apiError{Code: "validation_failed", Message: validation.Error(), Violations: validation.Violations}
	case errors.As(err, &business):
		output = &apiError{Code: string(business.Kind), Message: business.Message, Field: business.Field}
		switch business.Kind {
		case domain.KindNotFound:
			status = http.StatusNotFound
		case domain.KindConflict, domain.KindIllegal:
			status = http.StatusConflict
		default:
			status = http.StatusUnprocessableEntity
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Error: output})
}
