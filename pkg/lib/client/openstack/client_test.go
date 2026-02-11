package openstack

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/tokens"
	liberr "github.com/kubev2v/forklift/pkg/lib/error"
	"github.com/kubev2v/forklift/pkg/lib/logging"
	corev1 "k8s.io/api/core/v1"
)

func TestClient_LoadOptionsFromSecret(t *testing.T) {
	c := &Client{}
	sec := &corev1.Secret{
		Data: map[string][]byte{
			"username": []byte("u"),
			"password": []byte("p"),
		},
	}
	c.LoadOptionsFromSecret(sec)
	if c.Options["username"] != "u" || c.Options["password"] != "p" {
		t.Fatalf("unexpected options: %#v", c.Options)
	}
}

func TestClient_authType(t *testing.T) {
	c := &Client{Options: map[string]string{}}

	// default
	at, err := c.authType()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if at != supportedAuthTypes["password"] {
		t.Fatalf("expected password auth, got: %v", at)
	}

	// supported
	c.Options[AuthType] = "token"
	at, err = c.authType()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if at != supportedAuthTypes["token"] {
		t.Fatalf("expected token auth, got: %v", at)
	}

	// unsupported
	c.Options[AuthType] = "nope"
	_, err = c.authType()
	if err == nil {
		t.Fatalf("expected error")
	}

	// application credential
	c.Options[AuthType] = "applicationcredential"
	at, err = c.authType()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if at != supportedAuthTypes["applicationcredential"] {
		t.Fatalf("expected applicationcredential auth, got: %v", at)
	}
}

func TestClient_getTLSConfig(t *testing.T) {
	t.Run("invalid URL", func(t *testing.T) {
		c := &Client{URL: "://bad-url", Options: map[string]string{}, Log: logging.WithName("openstack-client-test")}
		_, err := c.getTLSConfig()
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("http scheme returns nil config", func(t *testing.T) {
		c := &Client{URL: "http://example.invalid", Options: map[string]string{}, Log: logging.WithName("openstack-client-test")}
		cfg, err := c.getTLSConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg != nil {
			t.Fatalf("expected nil tls config for http")
		}
	})

	t.Run("https insecure skip verify", func(t *testing.T) {
		c := &Client{
			URL: "https://example.invalid",
			Log: logging.WithName("openstack-client-test"),
			Options: map[string]string{
				InsecureSkipVerify: "true",
			},
		}
		cfg, err := c.getTLSConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg == nil || !cfg.InsecureSkipVerify {
			t.Fatalf("expected InsecureSkipVerify tls config, got: %#v", cfg)
		}
	})

	t.Run("https malformed cacert", func(t *testing.T) {
		c := &Client{
			URL: "https://example.invalid",
			Log: logging.WithName("openstack-client-test"),
			Options: map[string]string{
				CACert: "not-a-pem",
			},
		}
		_, err := c.getTLSConfig()
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestClient_getEndpointOpts(t *testing.T) {
	c := &Client{
		Options: map[string]string{
			RegionName:           "r1",
			EndpointAvailability: string(gophercloud.AvailabilityInternal),
		},
	}
	e := c.getEndpointOpts()
	if e.Region != "r1" || e.Availability != gophercloud.AvailabilityInternal {
		t.Fatalf("unexpected endpoint opts: %#v", e)
	}
}

func TestClient_getBoolFromOptions(t *testing.T) {
	c := &Client{Options: map[string]string{"x": "not-bool"}}
	if c.getBoolFromOptions("x") {
		t.Fatalf("expected false for invalid bool")
	}
	if c.getBoolFromOptions("missing") {
		t.Fatalf("expected false for missing key")
	}
}

func TestClient_IsNotFound_IsForbidden(t *testing.T) {
	c := &Client{}

	err404 := liberr.Wrap(gophercloud.ErrUnexpectedResponseCode{Actual: 404})
	if !c.IsNotFound(err404) {
		t.Fatalf("expected IsNotFound true")
	}
	if c.IsForbidden(err404) {
		t.Fatalf("expected IsForbidden false")
	}

	err403 := liberr.Wrap(gophercloud.ErrUnexpectedResponseCode{Actual: 403})
	if c.IsNotFound(err403) {
		t.Fatalf("expected IsNotFound false")
	}
	if !c.IsForbidden(err403) {
		t.Fatalf("expected IsForbidden true")
	}

	other := liberr.Wrap(errors.New("other"))
	if c.IsNotFound(other) || c.IsForbidden(other) {
		t.Fatalf("expected false for non gophercloud error")
	}
}

func TestClient_CRUD_DispatchUnsupportedTypes(t *testing.T) {
	c := &Client{Log: logging.WithName("openstack-client-test")}

	// List: unsupported type should wrap an unsupportedTypeError.
	if err := c.List(&struct{}{}, nil); err == nil {
		t.Fatalf("expected error")
	} else if !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("expected unsupported type error, got: %v", err)
	}

	// Get: unsupported type should wrap an unsupportedTypeError.
	if err := c.Get(&struct{}{}, "x"); err == nil {
		t.Fatalf("expected error")
	} else if !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("expected unsupported type error, got: %v", err)
	}

	// Create: unsupported type should wrap an unsupportedTypeError.
	if err := c.Create(&struct{}{}, nil); err == nil {
		t.Fatalf("expected error")
	} else if !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("expected unsupported type error, got: %v", err)
	}

	// Update: unsupported type should wrap an unsupportedTypeError.
	if err := c.Update(&struct{}{}, nil); err == nil {
		t.Fatalf("expected error")
	} else if !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("expected unsupported type error, got: %v", err)
	}

	// Delete: unsupported type should wrap an unsupportedTypeError.
	if err := c.Delete(&struct{}{}); err == nil {
		t.Fatalf("expected error")
	} else if !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("expected unsupported type error, got: %v", err)
	}
}

