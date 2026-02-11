package base

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	providermodel "github.com/kubev2v/forklift/pkg/controller/provider/model/base"
	libcontainer "github.com/kubev2v/forklift/pkg/lib/inventory/container"
	libmodel "github.com/kubev2v/forklift/pkg/lib/inventory/model"
	libweb "github.com/kubev2v/forklift/pkg/lib/inventory/web"
	"github.com/kubev2v/forklift/pkg/settings"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ---- Consolidated from handler_more_test.go ----

type stubCollector struct {
	owner     *api.Provider
	hasParity bool
}

func (c stubCollector) Name() string                                     { return "stub" }
func (c stubCollector) Owner() metav1.Object                             { return c.owner }
func (c stubCollector) Start() error                                     { return nil }
func (c stubCollector) Shutdown()                                        {}
func (c stubCollector) HasParity() bool                                  { return c.hasParity }
func (c stubCollector) DB() libmodel.DB                                  { return nil }
func (c stubCollector) Test() (int, error)                               { return 0, nil }
func (c stubCollector) Follow(interface{}, []string, interface{}) error  { return nil }
func (c stubCollector) Reset()                                           {}
func (c stubCollector) Version() (string, string, string, string, error) { return "", "", "", "", nil }

func newGinCtx(t *testing.T, rawURL string) *gin.Context {
	t.Helper()
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	u, _ := url.Parse(rawURL)
	ctx.Request = &http.Request{URL: u, Header: http.Header{}}
	return ctx
}

func TestHandler_Token_EmptyWhenNoHeader(t *testing.T) {
	h := &Handler{}
	ctx := newGinCtx(t, "https://example.invalid/x")
	if got := h.Token(ctx); got != "" {
		t.Fatalf("expected empty token, got %q", got)
	}
}

func TestHandler_Token_ParsesBearer(t *testing.T) {
	h := &Handler{}
	ctx := newGinCtx(t, "https://example.invalid/x")
	ctx.Request.Header.Set("Authorization", "Bearer abc")
	if got := h.Token(ctx); got != "abc" {
		t.Fatalf("expected token abc, got %q", got)
	}
}

func TestHandler_Token_IgnoresNonBearer(t *testing.T) {
	h := &Handler{}
	ctx := newGinCtx(t, "https://example.invalid/x")
	ctx.Request.Header.Set("Authorization", "Basic abc")
	if got := h.Token(ctx); got != "" {
		t.Fatalf("expected empty token, got %q", got)
	}
}

func TestHandler_Prepare_EarlyReturnOnBadLimit(t *testing.T) {
	h := &Handler{}
	ctx := newGinCtx(t, "https://example.invalid/x?limit=not-a-number")
	if st, err := h.Prepare(ctx); st != http.StatusBadRequest || err != nil {
		t.Fatalf("expected 400 nil, got %d %v", st, err)
	}
}

func TestHandler_Prepare_EarlyReturnOnBadOffset(t *testing.T) {
	h := &Handler{}
	ctx := newGinCtx(t, "https://example.invalid/x?offset=-1")
	if st, err := h.Prepare(ctx); st != http.StatusBadRequest || err != nil {
		t.Fatalf("expected 400 nil, got %d %v", st, err)
	}
}

func TestHandler_Prepare_SetsWatchRequestAndSnapshot(t *testing.T) {
	h := &Handler{}
	ctx := newGinCtx(t, "https://example.invalid/x")
	ctx.Request.Header[libweb.WatchHeader] = []string{libweb.WatchSnapshot}
	if st, err := h.Prepare(ctx); st != http.StatusOK || err != nil {
		t.Fatalf("expected 200 nil, got %d %v", st, err)
	}
	if !h.WatchRequest {
		t.Fatalf("expected WatchRequest=true")
	}
}

func TestHandler_Prepare_EarlyReturnOnBadDetail(t *testing.T) {
	h := &Handler{}
	ctx := newGinCtx(t, "https://example.invalid/x?detail=bad")
	if st, err := h.Prepare(ctx); st != http.StatusBadRequest || err != nil {
		t.Fatalf("expected 400 nil, got %d %v", st, err)
	}
}

