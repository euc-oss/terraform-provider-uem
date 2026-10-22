package macapplication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac/models"
	"github.com/euc-oss/terraform-provider-uem/internal/httpclient"
)

// fakeAppBlobDownloader is an injected test double for appBlobDownloader.
// InfoName/infoSizeKB are what onInfo is called with (simulating B10's
// pre-download name/size announcement); progressReads (if non-empty) are fed
// to the progress callback (as (read, total) pairs against progressTotal)
// before returning err/success, simulating in-flight byte progress.
type fakeAppBlobDownloader struct {
	calledWithUUID string
	id             int
	blob           []byte
	pkginfo        string
	err            error

	infoName   string
	infoSizeKB *int

	progressReads []int64
	progressTotal int64

	onInfoCalled   bool
	progressCalled []int64
}

func (f *fakeAppBlobDownloader) DownloadAppBlob(ctx context.Context, appUUID string, onInfo appInfoFunc, progress httpclient.ProgressFunc) (int, []byte, string, error) {
	f.calledWithUUID = appUUID
	if onInfo != nil {
		f.onInfoCalled = true
		onInfo(f.infoName, f.infoSizeKB)
	}
	if progress != nil {
		for _, read := range f.progressReads {
			f.progressCalled = append(f.progressCalled, read)
			progress(read, f.progressTotal)
		}
	}
	if f.err != nil {
		return 0, nil, "", f.err
	}
	return f.id, f.blob, f.pkginfo, nil
}

func newImportStateResponse() *resource.ImportStateResponse {
	return &resource.ImportStateResponse{
		State: emptyMacApplicationState(),
	}
}

// emptyMacApplicationState builds a null-valued tfsdk.State matching the
// resource schema, suitable as the starting state for ImportState tests.
func emptyMacApplicationState() tfsdk.State {
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
		values[name] = tftypes.NewValue(attrType, nil)
	}

	return tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaType, values),
	}
}

func TestImportState_BareScalarID_DerivesPathAndWritesBlob(t *testing.T) {
	tmpDir := t.TempDir()
	blobBytes := []byte("fake-dmg-binary-content")
	fake := &fakeAppBlobDownloader{blob: blobBytes, id: 42}

	r := &macapplicationResource{
		appBinaryStoragePath: tmpDir,
		blobDownloader:       fake,
	}

	req := resource.ImportStateRequest{ID: "9af645a8-fef3-3e6d-3408-5cc69e0937d4"}
	resp := newImportStateResponse()

	r.ImportState(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics error: %v", resp.Diagnostics)
	}

	if fake.calledWithUUID != "9af645a8-fef3-3e6d-3408-5cc69e0937d4" {
		t.Fatalf("expected downloader called with %q, got %q", "9af645a8-fef3-3e6d-3408-5cc69e0937d4", fake.calledWithUUID)
	}

	expectedPath := filepath.Join(tmpDir, "9af645a8-fef3-3e6d-3408-5cc69e0937d4", "app.dmg")
	writtenBytes, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("expected blob written to %s: %v", expectedPath, err)
	}
	if string(writtenBytes) != string(blobBytes) {
		t.Fatalf("expected written bytes %q, got %q", blobBytes, writtenBytes)
	}

	var data tf.MacApplicationResourceModel
	diags := resp.State.Get(context.Background(), &data)
	if diags.HasError() {
		t.Fatalf("unexpected error reading state: %v", diags)
	}

	if data.DMGFilePath.ValueString() != expectedPath {
		t.Fatalf("expected dmg_file_path %q, got %q", expectedPath, data.DMGFilePath.ValueString())
	}

	sum := sha256.Sum256(blobBytes)
	expectedSHA := hex.EncodeToString(sum[:])
	if data.DMGFileSHA256.ValueString() != expectedSHA {
		t.Fatalf("expected dmg_file_sha256 %q, got %q", expectedSHA, data.DMGFileSHA256.ValueString())
	}

	if data.UUID.ValueString() != "9af645a8-fef3-3e6d-3408-5cc69e0937d4" {
		t.Fatalf("expected uuid %q, got %q", "9af645a8-fef3-3e6d-3408-5cc69e0937d4", data.UUID.ValueString())
	}

	if data.ID.IsNull() || data.ID.ValueInt32() != 42 {
		t.Fatalf("expected id 42, got %v", data.ID)
	}

	if !data.OrgGroupID.IsNull() {
		t.Fatalf("expected org_group_id to remain null for bare-uuid import, got %v", data.OrgGroupID)
	}

	foundWarning := false
	for _, d := range resp.Diagnostics {
		if d.Severity() == diag.SeverityWarning && d.Summary() == "org_group_id Not Set by Import" {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatal("expected org_group_id Not Set by Import warning diagnostic")
	}
}

