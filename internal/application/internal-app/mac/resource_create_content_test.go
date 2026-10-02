package macapplication

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac/models"
)

// fakeBlobV1UploadService is an injected test double for
// appBlobV1ResourceServiceAPI that records every uploaded payload and hands
// back a distinct blob ID/UUID per call, mirroring the real V1 blob service
// used by Create's two uploadBlob calls (DMG then plist).
type fakeBlobV1UploadService struct {
	mu        sync.Mutex
	uploads   [][]byte
	fileNames []string // B47: opts.FileName for each call, same index as uploads
	nextID    int
	err       error
}

func (f *fakeBlobV1UploadService) UploadBlobAsync(
	ctx context.Context,
	request []byte,
	opts *sdk.BlobsV1UploadBlobAsyncOptions,
) (http.Header, *sdk.EntityV1Model, error) {
	if f.err != nil {
		return nil, nil, f.err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.uploads = append(f.uploads, append([]byte(nil), request...))
	f.fileNames = append(f.fileNames, opts.FileName)
	f.nextID++
	id := f.nextID
	return nil, &sdk.EntityV1Model{Value: &id, UUID: fmt.Sprintf("blob-uuid-%d", id)}, nil
}

// fakeMacAppCreateService is an injected test double for
// macAppResourceServiceAPI. It reports the created application's numeric ID
// via the Location header, matching parseAppIDFromLocation's expectations.
type fakeMacAppCreateService struct {
	appID int
}

func (f *fakeMacAppCreateService) CreateMacOSApplication(
	ctx context.Context,
	ID int,
	request *sdk.MacOsCreateApplicationRequestV1Model,
) (http.Header, error) {
	h := http.Header{}
	h.Set("Location", fmt.Sprintf("/API/mam/apps/internal/%d", f.appID))
	return h, nil
}

// buildMacApplicationPlan constructs a tfsdk.Plan matching the resource
// schema, using the supplied known values for the attributes given and
// leaving every other attribute Unknown — matching how Terraform core
// presents a real plan for Optional+Computed / Computed-only attributes with
// no configured value.
func buildMacApplicationPlan(known map[string]tftypes.Value) tfsdk.Plan {
	var schemaResp resource.SchemaResponse
	(&macapplicationResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	ctx := context.Background()
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)
	objType, ok := schemaType.(tftypes.Object)
	if !ok {
		panic("schema type is not an Object")
	}

	values := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, attrType := range objType.AttributeTypes {
		if v, ok := known[name]; ok {
			values[name] = v
			continue
		}
		values[name] = tftypes.NewValue(attrType, tftypes.UnknownValue)
	}

	return tfsdk.Plan{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaType, values),
	}
}

