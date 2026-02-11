package base

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetForkliftError_NoErr_NoHeader(t *testing.T) {
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	SetForkliftError(ctx, nil)
	if got := w.Header().Get("forklift-error-message"); got != "" {
		t.Fatalf("expected empty header, got %q", got)
	}
}

func TestSetForkliftError_SetsHeaderAndError(t *testing.T) {
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	err := errors.New("boom")
	SetForkliftError(ctx, err)
	if got := w.Header().Get("forklift-error-message"); got != "boom" {
		t.Fatalf("expected header boom, got %q", got)
	}
	if len(ctx.Errors) != 1 {
		t.Fatalf("expected ctx error recorded, got %d", len(ctx.Errors))
	}
}
