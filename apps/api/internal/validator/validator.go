package validator

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

type CustomValidator struct {
	validate *validator.Validate
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func New() *CustomValidator {
	return &CustomValidator{
		validate: validator.New(),
	}
}

func (cv *CustomValidator) Validate(i interface{}) []ValidationError {
	err := cv.validate.Struct(i)
	if err == nil {
		return nil
	}

	var errors []ValidationError
	for _, err := range err.(validator.ValidationErrors) {
		errors = append(errors, ValidationError{
			Field:   strings.ToLower(err.Field()),
			Message: formatMessage(err),
		})
	}
	return errors
}

func formatMessage(err validator.FieldError) string {
	switch err.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", strings.ToLower(err.Field()))
	case "min":
		return fmt.Sprintf("%s must be at least %s characters", strings.ToLower(err.Field()), err.Param())
	case "max":
		return fmt.Sprintf("%s cannot exceed %s characters", strings.ToLower(err.Field()), err.Param())
	default:
		return fmt.Sprintf("%s failed validation on %s", strings.ToLower(err.Field()), err.Tag())
	}
}