// runCreateWithFixtures exercises Create end-to-end against fakes, writing a
// DMG and plist file to disk with distinct known content, and returns the
// resulting state model plus the raw DMG/plist bytes used so callers can
// verify computed hash/content attributes.
func runCreateWithFixtures(t *testing.T, includeContent bool) (tf.MacApplicationResourceModel, []byte, []byte) {
	t.Helper()

	tmpDir := t.TempDir()
	dmgBytes := []byte("fake-dmg-binary-content-for-create-test")
	plistBytes := []byte("<plist>fake-pkginfo-for-create-test</plist>")

	dmgPath := filepath.Join(tmpDir, "app.dmg")
	plistPath := filepath.Join(tmpDir, "app.plist")
	if err := os.WriteFile(dmgPath, dmgBytes, 0o644); err != nil {
		t.Fatalf("failed to write dmg fixture: %v", err)
	}
	if err := os.WriteFile(plistPath, plistBytes, 0o644); err != nil {
		t.Fatalf("failed to write plist fixture: %v", err)
	}

	plan := buildMacApplicationPlan(map[string]tftypes.Value{
		"org_group_id":    tftypes.NewValue(tftypes.Number, 10),
		"dmg_file_path":   tftypes.NewValue(tftypes.String, dmgPath),
		"plist_file_path": tftypes.NewValue(tftypes.String, plistPath),
		"app_version":     tftypes.NewValue(tftypes.String, "1.0.0"),
		"include_content": tftypes.NewValue(tftypes.Bool, includeContent),
	})

	blobFake := &fakeBlobV1UploadService{}
	macAppFake := &fakeMacAppCreateService{appID: 999}
	internalFake := &fakeInternalAppsV1Service{
		model: &sdk.InternalAppModelV1{ID: intPtr(999), UUID: "3ba19203-a843-a493-4e49-e5383f9b9cf7"},
	}

	r := &macapplicationResource{
		client: &sdk.Client{},
		newBlobV1ResourceService: func(*sdk.Client) appBlobV1ResourceServiceAPI {
			return blobFake
		},
		newMacAppResourceService: func(*sdk.Client) macAppResourceServiceAPI {
			return macAppFake
		},
		newInternalAppResourceService: func(*sdk.Client) InternalAppsV1ServiceAPI {
			return internalFake
		},
	}

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{
		State: tfsdk.State(plan),
	}

	r.Create(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics error from Create: %v", resp.Diagnostics)
	}

	var data tf.MacApplicationResourceModel
	diags := resp.State.Get(context.Background(), &data)
	if diags.HasError() {
		t.Fatalf("unexpected error reading state after Create: %v", diags)
	}

	if len(blobFake.uploads) != 2 {
		t.Fatalf("expected 2 blob uploads (dmg + plist), got %d", len(blobFake.uploads))
	}

	return data, dmgBytes, plistBytes
}

// TestCreate_PopulatesDMGFileSHA256 proves that right after a real Create
// (not just after ImportState), dmg_file_sha256 is populated with the actual
// SHA-256 of the uploaded DMG bytes rather than being left null until a
// later `terraform import`.
func TestCreate_PopulatesDMGFileSHA256(t *testing.T) {
	data, dmgBytes, _ := runCreateWithFixtures(t, false)

	sum := sha256.Sum256(dmgBytes)
	wantSHA := hex.EncodeToString(sum[:])

	if data.DMGFileSHA256.IsNull() || data.DMGFileSHA256.IsUnknown() {
		t.Fatalf("expected dmg_file_sha256 to be populated after Create, got %#v", data.DMGFileSHA256)
	}
	if got := data.DMGFileSHA256.ValueString(); got != wantSHA {
		t.Fatalf("expected dmg_file_sha256 %q, got %q", wantSHA, got)
	}
}

// TestCreate_IncludeContentTrue_PopulatesContentBase64 proves that when
// include_content = true, Create populates dmg_content_base64 and
// plist_content_base64 with the base64 of the actually uploaded bytes.
func TestCreate_IncludeContentTrue_PopulatesContentBase64(t *testing.T) {
	data, dmgBytes, plistBytes := runCreateWithFixtures(t, true)

	wantDMGB64 := base64.StdEncoding.EncodeToString(dmgBytes)
	wantPlistB64 := base64.StdEncoding.EncodeToString(plistBytes)

	if data.DMGContentBase64.IsNull() || data.DMGContentBase64.IsUnknown() {
		t.Fatalf("expected dmg_content_base64 to be populated after Create, got %#v", data.DMGContentBase64)
	}
	if got := data.DMGContentBase64.ValueString(); got != wantDMGB64 {
		t.Fatalf("expected dmg_content_base64 %q, got %q", wantDMGB64, got)
	}

	if data.PlistContentBase64.IsNull() || data.PlistContentBase64.IsUnknown() {
		t.Fatalf("expected plist_content_base64 to be populated after Create, got %#v", data.PlistContentBase64)
	}
	if got := data.PlistContentBase64.ValueString(); got != wantPlistB64 {
		t.Fatalf("expected plist_content_base64 %q, got %q", wantPlistB64, got)
	}
}

