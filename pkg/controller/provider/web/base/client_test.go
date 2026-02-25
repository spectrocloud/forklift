package base

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	providermodel "github.com/kubev2v/forklift/pkg/controller/provider/model/base"
	libmodel "github.com/kubev2v/forklift/pkg/lib/inventory/model"
	libweb "github.com/kubev2v/forklift/pkg/lib/inventory/web"
	"github.com/kubev2v/forklift/pkg/settings"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type stubResolver struct {
	path string
	err  error
}

// ---- Consolidated from tree_more_test.go ----

type tm struct{ id string }

func (m tm) Pk() string { return m.id }

type nb struct{}

func (nb) Node(p *TreeNode, m libmodel.Model) *TreeNode {
	n := &TreeNode{
		Parent: p,
		Kind:   "tm",
		Object: m.Pk(),
	}
	return n
}

type branchNav struct {
	next map[string][]libmodel.Model
	err  error
}

func (n branchNav) Next(m libmodel.Model) ([]libmodel.Model, error) {
	if n.err != nil {
		return nil, n.err
	}
	return n.next[m.Pk()], nil
}

type parentNav struct {
	next map[string]libmodel.Model
	err  error
}

func (n parentNav) Next(m libmodel.Model) (libmodel.Model, error) {
	if n.err != nil {
		return nil, n.err
	}
	return n.next[m.Pk()], nil
}

func TestTree_Build_Basic(t *testing.T) {
	root := tm{id: "root"}
	n := branchNav{
		next: map[string][]libmodel.Model{
			"root": {tm{id: "a"}, tm{id: "b"}},
			"a":    {tm{id: "a1"}},
		},
	}
	tr := &Tree{NodeBuilder: nb{}, Depth: 0}

	tree, err := tr.Build(root, n)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if tree.Object != "root" {
		t.Fatalf("unexpected root: %#v", tree)
	}
	if len(tree.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(tree.Children))
	}
}

func TestTree_Build_DepthLimit(t *testing.T) {
	root := tm{id: "root"}
	n := branchNav{
		next: map[string][]libmodel.Model{
			"root": {tm{id: "a"}},
			"a":    {tm{id: "a1"}},
		},
	}
	tr := &Tree{NodeBuilder: nb{}, Depth: 1}
	tree, err := tr.Build(root, n)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(tree.Children) != 1 {
		t.Fatalf("expected 1 child")
	}
	// depth=1 should stop before adding grandchildren.
	if len(tree.Children[0].Children) != 0 {
		t.Fatalf("expected no grandchildren due to depth limit")
	}
}

func TestTree_Build_PropagatesNavigatorError(t *testing.T) {
	root := tm{id: "root"}
	sentinel := errors.New("boom")
	tr := &Tree{NodeBuilder: nb{}, Depth: 0}
	_, err := tr.Build(root, branchNav{err: sentinel})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel, got %v", err)
	}
}

func TestTree_Ancestry_BuildsChain(t *testing.T) {
	leaf := tm{id: "leaf"}
	n := parentNav{
		next: map[string]libmodel.Model{
			"leaf": tm{id: "p1"},
			"p1":   tm{id: "p2"},
			"p2":   nil,
		},
	}
	tr := &Tree{NodeBuilder: nb{}, Depth: 0}
	tree, err := tr.Ancestry(leaf, n)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if tree.Object != "p2" {
		t.Fatalf("expected root p2, got %#v", tree)
	}
	if len(tree.Children) != 1 || tree.Children[0].Object != "p1" {
		t.Fatalf("unexpected children: %#v", tree.Children)
	}
	if tree.Children[0].Children[0].Object != "leaf" {
		t.Fatalf("expected leaf, got %#v", tree.Children[0].Children)
	}
}

func TestTree_Ancestry_PropagatesError(t *testing.T) {
	leaf := tm{id: "leaf"}
	sentinel := errors.New("boom")
	tr := &Tree{NodeBuilder: nb{}, Depth: 0}
	_, err := tr.Ancestry(leaf, parentNav{err: sentinel})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel, got %v", err)
	}
}

// Ensure our stub types satisfy the expected interfaces at compile time.
var _ providermodel.BranchNavigator = branchNav{}
var _ providermodel.ParentNavigator = parentNav{}

func (r stubResolver) Path(resource interface{}, id string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	return r.path, nil
}

