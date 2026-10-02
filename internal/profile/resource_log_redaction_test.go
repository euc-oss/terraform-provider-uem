package profile

// zf1 part 2: secret leak via tflog.
//
// createTypedProfile, updateTypedProfile, and updateAppleOsXProfile used to
// json.Marshal the entire outbound profile entity and tflog.Debug the
// result under a "... payload" message. Typed profile entities carry
// secret-bearing sections (network/credentials passwords, macOS passcodes)
// as well as unmodeled General fields (General.Password) preserved
// verbatim from a prior read-modify-write Get — both would leak into
// TF_LOG=DEBUG output.
//
// These tests drive the real ProfileResource.Create/Update through the
// fakeProfileService double (defined in resource_update_rmw_test.go) with
// tflogtest.RootLogger capturing all emitted log entries into a buffer,
// and assert:
//
//   - two distinct secret canaries (one in a modeled, secret-bearing plan
//     field; one in an unmodeled live General.Password preserved by RMW)
//     never appear anywhere in the captured log output;
//   - the redacted summary line (message + a known section name) DID get
//     logged, so the test can't pass vacuously because logging silently
//     stopped happening.

import (
	"bytes"
	"context"
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflogtest"
)

// Distinctive secret canaries: canaryModeled is placed in a modeled,
// secret-bearing plan field (macOS network_list password); canaryGeneral
// is placed in the unmodeled General.Password of the live GET entity that
// read-modify-write preserves verbatim into the outbound Update entity.
const (
	canaryModeled = "zf1-SECRET-canary-9f3b"
	canaryGeneral = "zf1-SECRET-general-7c2a"
)

// assertNoCanaries fails the test if either secret canary appears anywhere
// in the captured log buffer.
func assertNoCanaries(t *testing.T, logOutput string) {
	t.Helper()
	if strings.Contains(logOutput, canaryModeled) {
		t.Errorf("log output must never contain the modeled-field secret canary %q; got: %s", canaryModeled, logOutput)
	}
	if strings.Contains(logOutput, canaryGeneral) {
		t.Errorf("log output must never contain the unmodeled General.Password secret canary %q; got: %s", canaryGeneral, logOutput)
	}
}

// findLogMessage returns the decoded log entry whose @message contains
// substr, or nil if none matched.
func findLogMessage(t *testing.T, entries []map[string]interface{}, substr string) map[string]interface{} {
	t.Helper()
	for _, e := range entries {
		if msg, _ := e["@message"].(string); strings.Contains(msg, substr) {
			return e
		}
	}
	return nil
}

// --- Update: macOS ---

func TestProfileResourceUpdate_AppleOsX_LogRedaction(t *testing.T) {
	t.Parallel()

	live := liveAppleOsXEntity()
	live.General.Password = canaryGeneral // unmodeled General field, preserved by RMW.

	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: live}}

	var logBuf bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &logBuf)

	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"id": stringVal("999"),
		// network_list is a modeled section for macOS Update (see overlay.go)
		// — the plan-supplied password therefore ends up in the entity
		// passed to svc.Update, and must not be logged.
		"network_list": networkListVal([]map[string]tftypes.Value{
			{
				"network_interface":      stringVal("BuiltInWireless"),
				"service_set_identifier": stringVal("Corp-WiFi"),
				"password":               stringVal(canaryModeled),
			},
		}),
	})

	res := newRMWTestResource(t, fake)
	plan := createResourcePlan(t, planValues)
	req := resource.UpdateRequest{Plan: plan, State: emptyResourceState(t)}
	resp := &resource.UpdateResponse{State: emptyResourceState(t)}
	res.Update(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	if fake.updateCalls != 1 {
		t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
	}
	sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
	if !ok {
		t.Fatalf("expected *sdk.AppleOsXDeviceProfileEntityV2, got %T", fake.updateEntity)
	}
	// Sanity: confirm the canary really did make it into the sent entity,
	// so a negative log assertion isn't trivially true because the canary
	// never reached the payload in the first place.
	if len(sent.NetworkList) != 1 || sent.NetworkList[0].Password != canaryModeled {
		t.Fatalf("expected the modeled canary to reach the sent entity's NetworkList, got %+v", sent.NetworkList)
	}
	if sent.General.Password != canaryGeneral {
		t.Fatalf("expected the unmodeled canary to survive RMW into General.Password, got %q", sent.General.Password)
	}

	assertNoCanaries(t, logBuf.String())

	entries, err := tflogtest.MultilineJSONDecode(&logBuf)
	if err != nil {
		t.Fatalf("failed to decode captured log output: %s\nraw: %s", err, logBuf.String())
	}
	found := findLogMessage(t, entries, "macOS Update payload")
	if found == nil {
		t.Fatalf("expected a 'macOS Update payload' log entry (positive control), got entries: %#v", entries)
	}
	sections, _ := found["sections"].([]interface{})
	if !containsString(sections, "General") {
		t.Errorf("expected redacted summary to list section %q, got: %#v", "General", sections)
	}
	if !containsString(sections, "NetworkList") {
		t.Errorf("expected redacted summary to list section %q, got: %#v", "NetworkList", sections)
	}
}

