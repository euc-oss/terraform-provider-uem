package macapplication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac/models"
)

// iconFakeDownloader adds appIconDownloader to the existing fake.
type iconFakeDownloader struct {
	*fakeAppBlobDownloader
	icon        []byte
	uemFileName string
}

func (f *iconFakeDownloader) DownloadAppIcon(context.Context, string) ([]byte, string, error) {
	return f.icon, f.uemFileName, nil
}

const f9UUID = "9af645a8-fef3-3e6d-3408-5cc69e0937d4"

// B32/F9: import writes the pkginfo to <uuid>/app.plist and the icon to
// <uuid>/icon, next to the binary, and points plist_file_path /
// icon_file_path (and icon_file_sha256) at them.
func TestImportState_WritesPlistAndIcon(t *testing.T) {
	tmp := t.TempDir()
	pkginfo := "<?xml version=\"1.0\"?><plist><dict><key>name</key><string>Escrow.Buddy</string></dict></plist>"
	icon := []byte("\x89PNG fake icon bytes")
	r := &macapplicationResource{
		appBinaryStoragePath: tmp,
		blobDownloader:       &iconFakeDownloader{fakeAppBlobDownloader: &fakeAppBlobDownloader{blob: []byte("dmg"), id: 42, pkginfo: pkginfo}, icon: icon},
	}
	resp := newImportStateResponse()
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: f9UUID}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import: %v", resp.Diagnostics)
	}
	dir := filepath.Join(tmp, f9UUID)
	for file, want := range map[string]string{importedPlistFileName: pkginfo, importedIconFileName: string(icon)} {
		got, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil || string(got) != want {
			t.Errorf("%s: got %q, err %v; want %q", file, got, err, want)
		}
	}
	var plistPath, iconPath, iconSHA types.String
	resp.State.GetAttribute(context.Background(), path.Root("plist_file_path"), &plistPath)
	resp.State.GetAttribute(context.Background(), path.Root("icon_file_path"), &iconPath)
	resp.State.GetAttribute(context.Background(), path.Root("icon_file_sha256"), &iconSHA)
	sum := sha256.Sum256(icon)
	if plistPath.ValueString() != filepath.Join(dir, "app.plist") {
		t.Errorf("plist_file_path = %q", plistPath.ValueString())
	}
	if iconPath.ValueString() != filepath.Join(dir, "icon") {
		t.Errorf("icon_file_path = %q", iconPath.ValueString())
	}
	if iconSHA.ValueString() != hex.EncodeToString(sum[:]) {
		t.Errorf("icon_file_sha256 = %q", iconSHA.ValueString())
	}
}

// TestImportState_IconWithPNGBytes_WritesIconWithPNGExtension covers B47: a
// real PNG-magic-bytes icon is imported as icon.png, not the extension-less
// "icon" (which UEM later refuses to re-type on create — "Invalid blob
// type"), and icon_file_path in state ends in icon.png. Revert check: if
// writeImportedIcon/iconFileName is reverted to always return
// importedIconFileName ("icon", hardcoded), this test fails because the
// written file and icon_file_path both end in "icon", not "icon.png".
func TestImportState_IconWithPNGBytes_WritesIconWithPNGExtension(t *testing.T) {
	tmp := t.TempDir()
	icon := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 'r', 'e', 's', 't'}
	r := &macapplicationResource{
		appBinaryStoragePath: tmp,
		blobDownloader:       &iconFakeDownloader{fakeAppBlobDownloader: &fakeAppBlobDownloader{blob: []byte("dmg"), id: 42}, icon: icon},
	}
	resp := newImportStateResponse()
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: f9UUID}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import: %v", resp.Diagnostics)
	}
	dir := filepath.Join(tmp, f9UUID)
	got, err := os.ReadFile(filepath.Join(dir, "icon.png"))
	if err != nil || string(got) != string(icon) {
		t.Fatalf("icon.png: got %q, err %v; want %q", got, err, icon)
	}
	if _, err := os.Stat(filepath.Join(dir, importedIconFileName)); !os.IsNotExist(err) {
		t.Fatalf("the extension-less %q must not also be written", importedIconFileName)
	}

	var iconPath types.String
	resp.State.GetAttribute(context.Background(), path.Root("icon_file_path"), &iconPath)
	if !strings.HasSuffix(iconPath.ValueString(), "icon.png") {
		t.Fatalf("icon_file_path = %q, want it to end in icon.png", iconPath.ValueString())
	}
}

