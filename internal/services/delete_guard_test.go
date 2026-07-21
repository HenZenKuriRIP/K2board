package services

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrGroupInUseWrap(t *testing.T) {
	err := fmt.Errorf("%w: %d user(s)", ErrGroupInUse, 3)
	if !errors.Is(err, ErrGroupInUse) {
		t.Fatal("errors.Is should match ErrGroupInUse")
	}
}

func TestErrPlanInUseWrap(t *testing.T) {
	err := fmt.Errorf("%w: %d user(s)", ErrPlanInUse, 2)
	if !errors.Is(err, ErrPlanInUse) {
		t.Fatal("errors.Is should match ErrPlanInUse")
	}
}
