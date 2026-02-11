package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kubev2v/forklift/pkg/lib/inventory/container"
	"github.com/kubev2v/forklift/pkg/lib/inventory/model"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type stubCollector struct {
	parity bool
}

func (s *stubCollector) Name() string                                    { return "stub" }
func (s *stubCollector) Owner() metav1.Object                            { return &metav1.ObjectMeta{Name: "o"} }
func (s *stubCollector) Start() error                                    { return nil }
func (s *stubCollector) Shutdown()                                       {}
func (s *stubCollector) HasParity() bool                                 { return s.parity }
func (s *stubCollector) DB() model.DB                                    { return nil }
func (s *stubCollector) Test() (int, error)                              { return 0, nil }
func (s *stubCollector) Follow(interface{}, []string, interface{}) error { return nil }
func (s *stubCollector) Reset()                                          {}
func (s *stubCollector) Version() (string, string, string, string, error) {
	return "", "", "", "", nil
}

var _ container.Collector = (*stubCollector)(nil)

func TestPaged_Prepare_SetsDefaultsAndValidates(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("defaults", func(t *testing.T) {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest(http.MethodGet, "http://example.invalid/x", nil)
		ctx.Request = req
		h := &Paged{}
		if status := h.Prepare(ctx); status != http.StatusOK {
			t.Fatalf("expected 200, got %d", status)
		}
		if h.Page.Offset != 0 || h.Page.Limit <= 0 {
			t.Fatalf("unexpected page: %#v", h.Page)
		}
	})

	t.Run("valid params", func(t *testing.T) {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest(http.MethodGet, "http://example.invalid/x?limit=10&offset=2", nil)
		ctx.Request = req
		h := &Paged{}
		if status := h.Prepare(ctx); status != http.StatusOK {
			t.Fatalf("expected 200, got %d", status)
		}
		if h.Page.Limit != 10 || h.Page.Offset != 2 {
			t.Fatalf("unexpected page: %#v", h.Page)
		}
	})

	t.Run("invalid limit", func(t *testing.T) {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest(http.MethodGet, "http://example.invalid/x?limit=-1", nil)
		ctx.Request = req
		h := &Paged{}
		if status := h.Prepare(ctx); status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", status)
		}
	})
}

func TestParity_EnsureParity(t *testing.T) {
	p := &Parity{}
	c := &stubCollector{parity: true}
	if status := p.EnsureParity(c, 0); status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}

	c2 := &stubCollector{parity: false}
	if status := p.EnsureParity(c2, time.Millisecond); status != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", status)
	}
}

func TestEvent_String(t *testing.T) {
	e := &Event{ID: 12, Action: model.Created, Resource: &struct{}{}}
	s := e.String()
	if s == "" || s[:6] != "event-" {
		t.Fatalf("unexpected string: %q", s)
	}
}

// ---- Consolidated from handler_more_test.go ----

func TestWatched_Prepare_NoHeader(t *testing.T) {
	w := &Watched{}
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "http://example.invalid/x", nil)

	if st := w.Prepare(ctx); st != http.StatusOK {
		t.Fatalf("expected ok, got %d", st)
	}
	if w.WatchRequest {
		t.Fatalf("expected WatchRequest=false")
	}
	if w.options.Snapshot {
		t.Fatalf("expected Snapshot=false")
	}
}

func TestWatched_Prepare_SnapshotOption(t *testing.T) {
	w := &Watched{}
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "http://example.invalid/x", nil)
	req.Header.Add(WatchHeader, WatchSnapshot)
	ctx.Request = req

	if st := w.Prepare(ctx); st != http.StatusOK {
		t.Fatalf("expected ok, got %d", st)
	}
	if !w.WatchRequest {
		t.Fatalf("expected WatchRequest=true")
	}
	if !w.options.Snapshot {
		t.Fatalf("expected Snapshot=true")
	}
}

