package macapplication

import (
	"context"
	"path/filepath"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac/models"
)

// createThroughFramework plans and applies a create (null prior state) via
// the framework server and returns the new state.
func createThroughFramework(t *testing.T, plistBytes []byte) map[string]tftypes.Value {
	t.Helper()
	dir := t.TempDir()
	dmgPath := filepath.Join(dir, "app.dmg")
	plistPath := filepath.Join(dir, "app.plist")
	writeFixtureFile(t, dmgPath, pathIdentityDMG)
	writeFixtureFile(t, plistPath, plistBytes)

	server, counts := newPlanHarnessServer(t)
	prior := tftypes.NewValue(macAppObjectType(t), nil)
	config := macAppObject(t, map[string]tftypes.Value{
		"org_group_id":    tftypes.NewValue(tftypes.Number, 10),
		"dmg_file_path":   tfStr(dmgPath),
		"plist_file_path": tfStr(plistPath),
		"app_version":     tfStr("1.0.0"),
	})
	planResp, err := server.PlanResourceChange(context.Background(), &tfprotov6.PlanResourceChangeRequest{
		TypeName:         "uem_mac_application",
		PriorState:       dynamicValue(t, prior),
		ProposedNewState: dynamicValue(t, config),
		Config:           dynamicValue(t, config),
	})
	if err != nil {
		t.Fatalf("PlanResourceChange: %v", err)
	}
	failOnErrorDiags(t, "PlanResourceChange", planResp.Diagnostics)

	newState := runApply(t, server, prior, config, planOutcome{resp: planResp})
	if counts.uploads != 2 || counts.creates != 1 {
		t.Fatalf("expected a real create (2 uploads, 1 create), got %s", counts)
	}
	return newState
}

func TestCreate_SetsPlistFileSHA256FromCanonicalUploadedPlist(t *testing.T) {
	newState := createThroughFramework(t, []byte(basePlist))
	got, ok := strAttr(t, newState, "plist_file_sha256")
	if !ok || got != mustCanonicalSHA(t, basePlist) {
		t.Fatalf("plist_file_sha256 after create = %q (known=%v), want canonical sha %q", got, ok, mustCanonicalSHA(t, basePlist))
	}
	// Canonical, not raw: differs from the plain byte sha of the file.
	if got == sha256Hex([]byte(basePlist)) {
		t.Fatal("plist_file_sha256 must be the canonical-form sha, not the raw file sha")
	}
}

func TestCreate_BinaryPlist_LeavesPlistFileSHA256Null(t *testing.T) {
	newState := createThroughFramework(t, []byte("bplist00\xd1\x01\x02Q"))
	if !newState["plist_file_sha256"].IsNull() {
		t.Fatalf("expected null plist_file_sha256 for a binary plist, got %v", newState["plist_file_sha256"])
	}
}

func TestImportState_SetsPlistFileSHA256FromPkginfo_ReadKeepsIt(t *testing.T) {
	blobFake := &fakeAppBlobDownloader{blob: []byte("fake-dmg"), id: 42, pkginfo: keyOrderVariant}
	readFake := &fakeInternalAppsV1Service{model: &sdk.InternalAppModelV1{
		ID: intPtr(42), UUID: appVersionTestUUID,
		// Read must ignore this pkginfo for plist_file_sha256.
		MacOsSoftwareDeploymentSummary: &sdk.MacOsSoftwareDeploymentSummaryModelV1{Pkginfo: "<plist><string>other</string></plist>"},
	}}
	r := &macapplicationResource{
		appBinaryStoragePath: t.TempDir(),
		blobDownloader:       blobFake,
		client:               &sdk.Client{},
		newInternalAppResourceService: func(*sdk.Client) InternalAppsV1ServiceAPI {
			return readFake
		},
	}

	importResp := newImportStateResponse()
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: appVersionTestUUID}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", importResp.Diagnostics)
	}
	var afterImport tf.MacApplicationResourceModel
	if diags := importResp.State.Get(context.Background(), &afterImport); diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	want := mustCanonicalSHA(t, basePlist) // keyOrderVariant canonicalizes identically
	if afterImport.PlistFileSHA256.ValueString() != want {
		t.Fatalf("plist_file_sha256 after import = %v, want %q", afterImport.PlistFileSHA256, want)
	}

	readResp := &resource.ReadResponse{State: importResp.State}
	r.Read(context.Background(), resource.ReadRequest{State: importResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics)
	}
	var afterRead tf.MacApplicationResourceModel
	if diags := readResp.State.Get(context.Background(), &afterRead); diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	if afterRead.PlistFileSHA256.ValueString() != want {
		t.Fatalf("Read changed plist_file_sha256: got %v, want %q", afterRead.PlistFileSHA256, want)
	}
}

func TestImportState_NonXMLPkginfo_LeavesPlistFileSHA256Null(t *testing.T) {
	r := &macapplicationResource{
		appBinaryStoragePath: t.TempDir(),
		blobDownloader:       &fakeAppBlobDownloader{blob: []byte("fake-dmg"), id: 42, pkginfo: "bplist00\xd1"},
	}
	importResp := newImportStateResponse()
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: appVersionTestUUID}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", importResp.Diagnostics)
	}
	var got types.String
	if diags := importResp.State.GetAttribute(context.Background(), path.Root("plist_file_sha256"), &got); diags.HasError() {
		t.Fatalf("GetAttribute: %v", diags)
	}
	if !got.IsNull() {
		t.Fatalf("expected null plist_file_sha256 for non-XML pkginfo, got %v", got)
	}
}

// Read never writes plist_file_sha256, whatever the prior value.
func TestRead_NeverWritesPlistFileSHA256(t *testing.T) {
	for _, prior := range []types.String{types.StringNull(), types.StringValue("recorded-sha")} {
		r := newAppVersionTestResource(t, &sdk.InternalAppModelV1{
			ID: intPtr(42), UUID: appVersionTestUUID,
			MacOsSoftwareDeploymentSummary: &sdk.MacOsSoftwareDeploymentSummaryModelV1{Pkginfo: basePlist},
		})
		state := emptyMacApplicationState()
		var m tf.MacApplicationResourceModel
		if diags := state.Get(context.Background(), &m); diags.HasError() {
			t.Fatalf("state get: %v", diags)
		}
		m.ID = typesInt32(42)
		m.UUID = typesString(appVersionTestUUID)
		m.PlistFileSHA256 = prior
		m = withTypedRecordNulls(m)
		if diags := state.Set(context.Background(), &m); diags.HasError() {
			t.Fatalf("state set: %v", diags)
		}
		readResp := &resource.ReadResponse{State: state}
		r.Read(context.Background(), resource.ReadRequest{State: state}, readResp)
		if readResp.Diagnostics.HasError() {
			t.Fatalf("Read: %v", readResp.Diagnostics)
		}
		var after tf.MacApplicationResourceModel
		if diags := readResp.State.Get(context.Background(), &after); diags.HasError() {
			t.Fatalf("state get: %v", diags)
		}
		if !after.PlistFileSHA256.Equal(prior) {
			t.Fatalf("Read changed plist_file_sha256 from %v to %v", prior, after.PlistFileSHA256)
		}
	}
}