func TestHandler_Prepare_EarlyReturnOnUnknownProvider(t *testing.T) {
	h := &Handler{Container: libcontainer.New()}
	ctx := newGinCtx(t, "https://example.invalid/x")
	ctx.Params = gin.Params{{Key: ProviderParam, Value: "uid-unknown"}}
	if st, err := h.Prepare(ctx); st != http.StatusNotFound || err != nil {
		t.Fatalf("expected 404 nil, got %d %v", st, err)
	}
	if got := ctx.Writer.Header().Get(ReasonHeader); got != UnknownProvider {
		t.Fatalf("expected reason header %q, got %q", UnknownProvider, got)
	}
}

func TestHandler_Prepare_SuccessWhenAuthNotRequired(t *testing.T) {
	oldAuth := Settings.AuthRequired
	t.Cleanup(func() { Settings.AuthRequired = oldAuth })
	Settings.AuthRequired = false

	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{UID: types.UID("uid-1"), Namespace: "ns", Name: "p1"}}
	cont := libcontainer.New()
	_ = cont.Add(stubCollector{owner: p, hasParity: true})

	h := &Handler{Container: cont}
	ctx := newGinCtx(t, "https://example.invalid/x")
	ctx.Params = gin.Params{{Key: ProviderParam, Value: "uid-1"}}
	if st, err := h.Prepare(ctx); st != http.StatusOK || err != nil {
		t.Fatalf("expected 200 nil, got %d %v", st, err)
	}
	if h.Provider == nil || h.Provider.Name != "p1" {
		t.Fatalf("expected provider set, got %#v", h.Provider)
	}
}

func TestHandler_Prepare_AuthRequired_UnauthorizedWhenNoToken(t *testing.T) {
	oldAuth := Settings.AuthRequired
	oldWriter := DefaultAuth.Writer
	oldTTL := DefaultAuth.TTL
	oldCache := DefaultAuth.cache
	t.Cleanup(func() {
		Settings.AuthRequired = oldAuth
		DefaultAuth.Writer = oldWriter
		DefaultAuth.TTL = oldTTL
		DefaultAuth.cache = oldCache
	})
	Settings.AuthRequired = true
	DefaultAuth.Writer = &fakeWriter{allowed: true}
	DefaultAuth.TTL = time.Second
	DefaultAuth.cache = nil

	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{UID: types.UID("uid-2"), Namespace: "ns", Name: "p2"}}
	cont := libcontainer.New()
	_ = cont.Add(stubCollector{owner: p, hasParity: true})

	h := &Handler{Container: cont}
	ctx := newGinCtx(t, "https://example.invalid/x")
	ctx.Params = gin.Params{{Key: ProviderParam, Value: "uid-2"}}
	if st, err := h.Prepare(ctx); st != http.StatusUnauthorized || err != nil {
		t.Fatalf("expected 401 nil, got %d %v", st, err)
	}
}

func TestHandler_Prepare_AuthRequired_OKWhenAllowed(t *testing.T) {
	oldAuth := Settings.AuthRequired
	oldWriter := DefaultAuth.Writer
	oldTTL := DefaultAuth.TTL
	oldCache := DefaultAuth.cache
	t.Cleanup(func() {
		Settings.AuthRequired = oldAuth
		DefaultAuth.Writer = oldWriter
		DefaultAuth.TTL = oldTTL
		DefaultAuth.cache = oldCache
	})
	Settings.AuthRequired = true
	DefaultAuth.Writer = &fakeWriter{allowed: true}
	DefaultAuth.TTL = time.Second
	DefaultAuth.cache = nil

	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{UID: types.UID("uid-3"), Namespace: "ns", Name: "p3"}}
	cont := libcontainer.New()
	_ = cont.Add(stubCollector{owner: p, hasParity: true})

	h := &Handler{Container: cont}
	ctx := newGinCtx(t, "https://example.invalid/x")
	ctx.Request.Header.Set("Authorization", "Bearer tok")
	ctx.Params = gin.Params{{Key: ProviderParam, Value: "uid-3"}}
	if st, err := h.Prepare(ctx); st != http.StatusOK || err != nil {
		t.Fatalf("expected 200 nil, got %d %v", st, err)
	}
}

