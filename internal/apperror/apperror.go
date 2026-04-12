package apperror

import (
	"fmt"
	"net/http"
)

type Error struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
	Err        error  `json:"-"`
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func NewNotFound(msg string) *Error {
	return &Error{Code: "NOT_FOUND", Message: msg, HTTPStatus: http.StatusNotFound}
}

func NewValidation(msg string) *Error {
	return &Error{Code: "VALIDATION_ERROR", Message: msg, HTTPStatus: http.StatusBadRequest}
}

func NewConflict(msg string) *Error {
	return &Error{Code: "CONFLICT", Message: msg, HTTPStatus: http.StatusConflict}
}

func NewUnauthorized(msg string) *Error {
	return &Error{Code: "UNAUTHORIZED", Message: msg, HTTPStatus: http.StatusUnauthorized}
}

func NewInternal(msg string, err error) *Error {
	return &Error{Code: "INTERNAL_ERROR", Message: msg, HTTPStatus: http.StatusInternalServerError, Err: err}
}

type Response struct {
	Error *ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func ToResponse(e *Error) Response {
	return Response{
		Error: &ErrorBody{
			Code:    e.Code,
			Message: e.Message,
		},
	}
}