func TestWatched_Prepare_UnknownOptionIgnored(t *testing.T) {
	w := &Watched{}
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "http://example.invalid/x", nil)
	req.Header.Add(WatchHeader, "unknown")
	ctx.Request = req

	_ = w.Prepare(ctx)
	if !w.WatchRequest {
		t.Fatalf("expected WatchRequest=true")
	}
	if w.options.Snapshot {
		t.Fatalf("expected Snapshot=false")
	}
}

func TestWatched_Watch_UpgradeFails_ReturnsError(t *testing.T) {
	w := &Watched{}
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "http://example.invalid/x", nil)

	err := w.Watch(ctx, nil, nil, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestSchemaHandler_AddRoutes_SetsRouter(t *testing.T) {
	r := gin.New()
	h := &SchemaHandler{}
	h.AddRoutes(r)
	if h.router == nil {
		t.Fatalf("expected router set")
	}
}

func TestSchemaHandler_List_ReturnsPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &SchemaHandler{Version: "v1", Release: 2}
	h.AddRoutes(r)
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodGet, "/schema", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rec.Code)
	}
	var payload struct {
		Version string   `json:"version"`
		Release int      `json:"release"`
		Paths   []string `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Version != "v1" || payload.Release != 2 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	joined := strings.Join(payload.Paths, " ")
	if !strings.Contains(joined, "/schema") {
		t.Fatalf("expected /schema in paths, got %#v", payload.Paths)
	}
	if !strings.Contains(joined, "/x") {
		t.Fatalf("expected /x in paths, got %#v", payload.Paths)
	}
}

func TestSchemaHandler_Get_MethodNotAllowed(t *testing.T) {
	r := gin.New()
	h := &SchemaHandler{}
	h.AddRoutes(r)
	r.GET("/get", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/get", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 got %d", rec.Code)
	}
}

func TestWatchWriter_Options_ReturnsOptions(t *testing.T) {
	ww := &WatchWriter{options: model.WatchOptions{Snapshot: true}}
	if !ww.Options().Snapshot {
		t.Fatalf("expected snapshot true")
	}
}

func TestWatchWriter_Send_DoneEarlyReturn_NoPanic(t *testing.T) {
	ww := &WatchWriter{done: true}
	ww.send(model.Event{Action: model.Created})
}

func TestWatchWriter_Started_DoneEarlyReturn_NoPanic(t *testing.T) {
	ww := &WatchWriter{done: true, log: logging.WithName("t")}
	ww.Started(1)
}

func TestWatchWriter_Parity_DoneEarlyReturn_NoPanic(t *testing.T) {
	ww := &WatchWriter{done: true, log: logging.WithName("t")}
	ww.Parity()
}

func TestWatchWriter_Created_DoneEarlyReturn_NoPanic(t *testing.T) {
	ww := &WatchWriter{done: true, log: logging.WithName("t")}
	ww.Created(model.Event{Action: model.Created})
}

func TestWatchWriter_Updated_DoneEarlyReturn_NoPanic(t *testing.T) {
	ww := &WatchWriter{done: true, log: logging.WithName("t")}
	ww.Updated(model.Event{Action: model.Updated})
}

func TestWatchWriter_Deleted_DoneEarlyReturn_NoPanic(t *testing.T) {
	ww := &WatchWriter{done: true, log: logging.WithName("t")}
	ww.Deleted(model.Event{Action: model.Deleted})
}

func TestWatchWriter_Error_DoneEarlyReturn_NoPanic(t *testing.T) {
	ww := &WatchWriter{done: true, log: logging.WithName("t")}
	ww.Error(errors.New("boom"))
}

func TestWatchWriter_End_ClosesWebsocket(t *testing.T) {
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		// Drain until closed.
		for {
			_, _, err := c.ReadMessage()
			if err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	ww := &WatchWriter{
		webSocket: conn,
		builder:   func(m model.Model) interface{} { return nil },
		log:       logging.WithName("t"),
	}
	ww.End()
	if !ww.done {
		t.Fatalf("expected done=true")
	}
}

func TestEvent_String_Unknown(t *testing.T) {
	e := &Event{ID: 1, Action: 255}
	if s := e.String(); !strings.Contains(s, "unknown") {
		t.Fatalf("unexpected: %q", s)
	}
}

func TestEvent_String_Started(t *testing.T) {
	e := &Event{ID: 1, Action: model.Started}
	if s := e.String(); !strings.Contains(s, "started") {
		t.Fatalf("unexpected: %q", s)
	}
}

func TestEvent_String_Parity(t *testing.T) {
	e := &Event{ID: 1, Action: model.Parity}
	if s := e.String(); !strings.Contains(s, "parity") {
		t.Fatalf("unexpected: %q", s)
	}
}

func TestEvent_String_Error(t *testing.T) {
	e := &Event{ID: 1, Action: model.Error}
	if s := e.String(); !strings.Contains(s, "error") {
		t.Fatalf("unexpected: %q", s)
	}
}

func TestEvent_String_End(t *testing.T) {
	e := &Event{ID: 1, Action: model.End}
	if s := e.String(); !strings.Contains(s, "end") {
		t.Fatalf("unexpected: %q", s)
	}
}

func TestEvent_String_Created(t *testing.T) {
	e := &Event{ID: 1, Action: model.Created}
	if s := e.String(); !strings.Contains(s, "created") {
		t.Fatalf("unexpected: %q", s)
	}
}

func TestEvent_String_Updated(t *testing.T) {
	e := &Event{ID: 1, Action: model.Updated}
	if s := e.String(); !strings.Contains(s, "updated") {
		t.Fatalf("unexpected: %q", s)
	}
}

func TestEvent_String_Deleted(t *testing.T) {
	e := &Event{ID: 1, Action: model.Deleted}
	if s := e.String(); !strings.Contains(s, "deleted") {
		t.Fatalf("unexpected: %q", s)
	}
}

// ---- Consolidated from web_more_test.go ----

type stubHandler struct {
	called int
}

func (s *stubHandler) AddRoutes(*gin.Engine) { s.called++ }

func TestWebServer_address_DefaultsTo8080WhenNoTLS(t *testing.T) {
	w := &WebServer{}
	got := w.address()
	if got != ":8080" {
		t.Fatalf("expected :8080 got %q", got)
	}
}

func TestWebServer_address_DefaultsTo8443WhenTLS(t *testing.T) {
	w := &WebServer{}
	w.TLS.Enabled = true
	got := w.address()
	if got != ":8443" {
		t.Fatalf("expected :8443 got %q", got)
	}
}

func TestWebServer_address_UsesExplicitPort(t *testing.T) {
	w := &WebServer{Port: 1234}
	got := w.address()
	if got != ":1234" {
		t.Fatalf("expected :1234 got %q", got)
	}
}

func TestWebServer_buildOrigins_SkipsInvalidRegex(t *testing.T) {
	w := &WebServer{AllowedOrigins: []string{"[", "^https://ok\\.example$"}}
	w.buildOrigins()
	if len(w.allowedOrigins) != 1 {
		t.Fatalf("expected 1 got %d", len(w.allowedOrigins))
	}
	if !w.allow("https://ok.example") {
		t.Fatalf("expected allowed")
	}
	if w.allow("https://no.example") {
		t.Fatalf("expected not allowed")
	}
}

func TestWebServer_allow_FalseWhenNoOrigins(t *testing.T) {
	w := &WebServer{}
	w.buildOrigins()
	if w.allow("https://x") {
		t.Fatalf("expected false")
	}
}

func TestWebServer_addRoutes_CallsHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &WebServer{}
	h1 := &stubHandler{}
	h2 := &stubHandler{}
	w.Handlers = []RequestHandler{h1, h2}
	r := gin.New()
	w.addRoutes(r)
	if h1.called != 1 || h2.called != 1 {
		t.Fatalf("expected handlers called")
	}
}