// TestImportState_RelativeStorageRoot_RecordsRelativePath is the f10 fix's
// revert check: when appBinaryStoragePath is a RELATIVE path (as it always
// is for the CLI's own generated provider.tf — see cli's
// onboardProviderConfigTmpl doc comment, and this package's artifactDir/
// dmgPath doc comments), ImportState must record a relative dmg_file_path in
// state, never absolutise it via filepath.Abs/os.Getwd. Absolutising here
// would defeat the whole point of the CLI's fix (embedding a plain,
// workDir-relative literal instead of a "${path.root}/..." interpolation
// that `terraform import` — uniquely among Terraform operations — resolves
// to an absolute path): the state value must stay exactly as
// filepath.Clean(join(storageRoot, uuid, name)) yields it, relative in,
// relative out, so a later `terraform plan`'s relative config value
// Clean-compares equal (cleanEquivalentPathUseState, plan_modifiers.go) and
// the whole repo stays relocatable.
func TestImportState_RelativeStorageRoot_RecordsRelativePath(t *testing.T) {
	// A real filesystem layout: cwd/workdir has app-binaries two levels up,
	// mirroring the CLI's own <repoRoot>/<tenant>/.ws1tf-onboard-work ->
	// <repoRoot>/uem-artifacts/<tenant>/app-binaries hop count.
	repoRoot := t.TempDir()
	workDir := filepath.Join(repoRoot, "prod", ".ws1tf-onboard-work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir workDir: %v", err)
	}
	t.Chdir(workDir)

	const relStorageRoot = "../../uem-artifacts/prod/app-binaries"
	blobBytes := []byte("fake-dmg-binary-content")
	fake := &fakeAppBlobDownloader{blob: blobBytes, id: 7}

	r := &macapplicationResource{
		appBinaryStoragePath: relStorageRoot,
		blobDownloader:       fake,
	}

	req := resource.ImportStateRequest{ID: "9af645a8-fef3-3e6d-3408-5cc69e0937d4"}
	resp := newImportStateResponse()
	r.ImportState(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics error: %v", resp.Diagnostics)
	}

	var data tf.MacApplicationResourceModel
	if diags := resp.State.Get(context.Background(), &data); diags.HasError() {
		t.Fatalf("unexpected error reading state: %v", diags)
	}

	got := data.DMGFilePath.ValueString()
	if filepath.IsAbs(got) {
		t.Fatalf("expected a relative dmg_file_path (storage root was relative), got absolute %q", got)
	}
	want := filepath.Clean(filepath.Join(relStorageRoot, "9af645a8-fef3-3e6d-3408-5cc69e0937d4", "app.dmg"))
	if got != want {
		t.Fatalf("dmg_file_path = %q, want %q", got, want)
	}

	// Confirm the file really landed on disk under the relative path,
	// resolved against the process cwd (workDir) — proving the relative
	// value recorded in state is actually usable, not just cosmetically
	// relative.
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("expected blob written at relative path %s (cwd %s): %v", got, workDir, err)
	}
}

// TestImportState_XarBlob_SavesAsAppPKG is B31: when the downloaded blob is
// a flat package (a xar archive, identified by its "xar!" magic bytes),
// ImportState must save it as app.pkg, not the hardcoded app.dmg — a real
// .pkg saved with a .dmg extension is otherwise indistinguishable from a
// corrupted DMG.
func TestImportState_XarBlob_SavesAsAppPKG(t *testing.T) {
	tmpDir := t.TempDir()
	blobBytes := append([]byte("xar!"), []byte("\x00\x01fake-flat-package-content")...)
	fake := &fakeAppBlobDownloader{blob: blobBytes, id: 99}

	r := &macapplicationResource{
		appBinaryStoragePath: tmpDir,
		blobDownloader:       fake,
	}

	req := resource.ImportStateRequest{ID: "9af645a8-fef3-3e6d-3408-5cc69e0937d4"}
	resp := newImportStateResponse()

	r.ImportState(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics error: %v", resp.Diagnostics)
	}

	expectedPath := filepath.Join(tmpDir, "9af645a8-fef3-3e6d-3408-5cc69e0937d4", "app.pkg")
	writtenBytes, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("expected blob written to %s: %v", expectedPath, err)
	}
	if string(writtenBytes) != string(blobBytes) {
		t.Fatalf("expected written bytes %q, got %q", blobBytes, writtenBytes)
	}

	var data tf.MacApplicationResourceModel
	diags := resp.State.Get(context.Background(), &data)
	if diags.HasError() {
		t.Fatalf("unexpected error reading state: %v", diags)
	}

	if !strings.HasSuffix(data.DMGFilePath.ValueString(), "app.pkg") {
		t.Fatalf("expected dmg_file_path to end in app.pkg, got %q", data.DMGFilePath.ValueString())
	}
	if data.DMGFilePath.ValueString() != expectedPath {
		t.Fatalf("expected dmg_file_path %q, got %q", expectedPath, data.DMGFilePath.ValueString())
	}
}