// TestImportState_IconUEMFileName_PreferredOverSniffing covers B47's
// preferred path: when UEM's blob download reports the icon's own filename
// (Content-Disposition), that extension is used even for bytes this
// provider's own sniffing wouldn't otherwise recognize as an image.
func TestImportState_IconUEMFileName_PreferredOverSniffing(t *testing.T) {
	tmp := t.TempDir()
	icon := []byte("not a recognized image format")
	r := &macapplicationResource{
		appBinaryStoragePath: tmp,
		blobDownloader: &iconFakeDownloader{
			fakeAppBlobDownloader: &fakeAppBlobDownloader{blob: []byte("dmg"), id: 42},
			icon:                  icon,
			uemFileName:           "AppIcon.icns",
		},
	}
	resp := newImportStateResponse()
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: f9UUID}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import: %v", resp.Diagnostics)
	}
	dir := filepath.Join(tmp, f9UUID)
	if _, err := os.Stat(filepath.Join(dir, "icon.icns")); err != nil {
		t.Fatalf("expected icon.icns to be written: %v", err)
	}
}

// An application without an icon (or a downloader that can't fetch icons)
// leaves icon_file_path and icon_file_sha256 null.
func TestImportState_NoIconLeavesIconNull(t *testing.T) {
	for name, dl := range map[string]appBlobDownloader{
		"no icon":             &iconFakeDownloader{fakeAppBlobDownloader: &fakeAppBlobDownloader{blob: []byte("dmg"), id: 42}},
		"downloader w/o icon": &fakeAppBlobDownloader{blob: []byte("dmg"), id: 42},
	} {
		tmp := t.TempDir()
		resp := newImportStateResponse()
		(&macapplicationResource{appBinaryStoragePath: tmp, blobDownloader: dl}).ImportState(context.Background(), resource.ImportStateRequest{ID: f9UUID}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("%s: %v", name, resp.Diagnostics)
		}
		var iconPath types.String
		resp.State.GetAttribute(context.Background(), path.Root("icon_file_path"), &iconPath)
		if !iconPath.IsNull() {
			t.Errorf("%s: icon_file_path = %q, want null", name, iconPath.ValueString())
		}
		if _, err := os.Stat(filepath.Join(tmp, f9UUID, importedIconFileName)); !os.IsNotExist(err) {
			t.Errorf("%s: no icon file may be written", name)
		}
	}
}

func intp(i int) *int { return &i }

// sampleRecord mirrors the shape of a live macOS internal app read
// (placeholder values).
func sampleRecord() *sdk.InternalAppModelV1 {
	return &sdk.InternalAppModelV1{
		ID:                                 intp(153),
		UUID:                               f9UUID,
		ActualFileVersion:                  "1.0.0",
		AirwatchAppVersion:                 "1.0.0.0",
		AppID:                              "com.ws1.macos.Example",
		AppSizeInKB:                        intp(81),
		ApplicationName:                    "Example",
		ApplicationURL:                     "",
		AssumeManagementOfUserInstalledApp: "No",
		CategoryList:                       []sdk.ApplicationCategoriesModelV1{{Name: "Utilities", ID: intp(7), UUID: "bafde89c-041e-1756-082b-933aaf16cad8"}},
		MacOsSoftwareDeploymentSummary:     &sdk.MacOsSoftwareDeploymentSummaryModelV1{IsManaged: "True", Pkginfo: "<plist/>"},
		ManagedBy:                          "12345",
		Platform:                           "AppleOsX",
		Rating:                             intp(0),
		Sdk:                                "Disabled",
		Status:                             "Active",
	}
}

