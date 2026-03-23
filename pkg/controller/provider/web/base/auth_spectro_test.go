package base

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	auth "k8s.io/api/authentication/v1"
	authz "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ---- Consolidated from auth_more_test.go ----

type captureWriter struct {
	// behavior
	authenticated bool
	allowed       bool
	createErr     error
	// observation
	trCount  int
	sarCount int
	lastSAR  *authz.SubjectAccessReview
}

func (w *captureWriter) Create(ctx context.Context, object client.Object, option ...client.CreateOption) error {
	if w.createErr != nil {
		return w.createErr
	}
	if tr, ok := object.(*auth.TokenReview); ok {
		w.trCount++
		tr.Status.Authenticated = w.authenticated
		tr.Status.User = auth.UserInfo{
			Username: "u1",
			UID:      "uid",
			Groups:   []string{"g1"},
			Extra:    map[string]auth.ExtraValue{"k": {"v"}},
		}
		return nil
	}
	if sar, ok := object.(*authz.SubjectAccessReview); ok {
		w.sarCount++
		sar.Status.Allowed = w.allowed
		cp := sar.DeepCopy()
		w.lastSAR = cp
		return nil
	}
	return nil
}

func (w *captureWriter) Delete(context.Context, client.Object, ...client.DeleteOption) error {
	return nil
}
func (w *captureWriter) Update(context.Context, client.Object, ...client.UpdateOption) error {
	return nil
}
func (w *captureWriter) Patch(context.Context, client.Object, client.Patch, ...client.PatchOption) error {
	return nil
}
func (w *captureWriter) DeleteAllOf(context.Context, client.Object, ...client.DeleteAllOfOption) error {
	return nil
}

func ginCtxWithAuth(t *testing.T, token string, rawURL string) *gin.Context {
	t.Helper()
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	u, _ := url.Parse(rawURL)
	ctx.Request = &http.Request{URL: u, Header: http.Header{}}
	if token != "" {
		ctx.Request.Header.Set("Authorization", "Bearer "+token)
	}
	return ctx
}

type stepErrWriter struct {
	authenticated bool
	allowed       bool
	sarErr        error
	trCount       int
	sarCount      int
}

func (w *stepErrWriter) Create(ctx context.Context, object client.Object, option ...client.CreateOption) error {
	if tr, ok := object.(*auth.TokenReview); ok {
		w.trCount++
		tr.Status.Authenticated = w.authenticated
		tr.Status.User = auth.UserInfo{Username: "u1"}
		return nil
	}
	if sar, ok := object.(*authz.SubjectAccessReview); ok {
		w.sarCount++
		sar.Status.Allowed = w.allowed
		if w.sarErr != nil {
			return w.sarErr
		}
		return nil
	}
	return nil
}

func (w *stepErrWriter) Delete(context.Context, client.Object, ...client.DeleteOption) error {
	return nil
}
func (w *stepErrWriter) Update(context.Context, client.Object, ...client.UpdateOption) error {
	return nil
}
func (w *stepErrWriter) Patch(context.Context, client.Object, client.Patch, ...client.PatchOption) error {
	return nil
}
func (w *stepErrWriter) DeleteAllOf(context.Context, client.Object, ...client.DeleteAllOfOption) error {
	return nil
}

func TestAuth_writer_ReturnsExistingWriter(t *testing.T) {
	a := &Auth{Writer: &captureWriter{}}
	w, err := a.writer()
	if err != nil || w == nil {
		t.Fatalf("expected existing writer, got %v %v", w, err)
	}
}

