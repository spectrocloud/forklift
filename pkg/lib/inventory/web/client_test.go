package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"time"

	"github.com/gorilla/websocket"
	"github.com/kubev2v/forklift/pkg/lib/inventory/model"
	"github.com/kubev2v/forklift/pkg/lib/logging"
)

func TestClient_Get_InvalidURL(t *testing.T) {
	c := &Client{}
	var out map[string]any
	if _, err := c.Get("://bad-url", &out); err == nil {
		t.Fatalf("expected error")
	}
}

func TestClient_Get_And_Post_JSON(t *testing.T) {
	type resp struct {
		Value string `json:"value"`
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(resp{Value: r.URL.Query().Get("q")})
	})
	mux.HandleFunc("/post", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		_ = json.NewEncoder(w).Encode(in)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{Header: http.Header{}}

	var got resp
	status, err := c.Get(srv.URL+"/get", &got, Param{Key: "q", Value: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusOK || got.Value != "x" {
		t.Fatalf("unexpected response: status=%d got=%#v", status, got)
	}

	var posted map[string]any
	status, err = c.Post(srv.URL+"/post", map[string]any{"k": "v"}, &posted)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusOK || posted["k"] != "v" {
		t.Fatalf("unexpected post response: status=%d got=%#v", status, posted)
	}
}

func TestClient_Get_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{not-json"))
	}))
	t.Cleanup(srv.Close)

	c := &Client{Header: http.Header{}}
	var out map[string]any
	if _, err := c.Get(srv.URL, &out); err == nil {
		t.Fatalf("expected error")
	}
}

func TestClient_patchURL(t *testing.T) {
	c := &Client{}
	if got := c.patchURL("http://example.invalid/x"); got != "ws://example.invalid/x" {
		t.Fatalf("unexpected patched url: %q", got)
	}
	if got := c.patchURL("https://example.invalid/x"); got != "wss://example.invalid/x" {
		t.Fatalf("unexpected patched url: %q", got)
	}
	// Unsupported scheme / invalid URL => unchanged.
	if got := c.patchURL("ftp://example.invalid/x"); got != "ftp://example.invalid/x" {
		t.Fatalf("expected unchanged, got %q", got)
	}
	if got := c.patchURL("://bad-url"); got != "://bad-url" {
		t.Fatalf("expected unchanged, got %q", got)
	}
}

func TestClient_Post_RawStringAndNonOK(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/raw", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		if string(b) != "raw-body" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	mux.HandleFunc("/nonok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ignored":true}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{Header: http.Header{}}
	var out map[string]any
	status, err := c.Post(srv.URL+"/raw", "raw-body", &out)
	if err != nil || status != http.StatusOK || out["ok"] != true {
		t.Fatalf("unexpected: status=%d err=%v out=%#v", status, err, out)
	}

	// non-OK returns status and no unmarshal attempt
	status, err = c.Post(srv.URL+"/nonok", map[string]any{"x": "y"}, nil)
	if err != nil || status != http.StatusAccepted {
		t.Fatalf("unexpected: status=%d err=%v", status, err)
	}
}

// ---- Consolidated from client_more_test.go ----

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type errReadCloser struct{}

func (e *errReadCloser) Read([]byte) (int, error) { return 0, errors.New("readfail") }
func (e *errReadCloser) Close() error             { return nil }

func TestClient_Get_TransportError_Wrapped(t *testing.T) {
	c := &Client{
		Header: http.Header{},
		Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("boom")
		}),
	}
	var out map[string]any
	_, err := c.Get("http://example.invalid/x", &out)
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestClient_Get_ReadBodyError_Wrapped(t *testing.T) {
	c := &Client{
		Header: http.Header{},
		Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"X": []string{"y"}},
				Body:       &errReadCloser{},
			}, nil
		}),
	}
	var out map[string]any
	_, err := c.Get("http://example.invalid/x", &out)
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestClient_Get_NonOK_DoesNotUnmarshal(t *testing.T) {
	c := &Client{
		Header: http.Header{},
		Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusAccepted,
				Header:     http.Header{"X": []string{"y"}},
				Body:       io.NopCloser(io.LimitReader(io.TeeReader(io.NopCloser(nil), io.Discard), 0)),
			}, nil
		}),
	}
	out := map[string]any{"k": "v"}
	status, err := c.Get("http://example.invalid/x", &out)
	if err != nil || status != http.StatusAccepted {
		t.Fatalf("unexpected: status=%d err=%v", status, err)
	}
	// Should remain unchanged (no unmarshal on non-OK).
	if out["k"] != "v" {
		t.Fatalf("expected unchanged")
	}
}

func TestClient_Get_SetsReplyHeaders(t *testing.T) {
	c := &Client{
		Header: http.Header{},
		Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusAccepted,
				Header:     http.Header{"X-Reply": []string{"1"}},
				Body:       io.NopCloser(&errReadCloser{}), // not read when non-OK? actually still read; use empty reader
			}, nil
		}),
	}
	var out map[string]any
	// Use 204 so body read is still attempted; ensure we avoid errReadCloser by providing empty.
	c.Transport = rtFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Header:     http.Header{"X-Reply": []string{"1"}},
			Body:       io.NopCloser(bytesReader{}),
		}, nil
	})
	_, _ = c.Get("http://example.invalid/x", &out)
	if c.Reply.Header.Get("X-Reply") != "1" {
		t.Fatalf("expected reply header")
	}
}