func writeKubeconfig(t *testing.T, dir string, token string) string {
	t.Helper()
	cfg := `apiVersion: v1
kind: Config
clusters:
- name: c
  cluster:
    server: https://example.invalid
    insecure-skip-tls-verify: true
contexts:
- name: ctx
  context:
    cluster: c
    user: u
current-context: ctx
users:
- name: u
  user:
    token: ` + token + `
`
	p := filepath.Join(dir, "kubeconfig.yaml")
	if err := os.WriteFile(p, []byte(cfg), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return p
}

func TestErrors_ErrorString(t *testing.T) {
	if s := (ResourceNotResolvedError{Object: "x"}).Error(); !strings.Contains(s, "cannot be resolved") {
		t.Fatalf("unexpected msg: %q", s)
	}
	if s := (RefNotUniqueError{Ref: Ref{ID: "1"}}).Error(); !strings.Contains(s, "matched multiple") {
		t.Fatalf("unexpected msg: %q", s)
	}
	if s := (NotFoundError{Ref: Ref{ID: "1"}}).Error(); !strings.Contains(s, "not found") {
		t.Fatalf("unexpected msg: %q", s)
	}
}

func TestRestClient_Get_ResolverNil(t *testing.T) {
	c := &RestClient{}
	var out struct{}
	if _, err := c.Get(&out, "id"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRestClient_Get_ResourceMustBePtr(t *testing.T) {
	c := &RestClient{Resolver: stubResolver{path: "/x"}}
	var out struct{}
	if _, err := c.Get(out, "id"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRestClient_List_ListMustBeSlicePtr(t *testing.T) {
	c := &RestClient{Resolver: stubResolver{path: "/x"}}
	var notSlice int
	if _, err := c.List(&notSlice); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRestClient_List_BuildsQueryParams(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("KUBECONFIG", writeKubeconfig(t, tmp, "tok1"))
	oldHost := settings.Settings.Inventory.Host
	oldPort := settings.Settings.Inventory.Port
	t.Cleanup(func() {
		settings.Settings.Inventory.Host = oldHost
		settings.Settings.Inventory.Port = oldPort
	})
	settings.Settings.Inventory.Host = "inv.local"
	settings.Settings.Inventory.Port = 8443

	transport := rtFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.RawQuery == "" || !strings.Contains(r.URL.RawQuery, "a=1") {
			t.Fatalf("expected query param, got url=%s", r.URL.String())
		}
		body := `[]`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     http.Header{},
		}, nil
	})

	c := &RestClient{
		LibClient: libweb.Client{
			Transport: transport,
		},
		Resolver: stubResolver{path: "/providers/:provider/things"},
		Params:   Params{ProviderParam: "p1"},
	}
	var list []map[string]any
	if _, err := c.List(&list, Param{Key: "a", Value: "1"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestRestClient_Get_SetsAuthorizationHeaderAndUnmarshals(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("KUBECONFIG", writeKubeconfig(t, tmp, "tok2"))
	oldHost := settings.Settings.Inventory.Host
	oldPort := settings.Settings.Inventory.Port
	t.Cleanup(func() {
		settings.Settings.Inventory.Host = oldHost
		settings.Settings.Inventory.Port = oldPort
	})
	settings.Settings.Inventory.Host = "inv.local"
	settings.Settings.Inventory.Port = 8443

	transport := rtFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok2" {
			t.Fatalf("expected bearer header, got %q", got)
		}
		body := `{"x":"y"}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     http.Header{},
		}, nil
	})

	c := &RestClient{
		LibClient: libweb.Client{
			Transport: transport,
		},
		Resolver: stubResolver{path: "/providers/:provider/thing"},
		Params:   Params{ProviderParam: "p1"},
	}

	var out struct {
		X string `json:"x"`
	}
	if _, err := c.Get(&out, "id1"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.X != "y" {
		t.Fatalf("expected unmarshal, got %#v", out)
	}
}

func TestRestClient_Get_Non200DoesNotUnmarshal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("KUBECONFIG", writeKubeconfig(t, tmp, "tok3"))
	oldHost := settings.Settings.Inventory.Host
	oldPort := settings.Settings.Inventory.Port
	t.Cleanup(func() {
		settings.Settings.Inventory.Host = oldHost
		settings.Settings.Inventory.Port = oldPort
	})
	settings.Settings.Inventory.Host = "inv.local"
	settings.Settings.Inventory.Port = 8443

	transport := rtFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString(`{"x":"should-not-unmarshal"}`)),
			Header:     http.Header{},
		}, nil
	})
	c := &RestClient{
		LibClient: libweb.Client{Transport: transport},
		Resolver:  stubResolver{path: "/providers/:provider/thing"},
		Params:    Params{ProviderParam: "p1"},
	}
	var out struct {
		X string `json:"x"`
	}
	status, err := c.Get(&out, "id1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", status)
	}
	if out.X != "" {
		t.Fatalf("expected no unmarshal, got %#v", out)
	}
}

func TestRestClient_Get_InvalidJSONReturnsErr(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("KUBECONFIG", writeKubeconfig(t, tmp, "tok4"))
	oldHost := settings.Settings.Inventory.Host
	oldPort := settings.Settings.Inventory.Port
	t.Cleanup(func() {
		settings.Settings.Inventory.Host = oldHost
		settings.Settings.Inventory.Port = oldPort
	})
	settings.Settings.Inventory.Host = "inv.local"
	settings.Settings.Inventory.Port = 8443

	transport := rtFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{bad-json`)),
			Header:     http.Header{},
		}, nil
	})
	c := &RestClient{
		LibClient: libweb.Client{Transport: transport},
		Resolver:  stubResolver{path: "/providers/:provider/thing"},
		Params:    Params{ProviderParam: "p1"},
	}
	var out map[string]any
	if _, err := c.Get(&out, "id1"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRestClient_Watch_ResolverNil(t *testing.T) {
	c := &RestClient{}
	var out struct{}
	if _, _, err := c.Watch(&out, &libweb.StockEventHandler{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRestClient_Watch_ResourceMustBePtr(t *testing.T) {
	c := &RestClient{Resolver: stubResolver{path: "/x"}}
	var out struct{}
	if _, _, err := c.Watch(out, &libweb.StockEventHandler{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRestClient_URL_AbsolutePreserved(t *testing.T) {
	c := &RestClient{}
	u := c.url("https://example.invalid/x")
	if u != "https://example.invalid/x" {
		t.Fatalf("expected absolute preserved, got %q", u)
	}
}

func TestRestClient_BuildTransport_DevelopmentSetsInsecureTLS(t *testing.T) {
	oldDev := settings.Settings.Development
	oldCA := settings.Settings.Inventory.TLS.CA
	t.Cleanup(func() {
		settings.Settings.Development = oldDev
		settings.Settings.Inventory.TLS.CA = oldCA
	})
	settings.Settings.Development = true
	settings.Settings.Inventory.TLS.CA = ""

	c := &RestClient{}
	if err := c.buildTransport(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if c.Transport == nil {
		t.Fatalf("expected transport")
	}
}

func TestRestClient_BuildTransport_CAFileMissingReturnsError(t *testing.T) {
	oldDev := settings.Settings.Development
	oldCA := settings.Settings.Inventory.TLS.CA
	t.Cleanup(func() {
		settings.Settings.Development = oldDev
		settings.Settings.Inventory.TLS.CA = oldCA
	})
	settings.Settings.Development = false
	settings.Settings.Inventory.TLS.CA = "/no/such/ca.pem"

	c := &RestClient{}
	if err := c.buildTransport(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRestClient_URL_FillsSchemeAndHostAndParams(t *testing.T) {
	oldHost := settings.Settings.Inventory.Host
	oldPort := settings.Settings.Inventory.Port
	t.Cleanup(func() {
		settings.Settings.Inventory.Host = oldHost
		settings.Settings.Inventory.Port = oldPort
	})
	settings.Settings.Inventory.Host = "inv.local"
	settings.Settings.Inventory.Port = 8443

	c := &RestClient{
		Params: Params{
			ProviderParam: "p1",
			NsParam:       "ns1",
		},
	}
	u := c.url("/providers/:provider/namespaces/:namespace/things")
	if !strings.HasPrefix(u, "https://inv.local:8443/") {
		t.Fatalf("unexpected url: %q", u)
	}
	if !strings.Contains(u, "/providers/p1/") || !strings.Contains(u, "/namespaces/ns1/") {
		t.Fatalf("expected params substituted, got %q", u)
	}
}