// --- Update: non-mac (Android) ---

func TestProfileResourceUpdate_Android_LogRedaction(t *testing.T) {
	t.Parallel()

	live := liveAndroidEntity()
	live.General.Password = canaryGeneral // unmodeled General field, preserved by RMW.

	fake := &fakeProfileService{getResult: &sdk.ProfileResult{Android: live}}

	var logBuf bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &logBuf)

	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAndroid), map[string]tftypes.Value{
		"id": stringVal("999"),
	})

	res := newRMWTestResource(t, fake)
	plan := createResourcePlan(t, planValues)
	req := resource.UpdateRequest{Plan: plan, State: emptyResourceState(t)}
	resp := &resource.UpdateResponse{State: emptyResourceState(t)}
	res.Update(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	if fake.updateCalls != 1 {
		t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
	}
	sent, ok := fake.updateEntity.(*sdk.AndroidDeviceProfileV2Entity)
	if !ok {
		t.Fatalf("expected *sdk.AndroidDeviceProfileV2Entity, got %T", fake.updateEntity)
	}
	if sent.General.Password != canaryGeneral {
		t.Fatalf("expected the unmodeled canary to survive RMW into General.Password, got %q", sent.General.Password)
	}

	assertNoCanaries(t, logBuf.String())

	entries, err := tflogtest.MultilineJSONDecode(&logBuf)
	if err != nil {
		t.Fatalf("failed to decode captured log output: %s\nraw: %s", err, logBuf.String())
	}
	found := findLogMessage(t, entries, "Update payload")
	if found == nil {
		t.Fatalf("expected an 'Update payload' log entry (positive control), got entries: %#v", entries)
	}
	sections, _ := found["sections"].([]interface{})
	if !containsString(sections, "General") {
		t.Errorf("expected redacted summary to list section %q, got: %#v", "General", sections)
	}
}

// --- Create: macOS ---

func TestProfileResourceCreate_AppleOsX_LogRedaction(t *testing.T) {
	t.Parallel()

	// getErr steers Create's post-write readback down the "Unable to read
	// back created profile" tflog.Warn branch instead of dereferencing a
	// nil *sdk.ProfileResult from the fake's zero-value Get result — this
	// test only cares about the Create payload log line, not readback.
	fake := &fakeProfileService{getErr: errGetFailed}

	var logBuf bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &logBuf)

	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"id": nullString(),
		// network_list carries the secret-bearing password field on Create.
		"network_list": networkListVal([]map[string]tftypes.Value{
			{
				"network_interface":      stringVal("BuiltInWireless"),
				"service_set_identifier": stringVal("Corp-WiFi"),
				"password":               stringVal(canaryModeled),
			},
		}),
	})

	res := newRMWTestResource(t, fake)
	plan := createResourcePlan(t, planValues)
	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: emptyResourceState(t)}
	res.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}

	assertNoCanaries(t, logBuf.String())

	entries, err := tflogtest.MultilineJSONDecode(&logBuf)
	if err != nil {
		t.Fatalf("failed to decode captured log output: %s\nraw: %s", err, logBuf.String())
	}
	found := findLogMessage(t, entries, "Create payload")
	if found == nil {
		t.Fatalf("expected a 'Create payload' log entry (positive control), got entries: %#v", entries)
	}
	sections, _ := found["sections"].([]interface{})
	if !containsString(sections, "General") {
		t.Errorf("expected redacted summary to list section %q, got: %#v", "General", sections)
	}
	if !containsString(sections, "NetworkList") {
		t.Errorf("expected redacted summary to list section %q, got: %#v", "NetworkList", sections)
	}
}

// containsString reports whether s (as a string) is present among the
// []interface{} decoded from a JSON array log field.
func containsString(items []interface{}, s string) bool {
	for _, it := range items {
		if str, ok := it.(string); ok && str == s {
			return true
		}
	}
	return false
}
