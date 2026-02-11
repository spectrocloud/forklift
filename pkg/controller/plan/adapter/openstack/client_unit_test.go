package openstack

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"unsafe"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	planapi "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/plan"
	"github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1/ref"
	plancontext "github.com/kubev2v/forklift/pkg/controller/plan/context"
	model "github.com/kubev2v/forklift/pkg/controller/provider/web/openstack"
	libclient "github.com/kubev2v/forklift/pkg/lib/client/openstack"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func setUnexportedField(ptrToStruct any, field string, value any) {
	v := reflect.ValueOf(ptrToStruct).Elem()
	f := v.FieldByName(field)
	if !f.IsValid() {
		panic("field not found: " + field)
	}
	reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

func newLibClientWithServices(t *testing.T, srv *httptest.Server) libclient.Client {
	t.Helper()

	pc := &gophercloud.ProviderClient{
		TokenID:    "tok",
		HTTPClient: *srv.Client(),
	}
	base := srv.URL + "/"
	idSvc := &gophercloud.ServiceClient{ProviderClient: pc, Endpoint: base, ResourceBase: base}
	compSvc := &gophercloud.ServiceClient{ProviderClient: pc, Endpoint: base, ResourceBase: base}
	imgSvc := &gophercloud.ServiceClient{ProviderClient: pc, Endpoint: base, ResourceBase: base}
	blkSvc := &gophercloud.ServiceClient{ProviderClient: pc, Endpoint: base, ResourceBase: base}
	netSvc := &gophercloud.ServiceClient{ProviderClient: pc, Endpoint: base, ResourceBase: base}

	c := libclient.Client{
		URL:     base,
		Options: map[string]string{},
		Log:     logging.WithName("openstack-adapter-client-test"),
	}
	setUnexportedField(&c, "provider", pc)
	setUnexportedField(&c, "identityService", idSvc)
	setUnexportedField(&c, "computeService", compSvc)
	setUnexportedField(&c, "imageService", imgSvc)
	setUnexportedField(&c, "blockStorageService", blkSvc)
	setUnexportedField(&c, "networkService", netSvc)
	return c
}

func TestAdapterClient_PowerAndGetByNameAndDeleteImage(t *testing.T) {
	var (
		vmStatus    atomic.Value
		deleteCount atomic.Int64
	)
	vmStatus.Store("ACTIVE")

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Compute: start/stop action.
		case strings.HasSuffix(r.URL.Path, "/action") && strings.Contains(r.URL.Path, "/servers/") && r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			switch {
			case bytes.Contains(body, []byte("os-stop")):
				vmStatus.Store("SHUTOFF")
			case bytes.Contains(body, []byte("os-start")):
				vmStatus.Store("ACTIVE")
			}
			w.WriteHeader(http.StatusAccepted)
			return

		// Compute: server list (used for getVM by Name)
		case (strings.HasSuffix(r.URL.Path, "/servers") || strings.Contains(r.URL.Path, "/servers/detail")) && r.Method == http.MethodGet:
			if strings.Contains(r.URL.RawQuery, "missing") {
				_ = json.NewEncoder(w).Encode(map[string]any{"servers": []any{}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"servers": []any{map[string]any{"id": "vm1", "name": "vm-by-name", "status": vmStatus.Load().(string)}},
			})
			return

		// Compute: server by ID
		case strings.Contains(r.URL.Path, "/servers/") && !strings.Contains(r.URL.Path, "/action") && r.Method == http.MethodGet:
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/servers/")+len("/servers/"):]
			_ = json.NewEncoder(w).Encode(map[string]any{
				"server": map[string]any{
					"id":     id,
					"name":   "vm",
					"status": vmStatus.Load().(string),
				},
			})
			return

		// Image list (used by getImage by Name and getImagesFromVolumes)
		case strings.HasSuffix(r.URL.Path, "/images") && r.Method == http.MethodGet:
			name := r.URL.Query().Get("name")
			if strings.Contains(name, "vol-missing") || name == "missing" {
				_ = json.NewEncoder(w).Encode(map[string]any{"images": []any{}})
				return
			}
			status := "active"
			id := "img1"
			if strings.Contains(name, "vol2") {
				status = "queued"
				id = "img2"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"images": []any{map[string]any{"id": id, "name": name, "status": status, "properties": map[string]any{}}},
			})
			return

		// Image delete
		case strings.Contains(r.URL.Path, "/images/") && r.Method == http.MethodDelete:
			deleteCount.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return

		// Block storage: volume by ID
		case strings.Contains(r.URL.Path, "/volumes/") && !strings.Contains(r.URL.Path, "/detail") && r.Method == http.MethodGet:
			id := strings.TrimPrefix(r.URL.Path, "/volumes/")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"volume": map[string]any{
					"id":     id,
					"name":   "vol",
					"status": "available",
					"metadata": map[string]any{
						forkliftPropertyOriginalVolumeID: id,
					},
				},
			})
			return

		// Block storage: volume list (used for getVolume by Name)
		case strings.Contains(r.URL.Path, "/volumes/detail") && r.Method == http.MethodGet:
			name := r.URL.Query().Get("name")
			if name == "missing" {
				_ = json.NewEncoder(w).Encode(map[string]any{"volumes": []any{}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"volumes": []any{map[string]any{"id": "vol1", "name": name, "status": "available", "metadata": map[string]any{forkliftPropertyOriginalVolumeID: "vol1"}}},
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ctx := &plancontext.Context{
		Plan: &api.Plan{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"}},
	}

	adapter := &Client{
		Client:  newLibClientWithServices(t, srv),
		Context: ctx,
	}

	// PowerState -> On (ACTIVE)
	state, err := adapter.PowerState(ref.Ref{ID: "vm1"})
	if err != nil {
		t.Fatalf("PowerState: %v", err)
	}
	if state != planapi.VMPowerStateOn {
		t.Fatalf("expected ON, got %v", state)
	}

	// PowerOff should issue stop action if not already off.
	if err := adapter.PowerOff(ref.Ref{ID: "vm1"}); err != nil {
		t.Fatalf("PowerOff: %v", err)
	}
	off, err := adapter.PoweredOff(ref.Ref{ID: "vm1"})
	if err != nil || !off {
		t.Fatalf("PoweredOff: off=%v err=%v", off, err)
	}

	// PowerOn should issue start action.
	if err := adapter.PowerOn(ref.Ref{ID: "vm1"}); err != nil {
		t.Fatalf("PowerOn: %v", err)
	}

	// getVM by Name (List path)
	vm, err := adapter.getVM(ref.Ref{Name: "vm-by-name"})
	if err != nil || vm == nil || vm.ID == "" {
		t.Fatalf("getVM by name: vm=%#v err=%v", vm, err)
	}
	// getVM by Name not found
	if _, err := adapter.getVM(ref.Ref{Name: "missing"}); err == nil {
		t.Fatalf("expected not found")
	}

	// getImage/getVolume by Name
	if _, err := adapter.getImage(ref.Ref{Name: "img-by-name"}); err != nil {
		t.Fatalf("getImage by name: %v", err)
	}
	if _, err := adapter.getVolume(ref.Ref{Name: "vol-by-name"}); err != nil {
		t.Fatalf("getVolume by name: %v", err)
	}
	if _, err := adapter.getImage(ref.Ref{Name: "missing"}); err == nil {
		t.Fatalf("expected missing image")
	}
	if _, err := adapter.getVolume(ref.Ref{Name: "missing"}); err == nil {
		t.Fatalf("expected missing volume")
	}

	// removeImagesFromVolumes: should delete only ACTIVE images
	vm2 := &libclient.VM{Server: servers.Server{ID: "vm1", Name: "vm"}}
	vm2.AttachedVolumes = []servers.AttachedVolume{{ID: "vol1"}, {ID: "vol2"}, {ID: "vol-missing"}}
	if err := adapter.removeImagesFromVolumes(vm2); err != nil {
		t.Fatalf("removeImagesFromVolumes: %v", err)
	}
	if deleteCount.Load() != 1 {
		t.Fatalf("expected 1 delete (only active image), got %d", deleteCount.Load())
	}
}

func TestAdapterClient_EnsureVmSnapshotAndImagesFromVolumesReady(t *testing.T) {
	ctx := &plancontext.Context{
		Plan: createPlan(),
	}
	snapshotName := getVmSnapshotName(ctx, "vm1")
	volumeImageName := getImageFromVolumeName(ctx, "vm1", "vol1")

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		// Image list (used by getVmSnapshotImage + getImagesFromVolumes).
		case strings.HasSuffix(r.URL.Path, "/images") && r.Method == http.MethodGet:
			name := r.URL.Query().Get("name")
			// Return a snapshot image and a volume image, both active.
			id := "img-unknown"
			extra := map[string]any{}
			switch name {
			case snapshotName:
				id = "img-snap"
			case volumeImageName:
				id = "img-vol1"
				// This must be a top-level key; gophercloud folds remaining keys into Image.Properties.
				extra[forkliftPropertyOriginalVolumeID] = "vol1"
			}
			payload := map[string]any{
				"id":     id,
				"name":   name,
				"status": "active",
			}
			for k, v := range extra {
				payload[k] = v
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"images": []any{payload}})
			return

		// Snapshot list (used by getSnapshotFromVolume -> ensure updateImageProperty path, and by cleanup goroutine).
		case strings.Contains(r.URL.Path, "/snapshots") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"snapshots": []any{}})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	src := &stubInv2{}
	src.findFn = func(resource interface{}, rf ref.Ref) error {
		switch r := resource.(type) {
		case *model.Image:
			switch rf.ID {
			case "img-snap":
				*r = model.Image{
					Resource:   model.Resource{ID: "img-snap", Name: "snap"},
					Status:     string(ImageStatusActive),
					Properties: map[string]interface{}{forkliftPropertyOriginalImageID: "origImg"},
				}
				return nil
			case "img-vol1":
				*r = model.Image{
					Resource:   model.Resource{ID: "img-vol1", Name: "vol1"},
					Status:     string(ImageStatusActive),
					Properties: map[string]interface{}{forkliftPropertyOriginalVolumeID: "vol1"},
				}
				return nil
			default:
				return model.NotFoundError{}
			}
		default:
			return nil
		}
	}
	ctx.Source.Inventory = src

	adapter := &Client{
		Client:  newLibClientWithServices(t, srv),
		Context: ctx,
	}

	vm := &libclient.VM{Server: servers.Server{ID: "vm1", Name: "vm"}}
	vm.AttachedVolumes = []servers.AttachedVolume{{ID: "vol1"}}
	vm.Image = map[string]interface{}{"id": "origImg"}

	ready, err := adapter.ensureVmSnapshot(vm)
	if err != nil {
		t.Fatalf("ensureVmSnapshot: %v", err)
	}
	if !ready {
		t.Fatalf("expected ensureVmSnapshot ready=true")
	}

	ready, err = adapter.ensureImagesFromVolumesReady(vm)
	if err != nil {
		t.Fatalf("ensureImagesFromVolumesReady: %v", err)
	}
	if !ready {
		t.Fatalf("expected ensureImagesFromVolumesReady ready=true")
	}
}

