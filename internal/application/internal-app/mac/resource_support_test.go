package macapplication

import (
	"net/http"
	"strings"
	"testing"
)

var (
	pngMagicBytes  = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 'r', 'e', 's', 't'}
	jpegMagicBytes = []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	gifMagicBytes  = []byte("GIF89a rest of a fake gif")
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
	req := CreateMacOSApplicationRequest(1, 2, nil, "1.2.3")
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

// TestBlobFileName covers B31: the downloaded application blob is named by
// its actual content (a xar archive is a flat package, everything else is
// treated as a DMG), never a hardcoded assumption.
func TestBlobFileName(t *testing.T) {
	tests := []struct {
		name  string
		bytes []byte
		want  string
	}{
		{name: "xar magic plus junk is a flat package", bytes: []byte("xar!\x00\x01some-more-bytes"), want: "app.pkg"},
		{name: "exact xar magic with nothing else", bytes: []byte("xar!"), want: "app.pkg"},
		{name: "zlib/koly-style DMG header is not a package", bytes: []byte("koly\x00\x00\x00\x04more-dmg-bytes"), want: "app.dmg"},
		{name: "random bytes are not a package", bytes: []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, want: "app.dmg"},
		{name: "empty input", bytes: []byte{}, want: "app.dmg"},
		{name: "nil input", bytes: nil, want: "app.dmg"},
		{name: "short input shorter than the magic", bytes: []byte("xar"), want: "app.dmg"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := blobFileName(tc.bytes); got != tc.want {
				t.Fatalf("blobFileName(%q) = %q, want %q", tc.bytes, got, tc.want)
			}
		})
	}
}

// TestIconFileName covers B47: the icon is saved with its real image
// extension, sniffed from content, so re-creating an imported app uploads a
// blob UEM can type. Revert check: hardcoding iconFileName to always return
// importedIconFileName (dropping both the uemFileName and sniffing
// branches) makes every non-"unknown bytes" case here fail.
func TestIconFileName(t *testing.T) {
	tests := []struct {
		name        string
		firstBytes  []byte
		uemFileName string
		want        string
	}{
		{name: "png magic bytes, no UEM name", firstBytes: pngMagicBytes, want: "icon.png"},
		{name: "jpeg magic bytes, no UEM name", firstBytes: jpegMagicBytes, want: "icon.jpg"},
		{name: "gif magic bytes, no UEM name", firstBytes: gifMagicBytes, want: "icon.gif"},
		{name: "unknown bytes, no UEM name", firstBytes: []byte("not an image"), want: "icon"},
		{name: "empty bytes, no UEM name", firstBytes: nil, want: "icon"},
		{name: "UEM name wins over sniffing", firstBytes: gifMagicBytes, uemFileName: "AppIcon.icns", want: "icon.icns"},
		{name: "UEM name with unusable extension falls back to sniffing", firstBytes: pngMagicBytes, uemFileName: "AppIcon", want: "icon.png"},
		{name: "UEM name and unknown bytes both unusable stays extension-less", firstBytes: []byte("not an image"), uemFileName: "AppIcon", want: "icon"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := iconFileName(tc.firstBytes, tc.uemFileName); got != tc.want {
				t.Fatalf("iconFileName(%q, %q) = %q, want %q", tc.firstBytes, tc.uemFileName, got, tc.want)
			}
		})
	}
}

func TestUsableFileExt(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "simple extension", in: "icon.png", want: ".png"},
		{name: "uppercase extension is lowercased", in: "ICON.PNG", want: ".png"},
		{name: "longer extension", in: "icon.icns", want: ".icns"},
		{name: "no dot at all", in: "icon", want: ""},
		{name: "empty name", in: "", want: ""},
		{name: "trailing dot with nothing after", in: "icon.", want: ""},
		{name: "extension only, nothing before the dot", in: ".png", want: ""},
		{name: "non-alphanumeric suffix", in: "icon.p-1", want: ""},
		{name: "too-long suffix", in: "icon.abcdefg", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := usableFileExt(tc.in); got != tc.want {
				t.Fatalf("usableFileExt(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestContentDispositionFileName(t *testing.T) {
	h := http.Header{}
	h.Set("Content-Disposition", `attachment; filename=test_upload.xml`)
	if got := contentDispositionFileName(h); got != "test_upload.xml" {
		t.Fatalf("contentDispositionFileName = %q, want test_upload.xml", got)
	}

	h2 := http.Header{}
	h2.Set("Content-Disposition", `attachment; filename="my icon.png"`)
	if got := contentDispositionFileName(h2); got != "my icon.png" {
		t.Fatalf("contentDispositionFileName (quoted) = %q, want %q", got, "my icon.png")
	}

	if got := contentDispositionFileName(nil); got != "" {
		t.Fatalf("nil headers: got %q, want empty", got)
	}
	if got := contentDispositionFileName(http.Header{}); got != "" {
		t.Fatalf("missing header: got %q, want empty", got)
	}

	h3 := http.Header{}
	h3.Set("Content-Disposition", "not a valid media type;;;")
	if got := contentDispositionFileName(h3); got != "" {
		t.Fatalf("unparsable header: got %q, want empty", got)
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