type bytesReader struct{}

func (bytesReader) Read(p []byte) (int, error) { return 0, io.EOF }

func TestClient_Post_InvalidURL_Err(t *testing.T) {
	c := &Client{}
	_, err := c.Post("://bad", map[string]any{}, nil)
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestClient_Post_TransportError_Wrapped(t *testing.T) {
	c := &Client{
		Header: http.Header{},
		Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("boom")
		}),
	}
	_, err := c.Post("http://example.invalid/x", map[string]any{}, nil)
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestClient_Post_ReadBodyError_Wrapped(t *testing.T) {
	c := &Client{
		Header: http.Header{},
		Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       &errReadCloser{},
				Header:     http.Header{},
			}, nil
		}),
	}
	_, err := c.Post("http://example.invalid/x", map[string]any{}, &map[string]any{})
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestClient_Post_OK_OutNil_NoUnmarshal(t *testing.T) {
	c := &Client{
		Header: http.Header{},
		Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytesReader{}),
				Header:     http.Header{},
			}, nil
		}),
	}
	status, err := c.Post("http://example.invalid/x", map[string]any{}, nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("unexpected: %d %v", status, err)
	}
}

func TestClient_Watch_PatchesSchemeAndPropagatesHeadersAndSnapshot(t *testing.T) {
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	var gotWatchHeader string
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotWatchHeader = r.Header.Get(WatchHeader)
		gotAuth = r.Header.Get("Authorization")
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Immediately end so client doesn't hang.
		_ = conn.WriteJSON(Event{Action: model.End})
	}))
	t.Cleanup(srv.Close)

	type R struct{ A string }
	h := &recHandler{opts: WatchOptions{Snapshot: true}}
	h.ensure()
	c := &Client{Header: http.Header{"Authorization": []string{"Bearer x"}}}
	status, wch, err := c.Watch(srv.URL, &R{}, h)
	if err != nil || status != http.StatusOK || wch == nil {
		t.Fatalf("unexpected: status=%d err=%v w=%v", status, err, wch)
	}
	// Wait for end.
	ended := h.ended
	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout")
	}
	if gotWatchHeader != WatchSnapshot {
		t.Fatalf("expected %q got %q", WatchSnapshot, gotWatchHeader)
	}
	if gotAuth != "Bearer x" {
		t.Fatalf("expected auth propagated")
	}
}

func TestClient_Watch_NonOKStatus_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	type R struct{ A string }
	h := &recHandler{opts: WatchOptions{}}
	h.ensure()
	c := &Client{Header: http.Header{}}
	status, wch, err := c.Watch(srv.URL, &R{}, h)
	if err == nil || status != http.StatusNotFound || wch != nil {
		t.Fatalf("expected err/status/w=nil, got status=%d err=%v w=%v", status, err, wch)
	}
}

func TestClient_WatchReader_clone_PreservesValue(t *testing.T) {
	type R struct{ A string }
	r := &WatchReader{}
	in := &R{A: "x"}
	out := r.clone(in).(*R)
	if out == in || out.A != "x" {
		t.Fatalf("expected cloned copy")
	}
}

func TestClient_WatchReader_Terminate_SetsDone(t *testing.T) {
	// Use a real websocket to avoid nil deref in Terminate().
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Wait a bit so client can close.
		time.Sleep(200 * time.Millisecond)
	}))
	t.Cleanup(srv.Close)

	wsURL := (&Client{}).patchURL(srv.URL)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	reader := &WatchReader{webSocket: conn, log: logging.WithName("test")}
	reader.Terminate()
	if !reader.done {
		t.Fatalf("expected done")
	}
}

type recHandler struct {
	StockEventHandler
	opts WatchOptions

	mu sync.Mutex

	started chan uint64
	parity  chan struct{}
	created chan Event
	updated chan Event
	deleted chan Event
	errors  chan error
	ended   chan struct{}
}

func (r *recHandler) Options() WatchOptions { return r.opts }
func (r *recHandler) Started(id uint64) {
	r.ensure()
	r.started <- id
}
func (r *recHandler) Parity() {
	r.ensure()
	r.parity <- struct{}{}
}
func (r *recHandler) Created(e Event) {
	r.ensure()
	r.created <- e
}
func (r *recHandler) Updated(e Event) {
	r.ensure()
	r.updated <- e
}
func (r *recHandler) Deleted(e Event) {
	r.ensure()
	r.deleted <- e
}
func (r *recHandler) Error(_ *Watch, err error) {
	r.ensure()
	r.errors <- err
}
func (r *recHandler) End() {
	r.ensure()
	close(r.ended)
}

func (r *recHandler) ensure() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ended != nil {
		return
	}
	r.started = make(chan uint64, 10)
	r.parity = make(chan struct{}, 10)
	r.created = make(chan Event, 10)
	r.updated = make(chan Event, 10)
	r.deleted = make(chan Event, 10)
	r.errors = make(chan error, 10)
	r.ended = make(chan struct{})
}