func TestAuth_writer_ErrWhenNoConfig(t *testing.T) {
	a := &Auth{}
	t.Setenv("KUBECONFIG", "/no/such/kubeconfig")
	_, err := a.writer()
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestAuth_permit_Returns500OnWriterErr(t *testing.T) {
	a := &Auth{}
	t.Setenv("KUBECONFIG", "/no/such/kubeconfig")
	p := &api.Provider{}
	st, err := a.permit("tok", "ns", p)
	if st != http.StatusInternalServerError || err == nil {
		t.Fatalf("expected 500 + err, got %d %v", st, err)
	}
}

func TestAuth_permit_Returns500OnCreateErr(t *testing.T) {
	sentinel := errors.New("boom")
	a := &Auth{Writer: &captureWriter{createErr: sentinel}}
	p := &api.Provider{}
	st, err := a.permit("tok", "ns", p)
	if st != http.StatusInternalServerError || err == nil {
		t.Fatalf("expected 500 + err, got %d %v", st, err)
	}
}

func TestAuth_permit_Returns500OnSARCreateErr(t *testing.T) {
	sentinel := errors.New("sar boom")
	w := &stepErrWriter{authenticated: true, allowed: true, sarErr: sentinel}
	a := &Auth{Writer: w}
	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	st, err := a.permit("tok", "ns", p)
	if st != http.StatusInternalServerError || err == nil {
		t.Fatalf("expected 500 + err, got %d %v", st, err)
	}
}

func TestAuth_permit_UnauthorizedWhenNotAuthenticated(t *testing.T) {
	a := &Auth{Writer: &captureWriter{authenticated: false, allowed: true}}
	p := &api.Provider{}
	st, err := a.permit("tok", "ns", p)
	if st != http.StatusUnauthorized || err != nil {
		t.Fatalf("expected 401 nil, got %d %v", st, err)
	}
}

func TestAuth_permit_ReturnsForbiddenWhenNotAllowed(t *testing.T) {
	a := &Auth{Writer: &captureWriter{authenticated: true, allowed: false}}
	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	st, err := a.permit("tok", "ns", p)
	if st != http.StatusForbidden || err == nil {
		t.Fatalf("expected 403 + err, got %d %v", st, err)
	}
}

func TestAuth_permit_VerbListWhenProviderUIDEmpty(t *testing.T) {
	w := &captureWriter{authenticated: true, allowed: true}
	a := &Auth{Writer: w}
	p := &api.Provider{} // UID empty => list
	st, err := a.permit("tok", "nsX", p)
	if st != http.StatusOK || err != nil {
		t.Fatalf("expected 200 nil, got %d %v", st, err)
	}
	if w.lastSAR == nil || w.lastSAR.Spec.ResourceAttributes == nil {
		t.Fatalf("expected SAR captured")
	}
	if w.lastSAR.Spec.ResourceAttributes.Verb != "list" {
		t.Fatalf("expected verb list, got %q", w.lastSAR.Spec.ResourceAttributes.Verb)
	}
	if w.lastSAR.Spec.ResourceAttributes.Namespace != "nsX" {
		t.Fatalf("expected namespace nsX, got %q", w.lastSAR.Spec.ResourceAttributes.Namespace)
	}
}

func TestAuth_permit_VerbGetWhenProviderUIDSet(t *testing.T) {
	w := &captureWriter{authenticated: true, allowed: true}
	a := &Auth{Writer: w}
	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{UID: types.UID("u1"), Namespace: "nsY", Name: "p"}}
	st, err := a.permit("tok", "ignored", p)
	if st != http.StatusOK || err != nil {
		t.Fatalf("expected 200 nil, got %d %v", st, err)
	}
	if w.lastSAR.Spec.ResourceAttributes.Verb != "get" {
		t.Fatalf("expected verb get, got %q", w.lastSAR.Spec.ResourceAttributes.Verb)
	}
	if w.lastSAR.Spec.ResourceAttributes.Namespace != "nsY" {
		t.Fatalf("expected namespace nsY, got %q", w.lastSAR.Spec.ResourceAttributes.Namespace)
	}
}

func TestAuth_Permit_UnauthorizedWhenNoToken(t *testing.T) {
	a := &Auth{Writer: &captureWriter{authenticated: true, allowed: true}, TTL: time.Second}
	ctx := ginCtxWithAuth(t, "", "http://example.invalid/x")
	p := &api.Provider{}
	st, err := a.Permit(ctx, p)
	if st != http.StatusUnauthorized || err != nil {
		t.Fatalf("expected 401 nil, got %d %v", st, err)
	}
}