func TestHandler_Prepare_AuthRequired_UsesNamespaceQueryWhenProviderUIDEmpty(t *testing.T) {
	oldAuth := Settings.AuthRequired
	oldWriter := DefaultAuth.Writer
	oldTTL := DefaultAuth.TTL
	oldCache := DefaultAuth.cache
	t.Cleanup(func() {
		Settings.AuthRequired = oldAuth
		DefaultAuth.Writer = oldWriter
		DefaultAuth.TTL = oldTTL
		DefaultAuth.cache = oldCache
	})
	Settings.AuthRequired = true
	DefaultAuth.Writer = &fakeWriter{allowed: true}
	DefaultAuth.TTL = time.Second
	DefaultAuth.cache = nil

	h := &Handler{Container: libcontainer.New()}
	ctx := newGinCtx(t, "https://example.invalid/x?namespace=ns1")
	ctx.Request.Header.Set("Authorization", "Bearer tok")
	// No provider UID => list verb path inside Auth.Permit.
	if st, err := h.Prepare(ctx); st != http.StatusOK || err != nil {
		t.Fatalf("expected 200 nil, got %d %v", st, err)
	}
}

func TestHandler_Prepare_AuthRequired_ForbiddenReturnsError(t *testing.T) {
	oldAuth := Settings.AuthRequired
	oldWriter := DefaultAuth.Writer
	oldTTL := DefaultAuth.TTL
	oldCache := DefaultAuth.cache
	t.Cleanup(func() {
		Settings.AuthRequired = oldAuth
		DefaultAuth.Writer = oldWriter
		DefaultAuth.TTL = oldTTL
		DefaultAuth.cache = oldCache
	})
	Settings.AuthRequired = true
	DefaultAuth.Writer = &fakeWriter{allowed: false}
	DefaultAuth.TTL = time.Second
	DefaultAuth.cache = nil

	// Note: fakeWriter returns TokenReview.Authenticated=false as well,
	// so this ends up Unauthorized (not Forbidden). Still covers AuthRequired branch.
	h := &Handler{Container: libcontainer.New()}
	ctx := newGinCtx(t, "https://example.invalid/x?namespace=ns1")
	ctx.Request.Header.Set("Authorization", "Bearer tok")
	if st, err := h.Prepare(ctx); st != http.StatusUnauthorized || err != nil {
		t.Fatalf("expected 401 nil, got %d %v", st, err)
	}
}

