package web

import (
	"errors"
	"net/http"
	"testing"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	"github.com/kubev2v/forklift/pkg/controller/provider/web/base"
)

type stubFinder struct {
	withCalled bool
	lastClient Client

	byRefCalled    bool
	vmCalled       bool
	workloadCalled bool
	networkCalled  bool
	storageCalled  bool
	hostCalled     bool

	byRefErr    error
	vmObj       interface{}
	vmErr       error
	workloadObj interface{}
	workloadErr error
	networkObj  interface{}
	networkErr  error
	storageObj  interface{}
	storageErr  error
	hostObj     interface{}
	hostErr     error
}

func (s *stubFinder) With(c Client) Finder {
	s.withCalled = true
	s.lastClient = c
	return s
}

func (s *stubFinder) ByRef(resource interface{}, ref base.Ref) error {
	s.byRefCalled = true
	return s.byRefErr
}

func (s *stubFinder) VM(ref *base.Ref) (interface{}, error) {
	s.vmCalled = true
	return s.vmObj, s.vmErr
}

func (s *stubFinder) Workload(ref *base.Ref) (interface{}, error) {
	s.workloadCalled = true
	return s.workloadObj, s.workloadErr
}

func (s *stubFinder) Network(ref *base.Ref) (interface{}, error) {
	s.networkCalled = true
	return s.networkObj, s.networkErr
}

func (s *stubFinder) Storage(ref *base.Ref) (interface{}, error) {
	s.storageCalled = true
	return s.storageObj, s.storageErr
}

func (s *stubFinder) Host(ref *base.Ref) (interface{}, error) {
	s.hostCalled = true
	return s.hostObj, s.hostErr
}

func readyProvider(pt api.ProviderType) *api.Provider {
	p := &api.Provider{}
	p.Spec.Type = &pt
	p.Status.ObservedGeneration = 1
	p.Generation = 1
	return p
}

func TestProviderClient_Finder_CallsWithAndReturnsFinder(t *testing.T) {
	pt := api.VSphere
	f := &stubFinder{}
	pc := &ProviderClient{provider: readyProvider(pt), finder: f}
	got := pc.Finder()
	if got == nil {
		t.Fatalf("expected finder")
	}
	if !f.withCalled || f.lastClient != pc {
		t.Fatalf("expected With called with provider client")
	}
}

func TestProviderClient_Find_DelegatesToFinderByRef(t *testing.T) {
	pt := api.VSphere
	f := &stubFinder{}
	pc := &ProviderClient{provider: readyProvider(pt), finder: f}
	ref := base.Ref{ID: "id"}
	var out struct{}
	if err := pc.Find(&out, ref); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !f.byRefCalled {
		t.Fatalf("expected ByRef called")
	}
}

func TestProviderClient_Find_PropagatesFinderError(t *testing.T) {
	pt := api.VSphere
	f := &stubFinder{byRefErr: errors.New("boom")}
	pc := &ProviderClient{provider: readyProvider(pt), finder: f}
	var out struct{}
	if err := pc.Find(&out, base.Ref{ID: "id"}); err == nil {
		t.Fatalf("expected err")
	}
}