// TestCreate_IncludeContentFalse_LeavesContentBase64Null proves that when
// include_content = false (or unset), Create explicitly nulls out
// dmg_content_base64 / plist_content_base64 rather than leaving them at
// whatever prior/zero value the model happened to carry.
func TestCreate_IncludeContentFalse_LeavesContentBase64Null(t *testing.T) {
	data, _, _ := runCreateWithFixtures(t, false)

	if !data.DMGContentBase64.IsNull() {
		t.Fatalf("expected dmg_content_base64 to be null when include_content=false, got %#v", data.DMGContentBase64)
	}
	if !data.PlistContentBase64.IsNull() {
		t.Fatalf("expected plist_content_base64 to be null when include_content=false, got %#v", data.PlistContentBase64)
	}
}

// TestCreate_IconFilePath_UploadsBlobNamedByBase covers B47's other half: the
// create path already names an uploaded blob via filepath.Base(filePath)
// (uploadBlob, resource_crud.go), so once ImportState/state holds an
// icon_file_path ending in "icon.png" (this task's fix), re-creating that
// app in a new environment uploads a blob UEM can type — no create-side
// code change was needed, but this pins the behavior the fix depends on.
func TestCreate_IconFilePath_UploadsBlobNamedByBase(t *testing.T) {
	tmpDir := t.TempDir()
	dmgBytes := []byte("fake-dmg-binary-content-for-icon-create-test")
	plistBytes := []byte("<plist>fake-pkginfo-for-icon-create-test</plist>")
	iconBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 'r', 'e', 's', 't'}

	dmgPath := filepath.Join(tmpDir, "app.dmg")
	plistPath := filepath.Join(tmpDir, "app.plist")
	iconPath := filepath.Join(tmpDir, "icon.png")
	if err := os.WriteFile(dmgPath, dmgBytes, 0o644); err != nil {
		t.Fatalf("failed to write dmg fixture: %v", err)
	}
	if err := os.WriteFile(plistPath, plistBytes, 0o644); err != nil {
		t.Fatalf("failed to write plist fixture: %v", err)
	}
	if err := os.WriteFile(iconPath, iconBytes, 0o644); err != nil {
		t.Fatalf("failed to write icon fixture: %v", err)
	}

	plan := buildMacApplicationPlan(map[string]tftypes.Value{
		"org_group_id":    tftypes.NewValue(tftypes.Number, 10),
		"dmg_file_path":   tftypes.NewValue(tftypes.String, dmgPath),
		"plist_file_path": tftypes.NewValue(tftypes.String, plistPath),
		"icon_file_path":  tftypes.NewValue(tftypes.String, iconPath),
		"app_version":     tftypes.NewValue(tftypes.String, "1.0.0"),
		"include_content": tftypes.NewValue(tftypes.Bool, false),
	})

	blobFake := &fakeBlobV1UploadService{}
	macAppFake := &fakeMacAppCreateService{appID: 999}
	internalFake := &fakeInternalAppsV1Service{
		model: &sdk.InternalAppModelV1{ID: intPtr(999), UUID: "3ba19203-a843-a493-4e49-e5383f9b9cf7"},
	}

	r := &macapplicationResource{
		client: &sdk.Client{},
		newBlobV1ResourceService: func(*sdk.Client) appBlobV1ResourceServiceAPI {
			return blobFake
		},
		newMacAppResourceService: func(*sdk.Client) macAppResourceServiceAPI {
			return macAppFake
		},
		newInternalAppResourceService: func(*sdk.Client) InternalAppsV1ServiceAPI {
			return internalFake
		},
	}

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: tfsdk.State(plan)}

	r.Create(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics error from Create: %v", resp.Diagnostics)
	}

	if len(blobFake.fileNames) != 3 {
		t.Fatalf("expected 3 blob uploads (dmg + plist + icon), got %d: %v", len(blobFake.fileNames), blobFake.fileNames)
	}
	if got := blobFake.fileNames[2]; got != "icon.png" {
		t.Fatalf("expected the icon blob to be uploaded as %q (filepath.Base of icon_file_path), got %q", "icon.png", got)
	}
}
