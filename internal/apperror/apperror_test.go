package apperror_test

import (
	"errors"
	"fmt"
	"net/http"
	"service/internal/apperror"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestError_Types(t *testing.T) {
	tests := []struct {
		name       string
		err        *apperror.Error
		wantCode   string
		wantStatus int
	}{
		{"not found", apperror.NewNotFound("item missing"), "NOT_FOUND", http.StatusNotFound},
		{"validation", apperror.NewValidation("bad input"), "VALIDATION_ERROR", http.StatusBadRequest},
		{"conflict", apperror.NewConflict("duplicate"), "CONFLICT", http.StatusConflict},
		{"unauthorized", apperror.NewUnauthorized("no token"), "UNAUTHORIZED", http.StatusUnauthorized},
		{"internal", apperror.NewInternal("oops", fmt.Errorf("db down")), "INTERNAL_ERROR", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantCode, tt.err.Code)
			assert.Equal(t, tt.wantStatus, tt.err.HTTPStatus)
			assert.NotEmpty(t, tt.err.Error())
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("root cause")
	appErr := apperror.NewInternal("wrapped", cause)

	assert.True(t, errors.Is(appErr, cause))
}

func TestToResponse(t *testing.T) {
	appErr := apperror.NewNotFound("playlist not found")
	resp := apperror.ToResponse(appErr)

	assert.Equal(t, "NOT_FOUND", resp.Error.Code)
	assert.Equal(t, "playlist not found", resp.Error.Message)
}
