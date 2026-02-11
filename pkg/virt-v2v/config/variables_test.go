package config

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetFlags(t *testing.T) {
	t.Helper()
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
}

func TestGetEnvBool(t *testing.T) {
	var c AppConfig
	t.Setenv("XBOOL", "true")
	if got := c.getEnvBool("XBOOL", false); got != true {
		t.Fatalf("expected true, got %v", got)
	}
	t.Setenv("XBOOL", "notabool")
	if got := c.getEnvBool("XBOOL", true); got != true {
		t.Fatalf("expected default on parse error, got %v", got)
	}
}

func TestGetExtraArgs(t *testing.T) {
	var c AppConfig
	t.Setenv(EnvExtraArgsName, `["--a","b"]`)
	args := c.getExtraArgs()
	if len(args) != 2 || args[0] != "--a" || args[1] != "b" {
		t.Fatalf("unexpected args: %#v", args)
	}

	t.Setenv(EnvExtraArgsName, `not-json`)
	if got := c.getExtraArgs(); got != nil {
		t.Fatalf("expected nil on invalid json, got %#v", got)
	}
}

func TestValidate_OVA_MissingEnv(t *testing.T) {
	c := &AppConfig{Source: OVA, IsInPlace: false}
	if err := c.validate(); err == nil || !strings.Contains(err.Error(), EnvDiskPathName) {
		t.Fatalf("expected missing disk-path error, got %v", err)
	}
	c.DiskPath = "/tmp/disk"
	if err := c.validate(); err == nil || !strings.Contains(err.Error(), EnvVmNameName) {
		t.Fatalf("expected missing vm-name error, got %v", err)
	}
}

func TestValidate_VSphere_MissingEnv(t *testing.T) {
	c := &AppConfig{Source: VSPHERE, IsInPlace: false}
	if err := c.validate(); err == nil || !strings.Contains(err.Error(), EnvLibvirtUrlName) {
		t.Fatalf("expected missing libvirt url error, got %v", err)
	}
	c.LibvirtUrl = "qemu+ssh://example"
	if err := c.validate(); err == nil || !strings.Contains(err.Error(), EnvVmNameName) {
		t.Fatalf("expected missing vm-name error, got %v", err)
	}
	c.VmName = "vm1"
	c.SecretKey = ""
	if err := c.validate(); err == nil || !strings.Contains(err.Error(), SecretKey) {
		t.Fatalf("expected missing secret-key error, got %v", err)
	}
}

func TestValidate_VSphere_LegacyDriversMissing_UnsetsEnv(t *testing.T) {
	tmp := t.TempDir()
	missingISO := filepath.Join(tmp, "nope.iso")
	t.Setenv(EnvVirtIoWinLegacyDriversName, missingISO)

	c := &AppConfig{
		Source:                 VSPHERE,
		LibvirtUrl:             "qemu+ssh://example",
		VmName:                 "vm1",
		SecretKey:              "/tmp/secret-does-not-matter",
		VirtIoWinLegacyDrivers: missingISO,
	}
	if err := c.validate(); err != nil {
		t.Fatalf("expected validate to succeed (and unset env), got %v", err)
	}
	if _, found := os.LookupEnv(EnvVirtIoWinLegacyDriversName); found {
		t.Fatalf("expected %s to be unset", EnvVirtIoWinLegacyDriversName)
	}
}

func TestValidate_VSphere_SecretKeyMissing_ReturnsStatError(t *testing.T) {
	tmp := t.TempDir()
	missingSecret := filepath.Join(tmp, "missing")
	c := &AppConfig{
		Source:     VSPHERE,
		LibvirtUrl: "qemu+ssh://example",
		VmName:     "vm1",
		SecretKey:  missingSecret,
	}
	if err := c.validate(); err == nil {
		t.Fatalf("expected stat error")
	}
}

func TestValidate_InvalidSource(t *testing.T) {
	c := &AppConfig{Source: "nope", IsInPlace: false}
	if err := c.validate(); err == nil {
		t.Fatalf("expected invalid source error")
	}
}

func TestLoad_UsesEnvAndFlagsAndValidates(t *testing.T) {
	resetFlags(t)
	t.Setenv(EnvSourceName, OVA)
	t.Setenv(EnvDiskPathName, "/tmp/disk")
	t.Setenv(EnvVmNameName, "vm1")

	// No extra flags.
	os.Args = []string{"cmd"}

	var c AppConfig
	if err := c.Load(); err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if c.Source != OVA || c.DiskPath != "/tmp/disk" || c.VmName != "vm1" {
		t.Fatalf("unexpected loaded config: %#v", c)
	}
	if c.IsVsphereMigration() {
		t.Fatalf("expected not vsphere migration")
	}
}
