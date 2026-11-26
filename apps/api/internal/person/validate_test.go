package person_test

import (
	"errors"
	"strings"
	"testing"

	"kinetiq/internal/person"
)

func TestValidateID(t *testing.T) {
	bad := []string{"", "   ", "null", "NULL", "undefined", "anonymous", "Guest", "[object Object]", "nan", "none", "0", strings.Repeat("x", 129)}
	for _, id := range bad {
		if err := person.ValidateID(id); !errors.Is(err, person.ErrInvalidID) {
			t.Errorf("ValidateID(%q) = %v, want ErrInvalidID", id, err)
		}
	}
	for _, id := range []string{"user_42", "a@b.co", "550e8400-e29b-41d4-a716-446655440000", strings.Repeat("x", 128)} {
		if err := person.ValidateID(id); err != nil {
			t.Errorf("ValidateID(%q) = %v, want nil", id, err)
		}
	}
}
