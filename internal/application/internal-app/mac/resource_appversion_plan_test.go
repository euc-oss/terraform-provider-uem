package macapplication

import (
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Plan-level tests for app_version's RequiresReplaceIf: a fresh import records
// the API's padded "1.0.0.0" while a TF-authored config says "1.0.0";
// semantic equality does not run at plan time, so without the modifier that
// diff would force a replace.

func appVersionPlanFixture(t *testing.T, stateVersion, configVersion string) (prior, config tftypes.Value) {
	t.Helper()
	dir := t.TempDir()
	dmg := filepath.Join(dir, "app.dmg")
	plist := filepath.Join(dir, "app.plist")
	writeFixtureFile(t, dmg, pathIdentityDMG)
	writeFixtureFile(t, plist, []byte(basePlist))

	prior = macAppObject(t, map[string]tftypes.Value{
		"id":                tftypes.NewValue(tftypes.Number, 42),
		"uuid":              tfStr(appVersionTestUUID),
		"org_group_id":      tftypes.NewValue(tftypes.Number, 10),
		"dmg_file_path":     tfStr(dmg),
		"plist_file_path":   tfStr(plist),
		"app_version":       tfStr(stateVersion),
		"include_content":   tftypes.NewValue(tftypes.Bool, false),
		"dmg_file_sha256":   tfStr(sha256Hex(pathIdentityDMG)),
		"plist_file_sha256": tfStr(mustCanonicalSHA(t, basePlist)),
	})
	config = macAppObject(t, map[string]tftypes.Value{
		"org_group_id":    tftypes.NewValue(tftypes.Number, 10),
		"dmg_file_path":   tfStr(dmg),
		"plist_file_path": tfStr(plist),
		"app_version":     tfStr(configVersion),
		"include_content": tftypes.NewValue(tftypes.Bool, false),
	})
	return prior, config
}

func TestPlan_AppVersion_PaddedEquivalent_InPlaceUpdateNoAPICalls(t *testing.T) {
	prior, config := appVersionPlanFixture(t, "1.0.0.0", "1.0.0")
	server, counts := newPlanHarnessServer(t)

	plan := runPlan(t, server, prior, config)
	if plan.requiresReplace("app_version") || len(plan.resp.RequiresReplace) != 0 {
		t.Fatalf("expected no replacement for state 1.0.0.0 vs config 1.0.0, got RequiresReplace=%v", plan.resp.RequiresReplace)
	}
	if got, _ := strAttr(t, plan.planned, "app_version"); got != "1.0.0" {
		t.Fatalf("planned app_version = %q, want %q", got, "1.0.0")
	}

	newState := runApply(t, server, prior, config, plan)
	if counts.total() != 0 {
		t.Fatalf("expected zero SDK calls for the padded app_version update, got %s", counts)
	}
	if got, _ := strAttr(t, newState, "app_version"); got != "1.0.0" {
		t.Fatalf("new state app_version = %q, want config value %q", got, "1.0.0")
	}
}

func TestPlan_AppVersion_GenuineChange_Replaces(t *testing.T) {
	prior, config := appVersionPlanFixture(t, "1.0.0.0", "1.0.1")
	server, _ := newPlanHarnessServer(t)
	if plan := runPlan(t, server, prior, config); !plan.requiresReplace("app_version") {
		t.Fatalf("expected RequiresReplace to contain app_version, got %v", plan.resp.RequiresReplace)
	}
}

// Null prior app_version (e.g. state from before app_version was read back)
// keeps the old behavior: a configured value replaces.
func TestPlan_AppVersion_NullState_Replaces(t *testing.T) {
	prior, config := appVersionPlanFixture(t, "unused", "1.0.0")
	var attrs map[string]tftypes.Value
	if err := prior.As(&attrs); err != nil {
		t.Fatalf("As: %v", err)
	}
	attrs["app_version"] = tftypes.NewValue(tftypes.String, nil)
	prior = tftypes.NewValue(prior.Type(), attrs)

	server, _ := newPlanHarnessServer(t)
	if plan := runPlan(t, server, prior, config); !plan.requiresReplace("app_version") {
		t.Fatalf("expected RequiresReplace to contain app_version for a null prior, got %v", plan.resp.RequiresReplace)
	}
}

// createWithAPIVersion creates through the framework server (PlanResourceChange
// + ApplyResourceChange, null prior) with the given configured app_version and
// a fake GET echoing apiVersion, and returns the new state's app_version.
func createWithAPIVersion(t *testing.T, configVersion, apiVersion string) string {
	t.Helper()
	dir := t.TempDir()
	dmg := filepath.Join(dir, "app.dmg")
	plist := filepath.Join(dir, "app.plist")
	writeFixtureFile(t, dmg, pathIdentityDMG)
	writeFixtureFile(t, plist, []byte(basePlist))

	server, counts := newPlanHarnessServerWithAPIVersion(t, apiVersion)
	prior := tftypes.NewValue(macAppObjectType(t), nil)
	config := macAppObject(t, map[string]tftypes.Value{
		"org_group_id":    tftypes.NewValue(tftypes.Number, 10),
		"dmg_file_path":   tfStr(dmg),
		"plist_file_path": tfStr(plist),
		"app_version":     tfStr(configVersion),
	})
	plan := runPlan(t, server, prior, config)
	// runApply fails the test on any error diagnostic, including the
	// framework's "inconsistent result after apply".
	newState := runApply(t, server, prior, config, plan)
	if counts.creates != 1 {
		t.Fatalf("expected a real create, got %s", counts)
	}
	got, _ := strAttr(t, newState, "app_version")
	return got
}

// I-2: Create persists the planned app_version even when UEM's echo is a
// .NET-style reformat ("1.01" -> "1.1.0.0").
func TestCreate_AppVersion_LeadingZeroConfig_PersistsPlanned(t *testing.T) {
	if got := createWithAPIVersion(t, "1.01", "1.1.0.0"); got != "1.01" {
		t.Fatalf("app_version after create = %q, want planned %q", got, "1.01")
	}
}

// I-2(b) independent of the numeric equivalence: a non-numeric version the
// API echoes differently must still be persisted as planned.
func TestCreate_AppVersion_NonNumericConfig_PersistsPlanned(t *testing.T) {
	if got := createWithAPIVersion(t, "1.0-beta", "1.0-beta.0"); got != "1.0-beta" {
		t.Fatalf("app_version after create = %q, want planned %q", got, "1.0-beta")
	}
}

// Leading-zero-equivalent config vs API-padded state (fresh import of
// "1.01"): in-place, not replace.
func TestPlan_AppVersion_LeadingZeroEquivalent_NoReplace(t *testing.T) {
	prior, config := appVersionPlanFixture(t, "1.1.0.0", "1.01")
	server, _ := newPlanHarnessServer(t)
	if plan := runPlan(t, server, prior, config); len(plan.resp.RequiresReplace) != 0 {
		t.Fatalf("expected no replacement for 1.1.0.0 vs 1.01, got %v", plan.resp.RequiresReplace)
	}
}