func TestImportState_InvalidID_ReturnsDiagnosticError(t *testing.T) {
	fake := &fakeAppBlobDownloader{}
	r := &macapplicationResource{
		appBinaryStoragePath: t.TempDir(),
		blobDownloader:       fake,
	}

	req := resource.ImportStateRequest{ID: "   "}
	resp := newImportStateResponse()

	r.ImportState(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error for blank import ID")
	}
	if fake.calledWithUUID != "" {
		t.Fatal("expected downloader not to be called for invalid ID")
	}
}

func TestImportState_NilDownloader_ReturnsDiagnosticError(t *testing.T) {
	r := &macapplicationResource{
		appBinaryStoragePath: t.TempDir(),
	}

	req := resource.ImportStateRequest{ID: "9af645a8-fef3-3e6d-3408-5cc69e0937d4"}
	resp := newImportStateResponse()

	r.ImportState(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error when blobDownloader is nil")
	}
}

func TestImportState_PathTraversalID_ReturnsDiagnosticError(t *testing.T) {
	tmpDir := t.TempDir()
	fake := &fakeAppBlobDownloader{blob: []byte("x")}
	r := &macapplicationResource{
		appBinaryStoragePath: tmpDir,
		blobDownloader:       fake,
	}

	for _, malicious := range []string{"../../../etc/cron.d/evil", "/etc/passwd", "..\\..\\windows"} {
		req := resource.ImportStateRequest{ID: malicious}
		resp := newImportStateResponse()

		r.ImportState(context.Background(), req, resp)

		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected diagnostics error for malicious id %q, got none", malicious)
		}
		if fake.calledWithUUID != "" {
			t.Fatalf("expected downloader not to be called for malicious id %q, got called with %q", malicious, fake.calledWithUUID)
		}
	}
}

func TestImportState_DownloadError_ReturnsDiagnosticError(t *testing.T) {
	fake := &fakeAppBlobDownloader{err: os.ErrNotExist}
	r := &macapplicationResource{
		appBinaryStoragePath: t.TempDir(),
		blobDownloader:       fake,
	}

	req := resource.ImportStateRequest{ID: "9af645a8-fef3-3e6d-3408-5cc69e0937d4"}
	resp := newImportStateResponse()

	r.ImportState(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error when downloader returns an error")
	}
}

func TestImportState_CompositeID_SetsOrgGroupIDWithoutWarning(t *testing.T) {
	tmpDir := t.TempDir()
	fake := &fakeAppBlobDownloader{blob: []byte("fake-dmg-binary-content"), id: 7}
	r := &macapplicationResource{
		appBinaryStoragePath: tmpDir,
		blobDownloader:       fake,
	}

	req := resource.ImportStateRequest{ID: "9af645a8-fef3-3e6d-3408-5cc69e0937d4,555"}
	resp := newImportStateResponse()

	r.ImportState(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics error: %v", resp.Diagnostics)
	}

	for _, d := range resp.Diagnostics {
		if d.Severity() == diag.SeverityWarning {
			t.Fatalf("expected no warning diagnostics for composite import ID, got: %v", d)
		}
	}

	var data tf.MacApplicationResourceModel
	diags := resp.State.Get(context.Background(), &data)
	if diags.HasError() {
		t.Fatalf("unexpected error reading state: %v", diags)
	}

	if data.OrgGroupID.IsNull() || data.OrgGroupID.ValueInt32() != 555 {
		t.Fatalf("expected org_group_id 555, got %v", data.OrgGroupID)
	}
}

