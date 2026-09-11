package service

import (
	"errors"
	"testing"

	"relationship/internal/models"
)

// TestPersonServiceListGuards the closed ordering vocabulary: an unknown sort
// or a negative page window is a caller mistake, not something to map onto a
// default silently.
func TestPersonServiceListGuards(t *testing.T) {
	svc := NewPersonService(nil, nil, nil)

	if _, _, err := svc.List(models.PersonFilter{Sort: "popularity"}); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("unknown sort = %v, want ErrInvalidInput", err)
	}
	if _, _, err := svc.List(models.PersonFilter{Limit: -1}); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("negative limit = %v, want ErrInvalidInput", err)
	}
	if _, _, err := svc.List(models.PersonFilter{Offset: -5}); !errors.Is(err, models.ErrInvalidInput) {
		t.Fatalf("negative offset = %v, want ErrInvalidInput", err)
	}
	// The accepted orderings are exercised with a real database by the
	// handler test; here the nil repo would panic, which is exactly the point
	// of keeping this test to the rejection paths.
}
