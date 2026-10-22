package macapplication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/euc-oss/terraform-provider-uem/internal/httpclient"
)

// Plan/apply harness shared by the path-identity (resource_path_identity_test.go)
// and app_version (resource_appversion_plan_test.go) plan-level tests. It drives
// PlanResourceChange / ApplyResourceChange through the real framework server
// (providerserver -> fwserver), with every SDK seam replaced by a counting fake.

// apiCallCounts counts every SDK call the resource can make.
type apiCallCounts struct {
	mu          sync.Mutex
	uploads     int
	creates     int
	blobDeletes int
	blobGets    int
	appGets     int
	appDeletes  int
	downloads   int
}

func (c *apiCallCounts) inc(p *int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	*p++
}

func (c *apiCallCounts) total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.uploads + c.creates + c.blobDeletes + c.blobGets + c.appGets + c.appDeletes + c.downloads
}

func (c *apiCallCounts) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return fmt.Sprintf("uploads=%d creates=%d blobDeletes=%d blobGets=%d appGets=%d appDeletes=%d downloads=%d",
		c.uploads, c.creates, c.blobDeletes, c.blobGets, c.appGets, c.appDeletes, c.downloads)
}

type countingBlobV1 struct{ c *apiCallCounts }

func (f countingBlobV1) UploadBlobAsync(context.Context, []byte, *sdk.BlobsV1UploadBlobAsyncOptions) (http.Header, *sdk.EntityV1Model, error) {
	f.c.inc(&f.c.uploads)
	id := 1
	return nil, &sdk.EntityV1Model{Value: &id, UUID: "blob-uuid-1"}, nil
}

type countingBlobV2 struct{ c *apiCallCounts }

func (f countingBlobV2) Delete(context.Context, string) (http.Header, error) {
	f.c.inc(&f.c.blobDeletes)
	return nil, nil
}

func (f countingBlobV2) Get(context.Context, string) (http.Header, []byte, error) {
	f.c.inc(&f.c.blobGets)
	return nil, nil, nil
}

type countingMacApp struct{ c *apiCallCounts }

func (f countingMacApp) CreateMacOSApplication(context.Context, int, *sdk.MacOsCreateApplicationRequestV1Model) (http.Header, error) {
	f.c.inc(&f.c.creates)
	h := http.Header{}
	h.Set("Location", "/API/mam/apps/internal/42")
	return h, nil
}

type countingInternalApp struct {
	c          *apiCallCounts
	apiVersion string // AirwatchAppVersion the fake GET echoes
}

func (f countingInternalApp) DeleteInternalAppAsync(context.Context, int) (http.Header, error) {
	f.c.inc(&f.c.appDeletes)
	return nil, nil
}

func (f countingInternalApp) GetInternalAppByIdAsync(context.Context, int) (http.Header, *sdk.InternalAppModelV1, error) {
	f.c.inc(&f.c.appGets)
	return nil, &sdk.InternalAppModelV1{ID: intPtr(42), UUID: appVersionTestUUID, AirwatchAppVersion: f.apiVersion}, nil
}

type countingDownloader struct{ c *apiCallCounts }

func (f countingDownloader) DownloadAppBlob(context.Context, string, appInfoFunc, httpclient.ProgressFunc) (int, []byte, string, error) {
	f.c.inc(&f.c.downloads)
	return 42, nil, "", nil
}

// newPlanHarnessServer returns a protocol-6 server exposing a mac application
// resource whose every SDK seam is a counting fake.
func newPlanHarnessServer(t *testing.T) (tfprotov6.ProviderServer, *apiCallCounts) {
	t.Helper()
	return newPlanHarnessServerWithAPIVersion(t, "")
}

// newPlanHarnessServerWithAPIVersion is newPlanHarnessServer whose fake GET
// reports apiVersion as AirwatchAppVersion.
func newPlanHarnessServerWithAPIVersion(t *testing.T, apiVersion string) (tfprotov6.ProviderServer, *apiCallCounts) {
	t.Helper()
	counts := &apiCallCounts{}
	r := &macapplicationResource{
		client:                        &sdk.Client{},
		appBinaryStoragePath:          t.TempDir(),
		blobDownloader:                countingDownloader{counts},
		newBlobV1ResourceService:      func(*sdk.Client) appBlobV1ResourceServiceAPI { return countingBlobV1{counts} },
		newBlobV2ResourceService:      func(*sdk.Client) appBlobV2ResourceServiceAPI { return countingBlobV2{counts} },
		newMacAppResourceService:      func(*sdk.Client) macAppResourceServiceAPI { return countingMacApp{counts} },
		newInternalAppResourceService: func(*sdk.Client) InternalAppsV1ServiceAPI { return countingInternalApp{counts, apiVersion} },
	}
	server, err := providerserver.NewProtocol6WithError(&appVersionTestProvider{r: r})()
	if err != nil {
		t.Fatalf("NewProtocol6WithError: %v", err)
	}
	return server, counts
}