func TestImportState_CompositeID_InvalidOrgGroupID_ReturnsDiagnosticError(t *testing.T) {
	fake := &fakeAppBlobDownloader{blob: []byte("x")}
	r := &macapplicationResource{
		appBinaryStoragePath: t.TempDir(),
		blobDownloader:       fake,
	}

	for _, badID := range []string{"not-a-number", "-5", "0", ""} {
		req := resource.ImportStateRequest{ID: "9af645a8-fef3-3e6d-3408-5cc69e0937d4," + badID}
		resp := newImportStateResponse()

		r.ImportState(context.Background(), req, resp)

		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected diagnostics error for invalid org_group_id %q, got none", badID)
		}
		if fake.calledWithUUID != "" {
			t.Fatalf("expected downloader not to be called for invalid org_group_id %q", badID)
		}
	}
}

func TestImportState_TooManyCommaFields_ReturnsDiagnosticError(t *testing.T) {
	fake := &fakeAppBlobDownloader{blob: []byte("x")}
	r := &macapplicationResource{
		appBinaryStoragePath: t.TempDir(),
		blobDownloader:       fake,
	}

	req := resource.ImportStateRequest{ID: "9af645a8-fef3-3e6d-3408-5cc69e0937d4,555,extra"}
	resp := newImportStateResponse()

	r.ImportState(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error for import ID with too many comma-separated fields")
	}
	if fake.calledWithUUID != "" {
		t.Fatal("expected downloader not to be called for malformed import ID")
	}
}

// fakeInternalAppsV1Service is an injected test double for
// InternalAppsV1ServiceAPI, used to exercise Read() after ImportState().
type fakeInternalAppsV1Service struct {
	model *sdk.InternalAppModelV1
	err   error
}

func (f *fakeInternalAppsV1Service) DeleteInternalAppAsync(ctx context.Context, ApplicationID int) (http.Header, error) {
	return nil, fmt.Errorf("DeleteInternalAppAsync not used by this fake")
}

func (f *fakeInternalAppsV1Service) GetInternalAppByIdAsync(ctx context.Context, ApplicationID int) (http.Header, *sdk.InternalAppModelV1, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	return nil, f.model, nil
}

// TestImportState_ThenRead mirrors the real Terraform import lifecycle,
// where Terraform automatically calls Read() immediately after
// ImportState() against the same state. It asserts: (a) id is populated
// after import, (b) org_group_id's post-import value (set or left null)
// survives Read() unchanged — Read() must not crash or silently invent a
// value for it, and (c) the org_group_id warning is present only for the
// bare-uuid form.
func TestImportState_ThenRead(t *testing.T) {
	const appUUID = "9af645a8-fef3-3e6d-3408-5cc69e0937d4"

	testCases := []struct {
		name          string
		importID      string
		wantOrgGroup  bool
		wantOrgGroupV int32
		wantWarning   bool
	}{
		{name: "bare uuid", importID: appUUID, wantOrgGroup: false, wantWarning: true},
		{name: "composite uuid,org_group_id", importID: appUUID + ",555", wantOrgGroup: true, wantOrgGroupV: 555, wantWarning: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			blobFake := &fakeAppBlobDownloader{blob: []byte("fake-dmg-binary-content"), id: 42}
			readFake := &fakeInternalAppsV1Service{
				model: &sdk.InternalAppModelV1{ID: intPtr(42), UUID: appUUID},
			}

			r := &macapplicationResource{
				appBinaryStoragePath: tmpDir,
				blobDownloader:       blobFake,
				client:               &sdk.Client{},
				newInternalAppResourceService: func(*sdk.Client) InternalAppsV1ServiceAPI {
					return readFake
				},
			}

			importResp := newImportStateResponse()
			r.ImportState(context.Background(), resource.ImportStateRequest{ID: tc.importID}, importResp)
			if importResp.Diagnostics.HasError() {
				t.Fatalf("unexpected error from ImportState: %v", importResp.Diagnostics)
			}

			var afterImport tf.MacApplicationResourceModel
			if diags := importResp.State.Get(context.Background(), &afterImport); diags.HasError() {
				t.Fatalf("unexpected error reading state after import: %v", diags)
			}
			if afterImport.ID.IsNull() || afterImport.ID.ValueInt32() != 42 {
				t.Fatalf("expected id 42 after import, got %v", afterImport.ID)
			}

			foundWarning := false
			for _, d := range importResp.Diagnostics {
				if d.Severity() == diag.SeverityWarning && d.Summary() == "org_group_id Not Set by Import" {
					foundWarning = true
				}
			}
			if foundWarning != tc.wantWarning {
				t.Fatalf("expected warning=%v after ImportState, got %v", tc.wantWarning, foundWarning)
			}

			readReq := resource.ReadRequest{State: importResp.State}
			readResp := &resource.ReadResponse{State: importResp.State}
			r.Read(context.Background(), readReq, readResp)
			if readResp.Diagnostics.HasError() {
				t.Fatalf("unexpected error from Read: %v", readResp.Diagnostics)
			}

			var afterRead tf.MacApplicationResourceModel
			if diags := readResp.State.Get(context.Background(), &afterRead); diags.HasError() {
				t.Fatalf("unexpected error reading state after Read: %v", diags)
			}

			if tc.wantOrgGroup {
				if afterRead.OrgGroupID.IsNull() || afterRead.OrgGroupID.ValueInt32() != tc.wantOrgGroupV {
					t.Fatalf("expected org_group_id %d to survive Read, got %v", tc.wantOrgGroupV, afterRead.OrgGroupID)
				}
			} else {
				if !afterRead.OrgGroupID.IsNull() {
					t.Fatalf("expected org_group_id to remain null after Read, got %v", afterRead.OrgGroupID)
				}
			}
		})
	}
}

