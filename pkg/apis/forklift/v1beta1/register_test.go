package v1beta1

import "testing"

func TestGetGroupResource(t *testing.T) {
	gr, err := GetGroupResource(&Provider{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gr.Resource != "providers" {
		t.Fatalf("unexpected resource: %#v", gr)
	}

	gr, err = GetGroupResource(&Plan{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gr.Resource != "plans" {
		t.Fatalf("unexpected resource: %#v", gr)
	}

	gr, err = GetGroupResource(&Migration{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if gr.Resource != "migrations" {
		t.Fatalf("unexpected resource: %#v", gr)
	}

	_, err = GetGroupResource(&NetworkMap{})
	if err == nil {
		t.Fatalf("expected error for unknown type")
	}
}