func TestProviderClient_VM_DelegatesToFinderVM(t *testing.T) {
	pt := api.VSphere
	f := &stubFinder{vmObj: "vm"}
	pc := &ProviderClient{provider: readyProvider(pt), finder: f}
	obj, err := pc.VM(&base.Ref{ID: "x"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if obj != "vm" || !f.vmCalled {
		t.Fatalf("unexpected: obj=%v called=%v", obj, f.vmCalled)
	}
}

func TestProviderClient_VM_PropagatesFinderError(t *testing.T) {
	pt := api.VSphere
	f := &stubFinder{vmErr: errors.New("boom")}
	pc := &ProviderClient{provider: readyProvider(pt), finder: f}
	if _, err := pc.VM(&base.Ref{ID: "x"}); err == nil {
		t.Fatalf("expected err")
	}
}

func TestProviderClient_Workload_DelegatesToFinderWorkload(t *testing.T) {
	pt := api.VSphere
	f := &stubFinder{workloadObj: 123}
	pc := &ProviderClient{provider: readyProvider(pt), finder: f}
	obj, err := pc.Workload(&base.Ref{ID: "x"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if obj != 123 || !f.workloadCalled {
		t.Fatalf("unexpected: obj=%v called=%v", obj, f.workloadCalled)
	}
}

func TestProviderClient_Workload_PropagatesFinderError(t *testing.T) {
	pt := api.VSphere
	f := &stubFinder{workloadErr: errors.New("boom")}
	pc := &ProviderClient{provider: readyProvider(pt), finder: f}
	if _, err := pc.Workload(&base.Ref{ID: "x"}); err == nil {
		t.Fatalf("expected err")
	}
}

func TestProviderClient_Network_DelegatesToFinderNetwork(t *testing.T) {
	pt := api.VSphere
	f := &stubFinder{networkObj: true}
	pc := &ProviderClient{provider: readyProvider(pt), finder: f}
	obj, err := pc.Network(&base.Ref{ID: "x"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if obj != true || !f.networkCalled {
		t.Fatalf("unexpected: obj=%v called=%v", obj, f.networkCalled)
	}
}

func TestProviderClient_Network_PropagatesFinderError(t *testing.T) {
	pt := api.VSphere
	f := &stubFinder{networkErr: errors.New("boom")}
	pc := &ProviderClient{provider: readyProvider(pt), finder: f}
	if _, err := pc.Network(&base.Ref{ID: "x"}); err == nil {
		t.Fatalf("expected err")
	}
}

func TestProviderClient_Storage_DelegatesToFinderStorage(t *testing.T) {
	pt := api.VSphere
	f := &stubFinder{storageObj: "ds"}
	pc := &ProviderClient{provider: readyProvider(pt), finder: f}
	obj, err := pc.Storage(&base.Ref{ID: "x"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if obj != "ds" || !f.storageCalled {
		t.Fatalf("unexpected: obj=%v called=%v", obj, f.storageCalled)
	}
}

func TestProviderClient_Storage_PropagatesFinderError(t *testing.T) {
	pt := api.VSphere
	f := &stubFinder{storageErr: errors.New("boom")}
	pc := &ProviderClient{provider: readyProvider(pt), finder: f}
	if _, err := pc.Storage(&base.Ref{ID: "x"}); err == nil {
		t.Fatalf("expected err")
	}
}

func TestProviderClient_Host_DelegatesToFinderHost(t *testing.T) {
	pt := api.VSphere
	f := &stubFinder{hostObj: "h"}
	pc := &ProviderClient{provider: readyProvider(pt), finder: f}
	obj, err := pc.Host(&base.Ref{ID: "x"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if obj != "h" || !f.hostCalled {
		t.Fatalf("unexpected: obj=%v called=%v", obj, f.hostCalled)
	}
}

func TestProviderClient_Host_PropagatesFinderError(t *testing.T) {
	pt := api.VSphere
	f := &stubFinder{hostErr: errors.New("boom")}
	pc := &ProviderClient{provider: readyProvider(pt), finder: f}
	if _, err := pc.Host(&base.Ref{ID: "x"}); err == nil {
		t.Fatalf("expected err")
	}
}

func TestProviderClient_HasReason_FalseWhenNoHeader(t *testing.T) {
	pt := api.VSphere
	pc := &ProviderClient{provider: readyProvider(pt)}
	if pc.HasReason(base.UnknownProvider) {
		t.Fatalf("expected false")
	}
}

func TestProviderClient_HasReason_TrueWhenAnyHeaderMatches(t *testing.T) {
	pt := api.VSphere
	pc := &ProviderClient{provider: readyProvider(pt)}
	pc.restClient.Reply.Header = http.Header{
		base.ReasonHeader: []string{"a", "b", base.UnknownProvider, "c"},
	}
	if !pc.HasReason(base.UnknownProvider) {
		t.Fatalf("expected true")
	}
}

func TestProviderClient_HasReason_TrueCaseInsensitive(t *testing.T) {
	pt := api.VSphere
	pc := &ProviderClient{provider: readyProvider(pt)}
	pc.restClient.Reply.Header = http.Header{
		base.ReasonHeader: []string{"pRoViDeRnOtFoUnD"},
	}
	if !pc.HasReason(base.UnknownProvider) {
		t.Fatalf("expected true")
	}
}

func TestProviderClient_HasReason_FalseWhenHeaderPresentButEmpty(t *testing.T) {
	pt := api.VSphere
	pc := &ProviderClient{provider: readyProvider(pt)}
	pc.restClient.Reply.Header = http.Header{
		base.ReasonHeader: []string{""},
	}
	if pc.HasReason(base.UnknownProvider) {
		t.Fatalf("expected false")
	}
}

func TestProviderClient_HasReason_FalseWhenDifferentReason(t *testing.T) {
	pt := api.VSphere
	pc := &ProviderClient{provider: readyProvider(pt)}
	pc.restClient.Reply.Header = http.Header{
		base.ReasonHeader: []string{"something-else"},
	}
	if pc.HasReason(base.UnknownProvider) {
		t.Fatalf("expected false")
	}
}

func TestProviderClient_asError_OK_Nil(t *testing.T) {
	pt := api.VSphere
	pc := &ProviderClient{provider: readyProvider(pt)}
	if err := pc.asError(http.StatusOK, ""); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestProviderClient_asError_OK_StillNilWhenIDProvided(t *testing.T) {
	pt := api.VSphere
	pc := &ProviderClient{provider: readyProvider(pt)}
	if err := pc.asError(http.StatusOK, "abc"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestProviderClient_asError_PartialContent_ProviderNotReady(t *testing.T) {
	pt := api.VSphere
	pc := &ProviderClient{provider: readyProvider(pt)}
	err := pc.asError(http.StatusPartialContent, "")
	if err == nil {
		t.Fatalf("expected err")
	}
	var pnr ProviderNotReadyError
	if !errors.As(err, &pnr) {
		t.Fatalf("expected ProviderNotReadyError, got: %v", err)
	}
}

func TestProviderClient_asError_NotFound_WithUnknownProviderReason_ProviderNotReady(t *testing.T) {
	pt := api.VSphere
	pc := &ProviderClient{provider: readyProvider(pt)}
	pc.restClient.Reply.Header = http.Header{
		base.ReasonHeader: []string{base.UnknownProvider},
	}
	err := pc.asError(http.StatusNotFound, "")
	if err == nil {
		t.Fatalf("expected err")
	}
	if err.Error() == "" {
		t.Fatalf("expected non-empty")
	}
}

func TestProviderClient_asError_NotFound_WithNonMatchingReason_NotFound(t *testing.T) {
	pt := api.VSphere
	pc := &ProviderClient{provider: readyProvider(pt)}
	pc.restClient.Reply.Header = http.Header{
		base.ReasonHeader: []string{"something-else"},
	}
	err := pc.asError(http.StatusNotFound, "abc")
	if err == nil {
		t.Fatalf("expected err")
	}
	var nf base.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected NotFoundError, got: %v", err)
	}
}

func TestProviderClient_asError_NotFound_WithoutReason_NotFoundErrorWithID(t *testing.T) {
	pt := api.VSphere
	pc := &ProviderClient{provider: readyProvider(pt)}
	err := pc.asError(http.StatusNotFound, "abc")
	if err == nil {
		t.Fatalf("expected err")
	}
	var nf base.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected NotFoundError, got: %v", err)
	}
}

func TestProviderClient_asError_NotFound_WithoutReason_EmptyID(t *testing.T) {
	pt := api.VSphere
	pc := &ProviderClient{provider: readyProvider(pt)}
	err := pc.asError(http.StatusNotFound, "")
	if err == nil {
		t.Fatalf("expected err")
	}
	var nf base.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected NotFoundError")
	}
}

func TestProviderClient_asError_Default_StatusTextError(t *testing.T) {
	pt := api.VSphere
	pc := &ProviderClient{provider: readyProvider(pt)}
	err := pc.asError(http.StatusTeapot, "")
	if err == nil {
		t.Fatalf("expected err")
	}
}

func TestProviderNotSupportedError_Error_NonEmpty(t *testing.T) {
	err := ProviderNotSupportedError{Provider: &api.Provider{}}
	if err.Error() == "" {
		t.Fatalf("expected non-empty")
	}
}

func TestProviderNotReadyError_Error_NonEmpty(t *testing.T) {
	err := ProviderNotReadyError{Provider: &api.Provider{}}
	if err.Error() == "" {
		t.Fatalf("expected non-empty")
	}
}