// TestImportState_StallError_ProducesRequiredDiagnosticMessage confirms B10's
// exact required diagnostic wording when the download stalls mid-transfer,
// using the app name learned via onInfo (not the raw UUID) and the byte
// counts carried by the typed *httpclient.StallError.
func TestImportState_StallError_ProducesRequiredDiagnosticMessage(t *testing.T) {
	fake := &fakeAppBlobDownloader{
		infoName: "exampleInstallerApp",
		err: &httpclient.StallError{
			BytesRead:   50 * 1024 * 1024,
			TotalBytes:  200 * 1024 * 1024,
			IdleTimeout: 60_000_000_000, // 60s, avoids importing "time" just for this literal
		},
	}
	r := &macapplicationResource{appBinaryStoragePath: t.TempDir(), blobDownloader: fake}

	req := resource.ImportStateRequest{ID: "9af645a8-fef3-3e6d-3408-5cc69e0937d4"}
	resp := newImportStateResponse()
	r.ImportState(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostics error for a stalled download")
	}
	if !fake.onInfoCalled {
		t.Fatal("expected onInfo to have been invoked before the stall was returned")
	}

	const want = "download of exampleInstallerApp stalled after 50.0 of 200.0 MB; check the network and re-run onboard"
	found := false
	for _, d := range resp.Diagnostics {
		if strings.Contains(d.Detail(), want) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a diagnostic detail containing %q, got: %v", want, resp.Diagnostics)
	}
}

// TestImportState_StallError_UnknownTotal_OmitsOfMPart confirms the "after N
// MB" (no "of M MB") variant of the message when the stall error carries no
// known total (TotalBytes < 0).
func TestImportState_StallError_UnknownTotal_OmitsOfMPart(t *testing.T) {
	fake := &fakeAppBlobDownloader{
		infoName: "someApp",
		err: &httpclient.StallError{
			BytesRead:   1024 * 1024,
			TotalBytes:  -1,
			IdleTimeout: 60_000_000_000,
		},
	}
	r := &macapplicationResource{appBinaryStoragePath: t.TempDir(), blobDownloader: fake}

	req := resource.ImportStateRequest{ID: "9af645a8-fef3-3e6d-3408-5cc69e0937d4"}
	resp := newImportStateResponse()
	r.ImportState(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostics error for a stalled download")
	}
	const want = "download of someApp stalled after 1.0 MB; check the network and re-run onboard"
	found := false
	for _, d := range resp.Diagnostics {
		if strings.Contains(d.Detail(), want) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a diagnostic detail containing %q, got: %v", want, resp.Diagnostics)
	}
}

// TestImportState_NonStallDownloadError_UsesGenericClientErrorPath confirms
// an ordinary (non-stall) download error still goes through the pre-existing
// "Client Error" diagnostic, not the stall-specific one.
func TestImportState_NonStallDownloadError_UsesGenericClientErrorPath(t *testing.T) {
	fake := &fakeAppBlobDownloader{err: fmt.Errorf("boom")}
	r := &macapplicationResource{appBinaryStoragePath: t.TempDir(), blobDownloader: fake}

	req := resource.ImportStateRequest{ID: "9af645a8-fef3-3e6d-3408-5cc69e0937d4"}
	resp := newImportStateResponse()
	r.ImportState(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostics error")
	}
	found := false
	for _, d := range resp.Diagnostics {
		if d.Summary() == "Client Error" {
			found = true
		}
		if d.Summary() == "Download Stalled" {
			t.Fatalf("expected a plain download error not to use the stall-specific diagnostic, got: %v", d)
		}
	}
	if !found {
		t.Fatal("expected the generic 'Client Error' diagnostic summary")
	}
}

func intPtr(v int) *int {
	return &v
}
