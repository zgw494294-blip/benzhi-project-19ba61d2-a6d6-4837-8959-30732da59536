package domain

import "fmt"

type ErrorKind string

const (
	KindInvalid  ErrorKind = "invalid"
	KindConflict ErrorKind = "conflict"
	KindNotFound ErrorKind = "not_found"
	KindIllegal  ErrorKind = "illegal_state"
)

type BusinessError struct {
	Kind    ErrorKind `json:"kind"`
	Field   string    `json:"field,omitempty"`
	Message string    `json:"message"`
}

func (e *BusinessError) Error() string { return e.Message }
func Invalid(field, message string) error {
	return &BusinessError{Kind: KindInvalid, Field: field, Message: message}
}
func Illegal(message string) error { return &BusinessError{Kind: KindIllegal, Message: message} }
func NotFound(entity, id string) error {
	return &BusinessError{Kind: KindNotFound, Message: fmt.Sprintf("%s %q 不存在", entity, id)}
}
func Conflict(message string) error { return &BusinessError{Kind: KindConflict, Message: message} }
