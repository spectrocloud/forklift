package util

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	api "github.com/kubev2v/forklift/pkg/apis/forklift/v1beta1"
	core "k8s.io/api/core/v1"
)

func TestExtractServerName(t *testing.T) {
	if got := extractServerName("example.com:443"); got != "example.com" {
		t.Fatalf("unexpected host: %s", got)
	}
	if got := extractServerName("example.com"); got != "example.com" {
		t.Fatalf("unexpected host: %s", got)
	}
	if got := extractServerName("example.com:"); got != "example.com" {
		t.Fatalf("unexpected host: %s", got)
	}
}

func TestInsecureProvider(t *testing.T) {
	sec := &core.Secret{Data: map[string][]byte{}}
	if InsecureProvider(sec) {
		t.Fatalf("expected false when not set")
	}
	sec.Data[api.Insecure] = []byte("true")
	if !InsecureProvider(sec) {
		t.Fatalf("expected true")
	}
	sec.Data[api.Insecure] = []byte("notabool")
	if InsecureProvider(sec) {
		t.Fatalf("expected false on parse error")
	}
}

func TestFingerprint(t *testing.T) {
	cert := &x509.Certificate{Raw: []byte{0x01, 0x02, 0x03}}
	fp := Fingerprint(cert)
	if fp == "" || fp[2] != ':' {
		t.Fatalf("unexpected fingerprint: %q", fp)
	}
}

func TestTLSConfigBranches(t *testing.T) {
	// InsecureProvider branch.
	sec := &core.Secret{Data: map[string][]byte{api.Insecure: []byte("true")}}
	cfg, err := tlsConfig(sec)
	if err != nil || cfg == nil || !cfg.InsecureSkipVerify {
		t.Fatalf("unexpected insecure config: cfg=%#v err=%v", cfg, err)
	}

	// cacert branch: invalid PEM => parse error.
	sec2 := &core.Secret{Data: map[string][]byte{"cacert": []byte("not a pem")}}
	_, err = tlsConfig(sec2)
	if err == nil {
		t.Fatalf("expected error for invalid cacert")
	}

	// cacert branch: valid PEM for a certificate (parsing should succeed).
	p := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: selfSignedDER(t)})
	sec3 := &core.Secret{Data: map[string][]byte{"cacert": p}}
	cfg, err = tlsConfig(sec3)
	if err != nil || cfg == nil || cfg.RootCAs == nil {
		t.Fatalf("unexpected cacert config: cfg=%#v err=%v", cfg, err)
	}
}

func selfSignedDER(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Minute),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, priv.Public(), priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return der
}