// Every record field is passed through exactly: "" stays "", a missing
// number is null, lists and the deployment summary map field-for-field.
func TestSetRecordFromV1_PassesThroughExactly(t *testing.T) {
	m := withTypedRecordNulls(tf.MacApplicationResourceModel{})
	if diags := setRecordFromV1(&m, sampleRecord()); diags.HasError() {
		t.Fatalf("setRecordFromV1: %v", diags)
	}
	checks := map[string][2]string{
		"application_name": {m.ApplicationName.ValueString(), "Example"},
		"app_id":           {m.AppIDValue.ValueString(), "com.ws1.macos.Example"},
		"application_url":  {m.ApplicationURL.ValueString(), ""},
		"managed_by":       {m.ManagedBy.ValueString(), "12345"},
		"status":           {m.Status.ValueString(), "Active"},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
	if m.ApplicationURL.IsNull() {
		t.Error(`an empty string from UEM must stay "", not null`)
	}
	if m.AppSizeInKB.ValueInt64() != 81 || !m.SdkProfileID.IsNull() {
		t.Errorf("ints: app_size_in_kb=%v sdk_profile_id=%v (want 81 and null)", m.AppSizeInKB, m.SdkProfileID)
	}
	if len(m.CategoryList.Elements()) != 1 || len(m.SupportedModels.Elements()) != 0 || m.SupportedModels.IsNull() {
		t.Errorf("lists: category_list=%v supported_models=%v", m.CategoryList, m.SupportedModels)
	}
	if got := m.MacOsSoftwareDeploymentSummary.Attributes()["pkginfo"]; got.String() != `"<plist/>"` {
		t.Errorf("mac_os_software_deployment_summary.pkginfo = %s", got)
	}
}

// refreshIntoState (the shared Create/Read path) fills the record.
func TestRefreshIntoState_FillsRecord(t *testing.T) {
	state := emptyMacApplicationState()
	data := withTypedRecordNulls(tf.MacApplicationResourceModel{})
	var diags diag.Diagnostics
	(&macapplicationResource{}).refreshIntoState(context.Background(), 153, sampleRecord(), &data, &state, &diags, nil)
	if diags.HasError() {
		t.Fatalf("refreshIntoState: %v", diags)
	}
	var name types.String
	state.GetAttribute(context.Background(), path.Root("application_name"), &name)
	if name.ValueString() != "Example" {
		t.Errorf("application_name in state = %q, want Example", name.ValueString())
	}
}

func TestCreateMacOSApplicationRequest_CarriesIconID(t *testing.T) {
	if got := CreateMacOSApplicationRequest(1, 2, intp(3), "1.0").ApplicationIconID; got == nil || *got != 3 {
		t.Errorf("ApplicationIconID = %v, want 3", got)
	}
	if got := CreateMacOSApplicationRequest(1, 2, nil, "1.0").ApplicationIconID; got != nil {
		t.Errorf("no icon must omit ApplicationIconID, got %v", *got)
	}
}

// icon_file_path is a create-only input: it carries the content-identity
// replace rule, like dmg_file_path.
func TestIconFilePath_RequiresReplaceOnContentChange(t *testing.T) {
	var sr resource.SchemaResponse
	(&macapplicationResource{}).Schema(context.Background(), resource.SchemaRequest{}, &sr)
	a, ok := sr.Schema.Attributes["icon_file_path"].(schema.StringAttribute)
	if !ok || !a.Optional || !a.Computed {
		t.Fatalf("icon_file_path must be an Optional+Computed string: %#v", sr.Schema.Attributes["icon_file_path"])
	}
	want := iconFilePathRequiresReplace().Description(context.Background())
	found := false
	for _, pm := range a.PlanModifiers {
		if pm.Description(context.Background()) == want {
			found = true
		}
	}
	if !found {
		t.Error("icon_file_path is missing its RequiresReplace-on-content-change modifier")
	}
	for _, name := range []string{"application_name", "app_id", "managed_by", "mac_os_software_deployment_summary"} {
		if _, ok := sr.Schema.Attributes[name]; !ok {
			t.Errorf("record attribute %s missing from schema", name)
		}
	}
}

// icon_file_path keeps a Clean-equivalent configured path as the state value
// (like dmg/plist), before its replace rule, so onboard's
// "${path.root}/../../x" rendering plans no diff against a recorded
// "../../x".
func TestIconFilePath_CleanEquivalentPathBeforeReplace(t *testing.T) {
	var sr resource.SchemaResponse
	(&macapplicationResource{}).Schema(context.Background(), resource.SchemaRequest{}, &sr)
	a, ok := sr.Schema.Attributes["icon_file_path"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("icon_file_path must be a string attribute: %#v", sr.Schema.Attributes["icon_file_path"])
	}
	clean := cleanEquivalentPathUseState{}.Description(context.Background())
	replace := iconFilePathRequiresReplace().Description(context.Background())
	cleanAt, replaceAt := -1, -1
	for i, pm := range a.PlanModifiers {
		switch pm.Description(context.Background()) {
		case clean:
			cleanAt = i
		case replace:
			replaceAt = i
		}
	}
	if cleanAt < 0 || replaceAt < 0 || cleanAt > replaceAt {
		t.Fatalf("icon_file_path must run the Clean-equivalent path rule before its replace rule: clean=%d replace=%d", cleanAt, replaceAt)
	}

	var resp planmodifier.StringResponse
	req := planmodifier.StringRequest{PlanValue: types.StringValue("./../../a/icon.png"), StateValue: types.StringValue("../../a/icon.png")}
	resp.PlanValue = req.PlanValue
	cleanEquivalentPathUseState{}.PlanModifyString(context.Background(), req, &resp)
	if resp.PlanValue.ValueString() != "../../a/icon.png" {
		t.Fatalf("planned %q, want the state value kept", resp.PlanValue.ValueString())
	}
}
