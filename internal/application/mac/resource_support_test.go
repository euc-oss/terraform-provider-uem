package macapplication

import (
	"net/http"
	"strings"
	"testing"
)

func TestCreateBlobUploadOptions(t *testing.T) {
	opts := CreateBlobUploadOptions("a.dmg", true, 10)
	if opts.FileName != "a.dmg" || opts.OrganizationGroupID != 10 {
		t.Fatalf("unexpected options: %#v", opts)
	}
	if opts.ModuleType == nil || *opts.ModuleType != "Application" {
		t.Fatalf("expected module type Application, got %#v", opts.ModuleType)
	}

	optsNoModule := CreateBlobUploadOptions("b.plist", false, 11)
	if optsNoModule.ModuleType != nil {
		t.Fatalf("expected nil module type for non-application blob, got %#v", optsNoModule.ModuleType)
	}
}

func TestCreateMacOSApplicationRequest(t *testing.T) {
	req := CreateMacOSApplicationRequest(1, 2, "1.2.3")
	if req.ApplicationBlobID == nil || *req.ApplicationBlobID != 1 {
		t.Fatalf("unexpected application blob id: %#v", req.ApplicationBlobID)
	}
	if req.PkgInfoBlobID == nil || *req.PkgInfoBlobID != 2 {
		t.Fatalf("unexpected pkg info blob id: %#v", req.PkgInfoBlobID)
	}
	if req.Version != "1.2.3" {
		t.Fatalf("unexpected version: %q", req.Version)
	}
}

func TestParseAppIDFromLocation(t *testing.T) {
	header := http.Header{}
	header.Set("Location", "/API/mam/apps/internal/12345")
	id, err := parseAppIDFromLocation(header)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 12345 {
		t.Fatalf("unexpected id: %d", id)
	}

	header.Set("Location", "")
	if _, err := parseAppIDFromLocation(header); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty location error, got: %v", err)
	}
}