func TestClient_IsNotFound_IsForbidden_UnwrapsDirectErrUnexpectedResponseCode(t *testing.T) {
	c := &Client{}
	err404 := gophercloud.ErrUnexpectedResponseCode{Actual: http.StatusNotFound}
	if !c.IsNotFound(err404) {
		t.Fatalf("expected IsNotFound true")
	}
	if c.IsForbidden(err404) {
		t.Fatalf("expected IsForbidden false")
	}
}

func TestClient_Authenticate_EarlyReturnWhenProviderAlreadySet(t *testing.T) {
	c := &Client{
		URL:     "https://identity.example.invalid",
		Options: map[string]string{},
		Log:     logging.WithName("openstack-client-test"),
		provider: &gophercloud.ProviderClient{
			// noop: Authenticate() should not touch this because provider != nil.
			EndpointLocator: func(eo gophercloud.EndpointOpts) (string, error) { return "", nil },
		},
	}
	if err := c.Authenticate(); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestClient_connectServiceAPIs_EndpointLocatorError(t *testing.T) {
	newClient := func() *Client {
		return &Client{
			URL:     "https://identity.example.invalid",
			Options: map[string]string{},
			Log:     logging.WithName("openstack-client-test"),
			provider: &gophercloud.ProviderClient{
				EndpointLocator: func(eo gophercloud.EndpointOpts) (string, error) {
					return "", errors.New("no endpoint")
				},
			},
		}
	}

	t.Run("connectIdentityServiceAPI", func(t *testing.T) {
		c := newClient()
		if err := c.connectIdentityServiceAPI(); err == nil {
			t.Fatalf("expected error")
		}
	})
	t.Run("connectComputeServiceAPI", func(t *testing.T) {
		c := newClient()
		if err := c.connectComputeServiceAPI(); err == nil {
			t.Fatalf("expected error")
		}
	})
	t.Run("connectImageServiceAPI", func(t *testing.T) {
		c := newClient()
		if err := c.connectImageServiceAPI(); err == nil {
			t.Fatalf("expected error")
		}
	})
	t.Run("connectBlockStorageServiceAPI", func(t *testing.T) {
		c := newClient()
		if err := c.connectBlockStorageServiceAPI(); err == nil {
			t.Fatalf("expected error")
		}
	})
	t.Run("connectNetworkServiceAPI", func(t *testing.T) {
		c := newClient()
		if err := c.connectNetworkServiceAPI(); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestClient_ServiceAPIs_UnsupportedType_Wrapped(t *testing.T) {
	// These tests avoid any real OpenStack calls by:
	// - setting provider != nil so Authenticate() short-circuits
	// - setting service clients != nil so connect*() doesn't try to create them
	// - passing unsupported object types so we never call gophercloud endpoints
	c := &Client{
		URL:     "https://identity.example.invalid",
		Options: map[string]string{},
		Log:     logging.WithName("openstack-client-test"),
		provider: &gophercloud.ProviderClient{
			EndpointLocator: func(eo gophercloud.EndpointOpts) (string, error) { return "", nil },
		},
		identityService:     &gophercloud.ServiceClient{},
		computeService:      &gophercloud.ServiceClient{},
		imageService:        &gophercloud.ServiceClient{},
		networkService:      &gophercloud.ServiceClient{},
		blockStorageService: &gophercloud.ServiceClient{},
	}

	assertUnsupported := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("expected error")
		}
		if !strings.Contains(err.Error(), "unsupported type") {
			t.Fatalf("expected unsupported type error, got: %v", err)
		}
	}

	t.Run("identityServiceAPI", func(t *testing.T) {
		assertUnsupported(t, c.identityServiceAPI(&struct{}{}, nil))
	})
	t.Run("computeServiceAPI", func(t *testing.T) {
		assertUnsupported(t, c.computeServiceAPI(&struct{}{}, nil))
	})
	t.Run("imageServiceAPI", func(t *testing.T) {
		assertUnsupported(t, c.imageServiceAPI(&struct{}{}, nil))
	})
	t.Run("blockStorageServiceAPI", func(t *testing.T) {
		assertUnsupported(t, c.blockStorageServiceAPI(&struct{}{}, nil))
	})
	t.Run("networkServiceAPI", func(t *testing.T) {
		assertUnsupported(t, c.networkServiceAPI(&struct{}{}, nil))
	})
}

type fakeAuthResult struct{}

func (fakeAuthResult) ExtractTokenID() (string, error) { return "tok", nil }

func TestClient_getAuthenticatedUserID_NoAuthResult(t *testing.T) {
	pc := &gophercloud.ProviderClient{}
	pc.SetToken("tok") // SetToken clears the auth result.

	c := &Client{provider: pc, Log: logging.WithName("openstack-client-test")}
	_, err := c.getAuthenticatedUserID()
	if err == nil || !strings.Contains(err.Error(), "no AuthResult available") {
		t.Fatalf("expected no AuthResult error, got: %v", err)
	}
}

func TestClient_getAuthenticatedUserID_UnsupportedAuthResultType(t *testing.T) {
	pc := &gophercloud.ProviderClient{}
	_ = pc.SetTokenAndAuthResult(fakeAuthResult{})

	c := &Client{provider: pc, Log: logging.WithName("openstack-client-test")}
	_, err := c.getAuthenticatedUserID()
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("expected unsupported type error, got: %v", err)
	}
}

func TestClient_getProjectIDFromApplicationCredentials_ByIDAndByName(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/users/u1/application_credentials/ac1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"application_credential":{"project_id":"pid-1"}}`))
	})
	mux.HandleFunc("/v3/users/u1/application_credentials", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// List endpoint used by applicationcredentials.List(...).AllPages()
		_, _ = w.Write([]byte(`{"application_credentials":[{"project_id":"pid-2"}],"links":{"next":""}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	pc := &gophercloud.ProviderClient{HTTPClient: *srv.Client()}
	var ar tokens.CreateResult
	ar.Body = map[string]interface{}{
		"token": map[string]interface{}{
			"user": map[string]interface{}{"id": "u1"},
		},
	}
	ar.Header = http.Header{"X-Subject-Token": []string{"tok"}}
	if err := pc.SetTokenAndAuthResult(ar); err != nil {
		t.Fatalf("SetTokenAndAuthResult: %v", err)
	}

	base := srv.URL + "/v3/"
	idSvc := &gophercloud.ServiceClient{
		ProviderClient: pc,
		Endpoint:       base,
		ResourceBase:   base,
	}

	t.Run("by ID", func(t *testing.T) {
		c := &Client{
			Options: map[string]string{
				ApplicationCredentialID: "ac1",
			},
			Log:             logging.WithName("openstack-client-test"),
			provider:        pc,
			identityService: idSvc,
		}
		pid, err := c.getProjectIDFromApplicationCredentials()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pid != "pid-1" {
			t.Fatalf("expected pid-1, got %q", pid)
		}
	})

	t.Run("by name", func(t *testing.T) {
		c := &Client{
			Options: map[string]string{
				ApplicationCredentialName: "anything",
			},
			Log:             logging.WithName("openstack-client-test"),
			provider:        pc,
			identityService: idSvc,
		}
		pid, err := c.getProjectIDFromApplicationCredentials()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pid != "pid-2" {
			t.Fatalf("expected pid-2, got %q", pid)
		}
	})
}