func TestAdapterClient_GetVolumeFromSnapshot_FoundAndNotFound(t *testing.T) {
	ctx := &plancontext.Context{Plan: createPlan()}
	vm := &libclient.VM{Server: servers.Server{ID: "vm1", Name: "vm"}}
	snapshotID := "snap1"
	volumeName := getVolumeFromSnapshotName(ctx, vm.ID, snapshotID)

	makeServer := func(returnVolume bool) *httptest.Server {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.Contains(r.URL.Path, "/snapshots/") && r.Method == http.MethodGet:
				id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
				_ = json.NewEncoder(w).Encode(map[string]any{
					"snapshot": map[string]any{
						"id":        id,
						"volume_id": "vol1",
						"status":    "available",
					},
				})
				return
			case strings.Contains(r.URL.Path, "/volumes/detail") && r.Method == http.MethodGet:
				if !returnVolume {
					_ = json.NewEncoder(w).Encode(map[string]any{"volumes": []any{}})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"volumes": []any{map[string]any{
						"id":          "vfs1",
						"name":        volumeName,
						"status":      "available",
						"snapshot_id": snapshotID,
						"metadata": map[string]any{
							forkliftPropertyOriginalVolumeID: "vol1",
						},
					}},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		})
		return httptest.NewServer(mux)
	}

	t.Run("found", func(t *testing.T) {
		srv := makeServer(true)
		t.Cleanup(srv.Close)
		adapter := &Client{Client: newLibClientWithServices(t, srv), Context: ctx}
		vol, err := adapter.getVolumeFromSnapshot(vm, snapshotID)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if vol == nil || vol.Name != volumeName {
			t.Fatalf("unexpected volume: %#v", vol)
		}
	})

	t.Run("not found => ResourceNotFoundError", func(t *testing.T) {
		srv := makeServer(false)
		t.Cleanup(srv.Close)
		adapter := &Client{Client: newLibClientWithServices(t, srv), Context: ctx}
		_, err := adapter.getVolumeFromSnapshot(vm, snapshotID)
		if err == nil || !errors.Is(err, ResourceNotFoundError) {
			t.Fatalf("expected ResourceNotFoundError, got %v", err)
		}
	})
}
