package validator

import (
	"encoding/json"
	"errors"
	"fmt"
	"service/internal/apperror"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/valyala/fasthttp"
)

var (
	once     sync.Once
	validate *validator.Validate
)

func instance() *validator.Validate {
	once.Do(func() {
		validate = validator.New(validator.WithRequiredStructEnabled())
	})
	return validate
}

// BindJSON decodes the request body into dst and runs struct validation.
// Returns an *apperror.Error on failure (bad JSON or validation violation).
func BindJSON(ctx *fasthttp.RequestCtx, dst any) *apperror.Error {
	body := ctx.PostBody()
	if len(body) == 0 {
		return apperror.NewValidation("request body is empty")
	}

	if err := json.Unmarshal(body, dst); err != nil {
		return apperror.NewValidation(fmt.Sprintf("invalid JSON: %s", err.Error()))
	}

	if err := instance().Struct(dst); err != nil {
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			return apperror.NewValidation(formatValidationErrors(ve))
		}
		return apperror.NewValidation(err.Error())
	}

	return nil
}

func formatValidationErrors(ve validator.ValidationErrors) string {
	msgs := make([]string, 0, len(ve))
	for _, fe := range ve {
		msgs = append(msgs, fmt.Sprintf("field '%s' failed on '%s' tag", fe.Field(), fe.Tag()))
	}
	return strings.Join(msgs, "; ")
}