func macAppObjectType(t *testing.T) tftypes.Object {
	t.Helper()
	objType, ok := emptyMacApplicationState().Raw.Type().(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an Object")
	}
	return objType
}

// macAppObject builds a full resource object; attributes not in vals are null.
func macAppObject(t *testing.T, vals map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	objType := macAppObjectType(t)
	values := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, attrType := range objType.AttributeTypes {
		if v, ok := vals[name]; ok {
			values[name] = v
			continue
		}
		values[name] = tftypes.NewValue(attrType, nil)
	}
	return tftypes.NewValue(objType, values)
}

// proposedNewState mirrors Terraform core's ProposedNew for this flat schema:
// a non-null configured value wins; otherwise the prior value is carried
// (Optional+Computed and Computed-only attributes alike).
func proposedNewState(t *testing.T, prior, config tftypes.Value) tftypes.Value {
	t.Helper()
	if prior.IsNull() {
		// Create: nothing to carry over.
		return config
	}
	var priorAttrs, configAttrs map[string]tftypes.Value
	if err := prior.As(&priorAttrs); err != nil {
		t.Fatalf("prior.As: %v", err)
	}
	if err := config.As(&configAttrs); err != nil {
		t.Fatalf("config.As: %v", err)
	}
	out := make(map[string]tftypes.Value, len(priorAttrs))
	for name, pv := range priorAttrs {
		if cv := configAttrs[name]; !cv.IsNull() {
			out[name] = cv
		} else {
			out[name] = pv
		}
	}
	return tftypes.NewValue(prior.Type(), out)
}

func dynamicValue(t *testing.T, v tftypes.Value) *tfprotov6.DynamicValue {
	t.Helper()
	dv, err := tfprotov6.NewDynamicValue(v.Type(), v)
	if err != nil {
		t.Fatalf("NewDynamicValue: %v", err)
	}
	return &dv
}

func failOnErrorDiags(t *testing.T, what string, diags []*tfprotov6.Diagnostic) {
	t.Helper()
	for _, d := range diags {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("%s error diagnostic: %s: %s", what, d.Summary, d.Detail)
		}
	}
}

type planOutcome struct {
	resp    *tfprotov6.PlanResourceChangeResponse
	planned map[string]tftypes.Value
}

func (p planOutcome) requiresReplace(attr string) bool {
	want := tftypes.NewAttributePath().WithAttributeName(attr)
	for _, rp := range p.resp.RequiresReplace {
		if rp.Equal(want) {
			return true
		}
	}
	return false
}

func runPlan(t *testing.T, server tfprotov6.ProviderServer, prior, config tftypes.Value) planOutcome {
	t.Helper()
	resp, err := server.PlanResourceChange(context.Background(), &tfprotov6.PlanResourceChangeRequest{
		TypeName:         "uem_mac_application",
		PriorState:       dynamicValue(t, prior),
		ProposedNewState: dynamicValue(t, proposedNewState(t, prior, config)),
		Config:           dynamicValue(t, config),
	})
	if err != nil {
		t.Fatalf("PlanResourceChange: %v", err)
	}
	failOnErrorDiags(t, "PlanResourceChange", resp.Diagnostics)
	return planOutcome{resp: resp, planned: unmarshalAttrs(t, resp.PlannedState)}
}

func runApply(t *testing.T, server tfprotov6.ProviderServer, prior, config tftypes.Value, plan planOutcome) map[string]tftypes.Value {
	t.Helper()
	resp, err := server.ApplyResourceChange(context.Background(), &tfprotov6.ApplyResourceChangeRequest{
		TypeName:       "uem_mac_application",
		PriorState:     dynamicValue(t, prior),
		PlannedState:   plan.resp.PlannedState,
		Config:         dynamicValue(t, config),
		PlannedPrivate: plan.resp.PlannedPrivate,
	})
	if err != nil {
		t.Fatalf("ApplyResourceChange: %v", err)
	}
	failOnErrorDiags(t, "ApplyResourceChange", resp.Diagnostics)
	return unmarshalAttrs(t, resp.NewState)
}

func unmarshalAttrs(t *testing.T, dv *tfprotov6.DynamicValue) map[string]tftypes.Value {
	t.Helper()
	if dv == nil {
		t.Fatal("nil DynamicValue")
	}
	v, err := dv.Unmarshal(macAppObjectType(t))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	var attrs map[string]tftypes.Value
	if err := v.As(&attrs); err != nil {
		t.Fatalf("As: %v", err)
	}
	return attrs
}

// strAttr returns a string attribute's value; ok is false when null/unknown.
func strAttr(t *testing.T, attrs map[string]tftypes.Value, name string) (string, bool) {
	t.Helper()
	v := attrs[name]
	if v.IsNull() || !v.IsKnown() {
		return "", false
	}
	var s string
	if err := v.As(&s); err != nil {
		t.Fatalf("%s.As: %v", name, err)
	}
	return s, true
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func tfStr(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }
