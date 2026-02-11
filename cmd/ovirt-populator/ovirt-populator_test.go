package main

import (
	"os"
	"testing"
)

func TestGetEnvAsBool_DefaultAndParse(t *testing.T) {
	// Default when unset.
	os.Unsetenv("insecureSkipVerify")
	if got := getEnvAsBool("insecureSkipVerify", false); got != false {
		t.Fatalf("expected default false, got %v", got)
	}

	t.Setenv("insecureSkipVerify", "true")
	if got := getEnvAsBool("insecureSkipVerify", false); got != true {
		t.Fatalf("expected true, got %v", got)
	}
}

func TestLoadEngineConfig_ReadsEnv(t *testing.T) {
	t.Setenv("user", "u")
	t.Setenv("password", "p")
	t.Setenv("cacert", "ca")
	t.Setenv("insecureSkipVerify", "true")

	cfg := loadEngineConfig("https://engine.example.invalid")
	if cfg.URL != "https://engine.example.invalid" {
		t.Fatalf("unexpected url: %s", cfg.URL)
	}
	if cfg.username != "u" || cfg.password != "p" || cfg.cacert != "ca" {
		t.Fatalf("unexpected cfg: %#v", cfg)
	}
	if !cfg.insecure {
		t.Fatalf("expected insecure true")
	}
}

func TestCreateCommandArguments_InsecureAndSecure(t *testing.T) {
	secure := &engineConfig{
		URL:      "https://engine.example.invalid",
		username: "u",
		password: "p",
		cacert:   "ca",
		insecure: false,
	}
	args := createCommandArguments(secure, "disk-1", "/vol")
	foundCA := false
	for _, a := range args {
		if a == "--cafile=/tmp/ca.pem" {
			foundCA = true
		}
	}
	if !foundCA {
		t.Fatalf("expected cafile arg, got %#v", args)
	}

	insecure := &engineConfig{
		URL:      "https://engine.example.invalid",
		username: "u",
		password: "p",
		insecure: true,
	}
	args2 := createCommandArguments(insecure, "disk-1", "/vol")
	foundInsecure := false
	for _, a := range args2 {
		if a == "--insecure" {
			foundInsecure = true
		}
	}
	if !foundInsecure {
		t.Fatalf("expected insecure arg, got %#v", args2)
	}
}
