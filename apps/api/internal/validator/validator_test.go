package validator_test

import (
	"testing"

	"kinetiq/internal/validator"
)

type sampleRequest struct {
	Name  string `json:"name" validate:"required,min=3,max=10"`
	Email string `json:"email" validate:"required,email"`
}

func TestCustomValidator_Unit(t *testing.T) {
	v := validator.New()

	t.Run("valid struct passes validation", func(t *testing.T) {
		req := sampleRequest{
			Name:  "Stream",
			Email: "test@kinetiq.io",
		}
		errs := v.Validate(req)
		if len(errs) != 0 {
			t.Errorf("expected 0 errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("missing required fields return validation errors", func(t *testing.T) {
		req := sampleRequest{}
		errs := v.Validate(req)
		if len(errs) == 0 {
			t.Fatalf("expected validation errors, got 0")
		}

		fields := make(map[string]bool)
		for _, e := range errs {
			fields[e.Field] = true
		}
		if !fields["name"] {
			t.Errorf("expected validation error on 'name'")
		}
		if !fields["email"] {
			t.Errorf("expected validation error on 'email'")
		}
	})

	t.Run("out of range min/max and malformed email return specific messages", func(t *testing.T) {
		req := sampleRequest{
			Name:  "ab", // min is 3
			Email: "not-an-email",
		}
		errs := v.Validate(req)
		if len(errs) != 2 {
			t.Fatalf("expected 2 validation errors, got %d", len(errs))
		}
	})
}
