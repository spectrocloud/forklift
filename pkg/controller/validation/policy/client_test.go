package policy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	refapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	model "github.com/kubev2v/forklift/pkg/controller/provider/model/base"
)

func TestClient_EnabledAndDisabled(t *testing.T) {
	orig := *Settings
	t.Cleanup(func() { *Settings = orig })

	c := &Client{}

	Settings.PolicyAgent.URL = ""
	if c.Enabled() {
		t.Fatalf("expected disabled when URL unset")
	}

	Settings.PolicyAgent.URL = "http://example.invalid"
	if !c.Enabled() {
		t.Fatalf("expected enabled when URL set")
	}
}

func TestClient_Version_DisabledIsNoop(t *testing.T) {
	orig := *Settings
	t.Cleanup(func() { *Settings = orig })

	Settings.PolicyAgent.URL = ""

	c := &Client{}
	v, err := c.Version("/version")
	if err != nil || v != 0 {
		t.Fatalf("expected noop (0,nil), got (%d,%v)", v, err)
	}
}

func TestClient_Version_SuccessAndNon200(t *testing.T) {
	orig := *Settings
	t.Cleanup(func() { *Settings = orig })

	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{
				"rules_version": 123,
			},
		})
	})
	mux.HandleFunc("/bad", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	Settings.PolicyAgent.URL = srv.URL

	c := &Client{}
	v, err := c.Version("/version")
	if err != nil || v != 123 {
		t.Fatalf("expected (123,nil), got (%d,%v)", v, err)
	}

	_, err = c.Version("/bad")
	if err == nil {
		t.Fatalf("expected error on non-200")
	}
}

func TestClient_Validate_SuccessAndValidationError(t *testing.T) {
	orig := *Settings
	t.Cleanup(func() { *Settings = orig })

	mux := http.NewServeMux()
	mux.HandleFunc("/validate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{
				"rules_version": 7,
				"concerns": []model.Concern{
					{Id: "c1", Category: "Info", Label: "l1", Assessment: "a1"},
				},
				"errors": []string{},
			},
		})
	})
	mux.HandleFunc("/validate-error", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{
				"rules_version": 8,
				"concerns":      []model.Concern{},
				"errors":        []string{"bad input"},
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	Settings.PolicyAgent.URL = srv.URL

	c := &Client{}
	v, concerns, err := c.Validate("/validate", map[string]any{"k": "v"})
	if err != nil || v != 7 || len(concerns) != 1 || concerns[0].Id != "c1" {
		t.Fatalf("unexpected result: v=%d concerns=%#v err=%v", v, concerns, err)
	}

	_, _, err = c.Validate("/validate-error", map[string]any{"k": "v"})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Errors) != 1 || ve.Errors[0] != "bad input" {
		t.Fatalf("expected ValidationError, got: %#v (err=%v)", ve, err)
	}
}

func TestClient_Get_InvalidBaseURL(t *testing.T) {
	orig := *Settings
	t.Cleanup(func() { *Settings = orig })

	Settings.PolicyAgent.URL = "%%%" // invalid URL

	c := &Client{}
	_, err := c.Version("/version")
	if err == nil {
		t.Fatalf("expected error on invalid URL")
	}
}

func TestClient_BuildTransport_CAAndDevelopment(t *testing.T) {
	orig := *Settings
	t.Cleanup(func() { *Settings = orig })

	Settings.PolicyAgent.URL = "http://example.invalid"

	// CA path branch.
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	// Not necessarily a valid CA PEM; buildTransport doesn't fail on append=false.
	if err := os.WriteFile(caPath, []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"), 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}
	Settings.PolicyAgent.TLS.CA = caPath
	Settings.Development = false

	c := &Client{}
	if err := c.buildTransport(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Transport == nil {
		t.Fatalf("expected transport to be set")
	}

	// Development branch (no CA).
	Settings.PolicyAgent.TLS.CA = ""
	Settings.Development = true
	c2 := &Client{}
	if err := c2.buildTransport(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tr, ok := c2.Transport.(*http.Transport)
	if !ok || tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("expected insecure TLS transport in development, got %#v", c2.Transport)
	}
}

func TestPool_SubmitAndResult_NoErrors(t *testing.T) {
	orig := *Settings
	t.Cleanup(func() { *Settings = orig })

	mux := http.NewServeMux()
	mux.HandleFunc("/validate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{
				"rules_version": 99,
				"concerns":      []model.Concern{},
				"errors":        []string{},
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	Settings.PolicyAgent.URL = srv.URL
	Settings.PolicyAgent.Limit.Worker = 1

	p := &Pool{Client: Client{}}

	// Submit before start => error.
	err := p.Submit(&Task{Result: make(chan *Task, 1), Context: context.Background()})
	if err == nil {
		t.Fatalf("expected submit error when pool not started")
	}

	// Start should be idempotent.
	p.Start()
	p.Start()
	t.Cleanup(p.Shutdown)

	result := make(chan *Task, 1)
	task := &Task{
		Path:     "/validate",
		Ref:      refapi.Ref{ID: "vm-1"},
		Revision: 1,
		Context:  context.Background(),
		Workload: func(string) (interface{}, error) { return map[string]any{"ok": true}, nil },
		Result:   result,
	}
	if err := p.Submit(task); err != nil {
		t.Fatalf("submit: %v", err)
	}

	select {
	case got := <-result:
		if got == nil || got.Version != 99 || got.Error != nil {
			t.Fatalf("unexpected task result: %#v", got)
		}
		if got.Worker() != 0 {
			t.Fatalf("expected worker 0, got %d", got.Worker())
		}
		// Duration should be >= 0 and non-zero-ish.
		if got.Duration() < 0*time.Second {
			t.Fatalf("unexpected duration: %s", got.Duration())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for task result")
	}

	// Backlog should be small (no pending work).
	if p.Backlog() < 0 {
		t.Fatalf("unexpected backlog: %d", p.Backlog())
	}
}