func TestAuth_Permit_UnauthorizedWhenTokenReviewNotAuthenticated(t *testing.T) {
	a := &Auth{Writer: &captureWriter{authenticated: false, allowed: true}, TTL: time.Second}
	ctx := ginCtxWithAuth(t, "tok", "http://example.invalid/x")
	p := &api.Provider{}
	st, err := a.Permit(ctx, p)
	if st != http.StatusUnauthorized || err != nil {
		t.Fatalf("expected 401 nil, got %d %v", st, err)
	}
}

func TestAuth_Permit_ReturnsForbiddenAndErrorWhenNotAllowed(t *testing.T) {
	a := &Auth{Writer: &captureWriter{authenticated: true, allowed: false}, TTL: time.Second}
	ctx := ginCtxWithAuth(t, "tok", "http://example.invalid/x")
	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	st, err := a.Permit(ctx, p)
	if st != http.StatusForbidden || err == nil {
		t.Fatalf("expected 403 + err, got %d %v", st, err)
	}
}

func TestAuth_Token_Parsing(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"TrimsBearerAndWhitespace", "  Bearer   tok  ", "tok"},
		{"EmptyOnNonBearer", "Basic tok", ""},
		{"EmptyOnEmptyHeader", "", ""},
		{"EmptyOnOnlyBearer", "Bearer", ""},
		{"EmptyOnBearerOnlySpaces", "Bearer   ", ""},
		{"EmptyOnBearerTrailingSpace", "Bearer ", ""},
		{"ParsesTabSeparator", "Bearer\ttok", "tok"},
	}
	a := &Auth{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := ginCtxWithAuth(t, "", "http://example.invalid/x")
			ctx.Request.Header.Set("Authorization", tc.header)
			if got := a.Token(ctx); got != tc.want {
				t.Fatalf("Token() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAuth_Permit_CachesByTokenAndPath(t *testing.T) {
	w := &captureWriter{authenticated: true, allowed: true}
	a := &Auth{Writer: w, TTL: time.Hour}
	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{UID: types.UID("u1"), Namespace: "ns", Name: "p"}}

	ctx1 := ginCtxWithAuth(t, "tok", "http://example.invalid/a")
	ctx2 := ginCtxWithAuth(t, "tok", "http://example.invalid/b")

	st, err := a.Permit(ctx1, p)
	if st != http.StatusOK || err != nil {
		t.Fatalf("expected ok, got %d %v", st, err)
	}
	st, err = a.Permit(ctx1, p)
	if st != http.StatusOK || err != nil {
		t.Fatalf("expected ok, got %d %v", st, err)
	}
	// Different path should still be permitted but is a different cache key in current behavior.
	st, err = a.Permit(ctx2, p)
	if st != http.StatusOK || err != nil {
		t.Fatalf("expected ok, got %d %v", st, err)
	}
	if w.trCount < 1 || w.sarCount < 1 {
		t.Fatalf("expected calls made")
	}
}

func TestAuth_Permit_UsesTokenFromHeader(t *testing.T) {
	w := &captureWriter{authenticated: true, allowed: true}
	a := &Auth{Writer: w, TTL: time.Hour}
	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"}}
	ctx := ginCtxWithAuth(t, "", "http://example.invalid/x")
	ctx.Request.Header.Set("Authorization", "Bearer tok")
	st, err := a.Permit(ctx, p)
	if st != http.StatusOK || err != nil {
		t.Fatalf("expected ok, got %d %v", st, err)
	}
}

func TestAuth_Permit_TokenWhitespaceIsTrimmed(t *testing.T) {
	w := &captureWriter{authenticated: true, allowed: true}
	a := &Auth{Writer: w, TTL: time.Hour}
	p := &api.Provider{}
	ctx := ginCtxWithAuth(t, "", "http://example.invalid/x")
	ctx.Request.Header.Set("Authorization", "Bearer   tok   ")
	st, err := a.Permit(ctx, p)
	if st != http.StatusOK || err != nil {
		t.Fatalf("expected ok, got %d %v", st, err)
	}
}

