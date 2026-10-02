package macapplication

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac/models"
)

const appVersionTestUUID = "9af645a8-fef3-3e6d-3408-5cc69e0937d4"

func newAppVersionTestResource(t *testing.T, model *sdk.InternalAppModelV1) *macapplicationResource {
	t.Helper()
	readFake := &fakeInternalAppsV1Service{model: model}
	return &macapplicationResource{
		appBinaryStoragePath: t.TempDir(),
		blobDownloader:       &fakeAppBlobDownloader{blob: []byte("fake-dmg-binary-content"), id: 42},
		client:               &sdk.Client{},
		newInternalAppResourceService: func(*sdk.Client) InternalAppsV1ServiceAPI {
			return readFake
		},
	}
}

// TestImportState_ThenRead_PopulatesAppVersion: import leaves app_version
// null; the Read Terraform runs right after import must recover it from the
// API's AirwatchAppVersion.
func TestImportState_ThenRead_PopulatesAppVersion(t *testing.T) {
	r := newAppVersionTestResource(t, &sdk.InternalAppModelV1{
		ID: intPtr(42), UUID: appVersionTestUUID,
		AirwatchAppVersion: "2.6.22.0", ActualFileVersion: "2.6",
	})

	importResp := newImportStateResponse()
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: appVersionTestUUID}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error from ImportState: %v", importResp.Diagnostics)
	}

	readResp := &resource.ReadResponse{State: importResp.State}
	r.Read(context.Background(), resource.ReadRequest{State: importResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error from Read: %v", readResp.Diagnostics)
	}

	var afterRead tf.MacApplicationResourceModel
	if diags := readResp.State.Get(context.Background(), &afterRead); diags.HasError() {
		t.Fatalf("unexpected error reading state after Read: %v", diags)
	}
	if afterRead.AppVersion.IsNull() || afterRead.AppVersion.ValueString() != "2.6.22.0" {
		t.Fatalf("expected app_version %q after import+Read, got %v", "2.6.22.0", afterRead.AppVersion)
	}
}

// TestRead_EmptyAPIAppVersionKeepsPrior: an empty AirwatchAppVersion must not
// wipe a known prior app_version.
func TestRead_EmptyAPIAppVersionKeepsPrior(t *testing.T) {
	r := newAppVersionTestResource(t, &sdk.InternalAppModelV1{ID: intPtr(42), UUID: appVersionTestUUID})

	state := emptyMacApplicationState()
	prior := tf.MacApplicationResourceModel{}
	if diags := state.Get(context.Background(), &prior); diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	prior.ID = typesInt32(42)
	prior.UUID = typesString(appVersionTestUUID)
	prior.AppVersion = tf.NewAppVersionValue("2.6.22")
	prior = withTypedRecordNulls(prior)
	if diags := state.Set(context.Background(), &prior); diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}

	readResp := &resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error from Read: %v", readResp.Diagnostics)
	}
	var after tf.MacApplicationResourceModel
	if diags := readResp.State.Get(context.Background(), &after); diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if after.AppVersion.ValueString() != "2.6.22" {
		t.Fatalf("expected prior app_version to survive an empty API value, got %v", after.AppVersion)
	}
}

// appVersionTestProvider is a minimal framework provider exposing only the
// pre-built mac application resource, so ReadResource can be driven through
// the real framework server (providerserver -> fwserver), including its
// semantic-equality pass.
type appVersionTestProvider struct {
	r *macapplicationResource
}

func (p *appVersionTestProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "uem"
}

func (p *appVersionTestProvider) Schema(context.Context, provider.SchemaRequest, *provider.SchemaResponse) {
}

func (p *appVersionTestProvider) Configure(context.Context, provider.ConfigureRequest, *provider.ConfigureResponse) {
}

func (p *appVersionTestProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{func() resource.Resource { return p.r }}
}

func (p *appVersionTestProvider) DataSources(context.Context) []func() datasource.DataSource {
	return nil
}

// TestReadResource_FrameworkServer_AppVersionSemanticEquality drives
// ReadResource through providerserver.NewProtocol6: with prior state
// app_version "2.6.22" and the API reporting "2.6.22.0", the framework's
// semantic-equality pass must keep the prior value (no diff), while a
// genuinely different API version ("2.6.23.0") must come through.
func TestReadResource_FrameworkServer_AppVersionSemanticEquality(t *testing.T) {
	testCases := []struct {
		name       string
		apiVersion string
		want       string
	}{
		{name: "padded same version keeps prior", apiVersion: "2.6.22.0", want: "2.6.22"},
		{name: "different version takes api value", apiVersion: "2.6.23.0", want: "2.6.23.0"},
		{name: "empty api version keeps prior", apiVersion: "", want: "2.6.22"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r := newAppVersionTestResource(t, &sdk.InternalAppModelV1{
				ID: intPtr(42), UUID: appVersionTestUUID, AirwatchAppVersion: tc.apiVersion,
			})
			server, err := providerserver.NewProtocol6WithError(&appVersionTestProvider{r: r})()
			if err != nil {
				t.Fatalf("NewProtocol6WithError: %v", err)
			}

			objType, ok := emptyMacApplicationState().Raw.Type().(tftypes.Object)
			if !ok {
				t.Fatal("schema type is not an Object")
			}
			values := make(map[string]tftypes.Value, len(objType.AttributeTypes))
			for name, attrType := range objType.AttributeTypes {
				values[name] = tftypes.NewValue(attrType, nil)
			}
			values["id"] = tftypes.NewValue(tftypes.Number, 42)
			values["uuid"] = tftypes.NewValue(tftypes.String, appVersionTestUUID)
			values["app_version"] = tftypes.NewValue(tftypes.String, "2.6.22")
			current, err := tfprotov6.NewDynamicValue(objType, tftypes.NewValue(objType, values))
			if err != nil {
				t.Fatalf("NewDynamicValue: %v", err)
			}

			resp, err := server.ReadResource(ctx, &tfprotov6.ReadResourceRequest{
				TypeName:     "uem_mac_application",
				CurrentState: &current,
			})
			if err != nil {
				t.Fatalf("ReadResource: %v", err)
			}
			for _, d := range resp.Diagnostics {
				if d.Severity == tfprotov6.DiagnosticSeverityError {
					t.Fatalf("ReadResource error diagnostic: %s: %s", d.Summary, d.Detail)
				}
			}

			newState, err := resp.NewState.Unmarshal(objType)
			if err != nil {
				t.Fatalf("unmarshal NewState: %v", err)
			}
			var attrs map[string]tftypes.Value
			if err := newState.As(&attrs); err != nil {
				t.Fatalf("NewState.As: %v", err)
			}
			var got string
			if err := attrs["app_version"].As(&got); err != nil {
				t.Fatalf("app_version.As: %v", err)
			}
			if got != tc.want {
				t.Fatalf("app_version after framework ReadResource = %q, want %q", got, tc.want)
			}
		})
	}
}