func TestLink_SubstitutesParams(t *testing.T) {
	got := Link("/providers/:provider/things/:name", Params{"provider": "p1", "name": "n1"})
	if got != "/providers/p1/things/n1" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestHandler_Link_Delegates(t *testing.T) {
	h := &Handler{}
	got := h.Link("/providers/:provider", Params{"provider": "p1"})
	if got != "/providers/p1" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestHandler_setDetail_DefaultOK(t *testing.T) {
	h := &Handler{}
	ctx := newGinCtx(t, "https://example.invalid/x")
	if st := h.setDetail(ctx); st != http.StatusOK {
		t.Fatalf("expected OK, got %d", st)
	}
	if h.Detail != 0 {
		t.Fatalf("expected default detail 0, got %d", h.Detail)
	}
}

func TestHandler_setDetail_AllSetsMaxDetail(t *testing.T) {
	h := &Handler{}
	ctx := newGinCtx(t, "https://example.invalid/x?detail=all")
	if st := h.setDetail(ctx); st != http.StatusOK {
		t.Fatalf("expected OK, got %d", st)
	}
	if h.Detail != providermodel.MaxDetail {
		t.Fatalf("expected MaxDetail, got %d", h.Detail)
	}
}

func TestHandler_setDetail_NumberParses(t *testing.T) {
	h := &Handler{}
	ctx := newGinCtx(t, "https://example.invalid/x?detail=3")
	if st := h.setDetail(ctx); st != http.StatusOK {
		t.Fatalf("expected OK, got %d", st)
	}
	if h.Detail != 3 {
		t.Fatalf("expected 3, got %d", h.Detail)
	}
}

func TestHandler_setDetail_InvalidIsBadRequest(t *testing.T) {
	h := &Handler{}
	ctx := newGinCtx(t, "https://example.invalid/x?detail=not-a-number")
	if st := h.setDetail(ctx); st != http.StatusBadRequest {
		t.Fatalf("expected BadRequest, got %d", st)
	}
}

func TestHandler_setProvider_EmptyUID_OK(t *testing.T) {
	h := &Handler{
		Container: libcontainer.New(),
	}
	ctx := newGinCtx(t, "https://example.invalid/x")
	ctx.Params = gin.Params{} // no provider param
	if st := h.setProvider(ctx); st != http.StatusOK {
		t.Fatalf("expected OK, got %d", st)
	}
}

func TestHandler_setProvider_UnknownProvider_NotFoundWithReason(t *testing.T) {
	h := &Handler{
		Container: libcontainer.New(),
	}
	ctx := newGinCtx(t, "https://example.invalid/x")
	ctx.Params = gin.Params{{Key: ProviderParam, Value: "uid-1"}}
	if st := h.setProvider(ctx); st != http.StatusNotFound {
		t.Fatalf("expected NotFound, got %d", st)
	}
	if got := ctx.Writer.Header().Get(ReasonHeader); got != UnknownProvider {
		t.Fatalf("expected reason %q got %q", UnknownProvider, got)
	}
}

func TestHandler_setProvider_FoundProvider_OKAndSetsProvider(t *testing.T) {
	p := &api.Provider{ObjectMeta: metav1.ObjectMeta{UID: types.UID("uid-2"), Namespace: "ns", Name: "p1"}}
	coll := stubCollector{owner: p, hasParity: true}
	cont := libcontainer.New()
	_ = cont.Add(coll) // Start is no-op; adds to map.

	h := &Handler{
		Container: cont,
	}
	ctx := newGinCtx(t, "https://example.invalid/x")
	ctx.Params = gin.Params{{Key: ProviderParam, Value: "uid-2"}}
	if st := h.setProvider(ctx); st != http.StatusOK {
		t.Fatalf("expected OK, got %d", st)
	}
	if h.Provider == nil || h.Provider.Name != "p1" {
		t.Fatalf("expected provider set, got %#v", h.Provider)
	}
}

func TestHandler_Permit_WhenAuthNotRequired_OK(t *testing.T) {
	old := settings.Settings.Inventory.AuthRequired
	t.Cleanup(func() { settings.Settings.Inventory.AuthRequired = old })
	settings.Settings.Inventory.AuthRequired = false

	h := &Handler{}
	ctx := newGinCtx(t, "https://example.invalid/x")
	if st, err := h.permit(ctx); st != http.StatusOK || err != nil {
		t.Fatalf("expected OK nil, got %d %v", st, err)
	}
}

func TestHandler_PathMatch_SuffixMatch(t *testing.T) {
	h := &Handler{}
	if !h.PathMatch("/dc/cluster/networks/net1", "/networks/net1") {
		t.Fatalf("expected match")
	}
}

func TestHandler_PathMatch_NoMatch(t *testing.T) {
	h := &Handler{}
	if h.PathMatch("/a/b/c", "/x/c") {
		t.Fatalf("expected no match")
	}
}

func TestHandler_PathMatchRoot_SameRoot(t *testing.T) {
	h := &Handler{}
	if !h.PathMatchRoot("/dc1/cluster/a", "/dc1/networks/b") {
		t.Fatalf("expected match root")
	}
}

func TestHandler_PathMatchRoot_DifferentRoot(t *testing.T) {
	h := &Handler{}
	if h.PathMatchRoot("/dc1/cluster/a", "/dc2/networks/b") {
		t.Fatalf("expected different root")
	}
}

func TestHandler_PathMatch_TrimsLeadingSlashes(t *testing.T) {
	h := &Handler{}
	if !h.PathMatch("///a/b/c", "///b/c") {
		t.Fatalf("expected match")
	}
}

func TestHandler_PathMatchRoot_TrimsLeadingSlashes(t *testing.T) {
	h := &Handler{}
	if !h.PathMatchRoot("///dc1/cluster/a", "/dc1/networks/b") {
		t.Fatalf("expected match root")
	}
}
