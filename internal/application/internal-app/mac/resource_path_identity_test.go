package macapplication

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Plan-level tests (tfprotov6 PlanResourceChange / ApplyResourceChange via
// providerserver) for dmg_file_path / plist_file_path content identity: a
// path-only change to identical content is an in-place update with no API
// call; anything else replaces.

var pathIdentityDMG = []byte("fake-dmg-binary-content-for-path-identity")

type pathIdentityFixture struct {
	dir       string
	oldDMG    string
	oldPlist  string
	dmgSHA    string
	plistSHA  string
	priorVals map[string]tftypes.Value
}

func newPathIdentityFixture(t *testing.T) *pathIdentityFixture {
	t.Helper()
	dir := t.TempDir()
	f := &pathIdentityFixture{
		dir:      dir,
		oldDMG:   filepath.Join(dir, "old", "app.dmg"),
		oldPlist: filepath.Join(dir, "old", "app.plist"),
		dmgSHA:   sha256Hex(pathIdentityDMG),
		plistSHA: mustCanonicalSHA(t, basePlist),
	}
	writeFixtureFile(t, f.oldDMG, pathIdentityDMG)
	writeFixtureFile(t, f.oldPlist, []byte(basePlist))
	f.priorVals = map[string]tftypes.Value{
		"id":                tftypes.NewValue(tftypes.Number, 42),
		"uuid":              tfStr(appVersionTestUUID),
		"org_group_id":      tftypes.NewValue(tftypes.Number, 10),
		"dmg_file_path":     tfStr(f.oldDMG),
		"plist_file_path":   tfStr(f.oldPlist),
		"app_version":       tfStr("1.0.0"),
		"include_content":   tftypes.NewValue(tftypes.Bool, false),
		"dmg_file_sha256":   tfStr(f.dmgSHA),
		"plist_file_sha256": tfStr(f.plistSHA),
	}
	return f
}

func (f *pathIdentityFixture) prior(t *testing.T, overrides map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	vals := make(map[string]tftypes.Value, len(f.priorVals))
	for k, v := range f.priorVals {
		vals[k] = v
	}
	for k, v := range overrides {
		vals[k] = v
	}
	return macAppObject(t, vals)
}

func (f *pathIdentityFixture) config(t *testing.T, dmgPath, plistPath string) tftypes.Value {
	t.Helper()
	return macAppObject(t, map[string]tftypes.Value{
		"org_group_id":    tftypes.NewValue(tftypes.Number, 10),
		"dmg_file_path":   tfStr(dmgPath),
		"plist_file_path": tfStr(plistPath),
		"app_version":     tfStr("1.0.0"),
		"include_content": tftypes.NewValue(tftypes.Bool, false),
	})
}

func writeFixtureFile(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

// assertInPlacePathUpdate plans, asserts no replacement and the new path in
// the plan, then applies and asserts zero SDK calls and the new path (and
// unchanged identity sha) in the new state.
func assertInPlacePathUpdate(t *testing.T, f *pathIdentityFixture, pathAttr, shaAttr, newPath, wantSHA string, config tftypes.Value) {
	t.Helper()
	server, counts := newPlanHarnessServer(t)
	prior := f.prior(t, nil)

	plan := runPlan(t, server, prior, config)
	if plan.requiresReplace(pathAttr) || len(plan.resp.RequiresReplace) != 0 {
		t.Fatalf("expected no replacement for same-content %s change, got RequiresReplace=%v", pathAttr, plan.resp.RequiresReplace)
	}
	if got, _ := strAttr(t, plan.planned, pathAttr); got != newPath {
		t.Fatalf("planned %s = %q, want %q", pathAttr, got, newPath)
	}
	if got, _ := strAttr(t, plan.planned, shaAttr); got != wantSHA {
		t.Fatalf("planned %s = %q, want prior %q", shaAttr, got, wantSHA)
	}

	newState := runApply(t, server, prior, config, plan)
	if counts.total() != 0 {
		t.Fatalf("expected zero SDK calls for an in-place path update, got %s", counts)
	}
	if got, _ := strAttr(t, newState, pathAttr); got != newPath {
		t.Fatalf("new state %s = %q, want %q", pathAttr, got, newPath)
	}
	if got, _ := strAttr(t, newState, shaAttr); got != wantSHA {
		t.Fatalf("new state %s = %q, want prior %q", shaAttr, got, wantSHA)
	}
}

func assertReplace(t *testing.T, prior, config tftypes.Value, attr string) {
	t.Helper()
	server, _ := newPlanHarnessServer(t)
	plan := runPlan(t, server, prior, config)
	if !plan.requiresReplace(attr) {
		t.Fatalf("expected RequiresReplace to contain %s, got %v", attr, plan.resp.RequiresReplace)
	}
}

func TestPlan_DMGPathChange_SameContent_InPlaceUpdateNoAPICalls(t *testing.T) {
	f := newPathIdentityFixture(t)
	newDMG := filepath.Join(f.dir, "new", "renamed.dmg")
	writeFixtureFile(t, newDMG, pathIdentityDMG)
	assertInPlacePathUpdate(t, f, "dmg_file_path", "dmg_file_sha256", newDMG, f.dmgSHA, f.config(t, newDMG, f.oldPlist))
}

func TestPlan_DMGPathChange_DifferentContent_Replaces(t *testing.T) {
	f := newPathIdentityFixture(t)
	newDMG := filepath.Join(f.dir, "new", "app.dmg")
	writeFixtureFile(t, newDMG, []byte("different-dmg-content"))
	assertReplace(t, f.prior(t, nil), f.config(t, newDMG, f.oldPlist), "dmg_file_path")
}

func TestPlan_DMGPathChange_MissingFile_Replaces(t *testing.T) {
	f := newPathIdentityFixture(t)
	assertReplace(t, f.prior(t, nil), f.config(t, filepath.Join(f.dir, "missing.dmg"), f.oldPlist), "dmg_file_path")
}

func TestPlan_DMGPathChange_NullStateSHA_Replaces(t *testing.T) {
	f := newPathIdentityFixture(t)
	newDMG := filepath.Join(f.dir, "new", "app.dmg")
	writeFixtureFile(t, newDMG, pathIdentityDMG)
	prior := f.prior(t, map[string]tftypes.Value{"dmg_file_sha256": tftypes.NewValue(tftypes.String, nil)})
	assertReplace(t, prior, f.config(t, newDMG, f.oldPlist), "dmg_file_path")
}

// TestPlan_ImportedPKGPathChange_SameContent_InPlaceUpdateNoAPICalls is B31:
// an imported flat package's dmg_file_path already points at an app.pkg file
// (ImportState names it that way for a xar-content blob, see
// TestImportState_XarBlob_SavesAsAppPKG). The path-identity logic is
// content-only (plain SHA-256, see fileSHA256/pathContentChanged), so a
// subsequent config path change to a different .pkg file with the same
// content must still be an in-place update, not a replace — exactly like the
// .dmg case, never treated differently just because the extension is .pkg.
func TestPlan_ImportedPKGPathChange_SameContent_InPlaceUpdateNoAPICalls(t *testing.T) {
	f := newPathIdentityFixture(t)

	pkgContent := append([]byte("xar!"), []byte("\x00\x01fake-flat-package-content-for-path-identity")...)
	pkgSHA := sha256Hex(pkgContent)

	oldPKG := filepath.Join(f.dir, "old", "app.pkg")
	writeFixtureFile(t, oldPKG, pkgContent)

	prior := f.prior(t, map[string]tftypes.Value{
		"dmg_file_path":   tfStr(oldPKG),
		"dmg_file_sha256": tfStr(pkgSHA),
	})

	newPKG := filepath.Join(f.dir, "new", "renamed.pkg")
	writeFixtureFile(t, newPKG, pkgContent)
	config := f.config(t, newPKG, f.oldPlist)

	server, counts := newPlanHarnessServer(t)
	plan := runPlan(t, server, prior, config)
	if plan.requiresReplace("dmg_file_path") || len(plan.resp.RequiresReplace) != 0 {
		t.Fatalf("expected no replacement for same-content dmg_file_path change, got RequiresReplace=%v", plan.resp.RequiresReplace)
	}
	if got, _ := strAttr(t, plan.planned, "dmg_file_path"); got != newPKG {
		t.Fatalf("planned dmg_file_path = %q, want %q", got, newPKG)
	}
	if got, _ := strAttr(t, plan.planned, "dmg_file_sha256"); got != pkgSHA {
		t.Fatalf("planned dmg_file_sha256 = %q, want prior %q", got, pkgSHA)
	}

	newState := runApply(t, server, prior, config, plan)
	if counts.total() != 0 {
		t.Fatalf("expected zero SDK calls for an in-place path update, got %s", counts)
	}
	if got, _ := strAttr(t, newState, "dmg_file_path"); got != newPKG {
		t.Fatalf("new state dmg_file_path = %q, want %q", got, newPKG)
	}
	if got, _ := strAttr(t, newState, "dmg_file_sha256"); got != pkgSHA {
		t.Fatalf("new state dmg_file_sha256 = %q, want prior %q", got, pkgSHA)
	}
}

// TestPlan_DMGPathChange_CleanEquivalentPath_NoDiff is F10: a configured
// dmg_file_path that differs from the recorded state path only in a way
// filepath.Clean normalizes away (a "/./" segment here, matching the design
// note's "./../../x" vs "../../x" example) must plan NO diff at all for the
// attribute — not merely "no replace", but the state value kept verbatim, as
// if the config had matched state exactly. This is a local path-string
// comparison (cleanEquivalentPathUseState), not a content check, so it never
// even reads the file at the cosmetically-different path.
func TestPlan_DMGPathChange_CleanEquivalentPath_NoDiff(t *testing.T) {
	f := newPathIdentityFixture(t)

	// A raw string construction (not filepath.Join, which would itself
	// clean it away) that Clean-normalizes to f.oldDMG but is not equal to
	// it as a string.
	cosmetic := filepath.Dir(f.oldDMG) + string(filepath.Separator) + "." + string(filepath.Separator) + filepath.Base(f.oldDMG)
	if cosmetic == f.oldDMG {
		t.Fatalf("test fixture bug: cosmetic path %q must differ textually from %q", cosmetic, f.oldDMG)
	}
	if filepath.Clean(cosmetic) != f.oldDMG {
		t.Fatalf("test fixture bug: filepath.Clean(%q) = %q, want %q", cosmetic, filepath.Clean(cosmetic), f.oldDMG)
	}

	server, counts := newPlanHarnessServer(t)
	prior := f.prior(t, nil)
	config := f.config(t, cosmetic, f.oldPlist)

	plan := runPlan(t, server, prior, config)
	if len(plan.resp.RequiresReplace) != 0 {
		t.Fatalf("expected no RequiresReplace for a Clean-equivalent path, got %v", plan.resp.RequiresReplace)
	}
	if got, _ := strAttr(t, plan.planned, "dmg_file_path"); got != f.oldDMG {
		t.Fatalf("planned dmg_file_path = %q, want unchanged state value %q (no diff)", got, f.oldDMG)
	}
	planned, err := plan.resp.PlannedState.Unmarshal(macAppObjectType(t))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !planned.Equal(prior) {
		diffs, _ := planned.Diff(prior)
		t.Fatalf("expected planned state to equal prior state (no diff at all), diffs: %v", diffs)
	}

	newState := runApply(t, server, prior, config, plan)
	if counts.total() != 0 {
		t.Fatalf("expected zero SDK calls, got %s", counts)
	}
	if got, _ := strAttr(t, newState, "dmg_file_path"); got != f.oldDMG {
		t.Fatalf("new state dmg_file_path = %q, want unchanged %q", got, f.oldDMG)
	}
}

// TestPlan_PlistPathChange_CleanEquivalentPath_NoDiff mirrors the dmg case
// above for plist_file_path.
func TestPlan_PlistPathChange_CleanEquivalentPath_NoDiff(t *testing.T) {
	f := newPathIdentityFixture(t)

	cosmetic := filepath.Dir(f.oldPlist) + string(filepath.Separator) + "." + string(filepath.Separator) + filepath.Base(f.oldPlist)
	if cosmetic == f.oldPlist {
		t.Fatalf("test fixture bug: cosmetic path %q must differ textually from %q", cosmetic, f.oldPlist)
	}
	if filepath.Clean(cosmetic) != f.oldPlist {
		t.Fatalf("test fixture bug: filepath.Clean(%q) = %q, want %q", cosmetic, filepath.Clean(cosmetic), f.oldPlist)
	}

	server, counts := newPlanHarnessServer(t)
	prior := f.prior(t, nil)
	config := f.config(t, f.oldDMG, cosmetic)

	plan := runPlan(t, server, prior, config)
	if len(plan.resp.RequiresReplace) != 0 {
		t.Fatalf("expected no RequiresReplace for a Clean-equivalent path, got %v", plan.resp.RequiresReplace)
	}
	planned, err := plan.resp.PlannedState.Unmarshal(macAppObjectType(t))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !planned.Equal(prior) {
		diffs, _ := planned.Diff(prior)
		t.Fatalf("expected planned state to equal prior state (no diff at all), diffs: %v", diffs)
	}

	newState := runApply(t, server, prior, config, plan)
	if counts.total() != 0 {
		t.Fatalf("expected zero SDK calls, got %s", counts)
	}
	if got, _ := strAttr(t, newState, "plist_file_path"); got != f.oldPlist {
		t.Fatalf("new state plist_file_path = %q, want unchanged %q", got, f.oldPlist)
	}
}

func TestPlan_PlistPathChange_SameContent_InPlaceUpdateNoAPICalls(t *testing.T) {
	f := newPathIdentityFixture(t)
	newPlist := filepath.Join(f.dir, "new", "app.plist")
	writeFixtureFile(t, newPlist, []byte(basePlist))
	assertInPlacePathUpdate(t, f, "plist_file_path", "plist_file_sha256", newPlist, f.plistSHA, f.config(t, f.oldDMG, newPlist))
}

// A reformatted plist with a different <dict> key order, declaration and
// whitespace at the new path is the same content: no replace.
func TestPlan_PlistPathChange_KeyOrderWhitespaceVariant_InPlaceUpdateNoAPICalls(t *testing.T) {
	f := newPathIdentityFixture(t)
	newPlist := filepath.Join(f.dir, "new", "variant.plist")
	writeFixtureFile(t, newPlist, []byte(keyOrderVariant))
	assertInPlacePathUpdate(t, f, "plist_file_path", "plist_file_sha256", newPlist, f.plistSHA, f.config(t, f.oldDMG, newPlist))
}

func TestPlan_PlistPathChange_DifferentContent_Replaces(t *testing.T) {
	f := newPathIdentityFixture(t)
	newPlist := filepath.Join(f.dir, "new", "app.plist")
	writeFixtureFile(t, newPlist, []byte(keyOrderVariant[:len(keyOrderVariant)-len("</dict></plist>")]+"<key>extra</key><true/></dict></plist>"))
	assertReplace(t, f.prior(t, nil), f.config(t, f.oldDMG, newPlist), "plist_file_path")
}

func TestPlan_PlistPathChange_MissingFile_Replaces(t *testing.T) {
	f := newPathIdentityFixture(t)
	assertReplace(t, f.prior(t, nil), f.config(t, f.oldDMG, filepath.Join(f.dir, "missing.plist")), "plist_file_path")
}

func TestPlan_PlistPathChange_BinaryPlist_Replaces(t *testing.T) {
	f := newPathIdentityFixture(t)
	newPlist := filepath.Join(f.dir, "new", "app.plist")
	writeFixtureFile(t, newPlist, []byte("bplist00\xd1\x01\x02Q"))
	assertReplace(t, f.prior(t, nil), f.config(t, f.oldDMG, newPlist), "plist_file_path")
}

// A plist larger than maxPlistIdentityBytes is never read whole: it fails safe
// and replaces, even though its canonical content matches the recorded sha
// (the padding is insignificant inter-element whitespace).
func TestPlan_PlistPathChange_Oversize_Replaces(t *testing.T) {
	f := newPathIdentityFixture(t)
	i := strings.LastIndex(basePlist, "</plist>")
	if i < 0 {
		t.Fatal("basePlist has no </plist>")
	}
	oversize := basePlist[:i] + strings.Repeat(" ", maxPlistIdentityBytes+1) + basePlist[i:]
	if got := mustCanonicalSHA(t, oversize); got != f.plistSHA {
		t.Fatalf("padded plist canonical sha = %q, want recorded %q (padding must be insignificant)", got, f.plistSHA)
	}
	newPlist := filepath.Join(f.dir, "new", "oversize.plist")
	writeFixtureFile(t, newPlist, []byte(oversize))
	assertReplace(t, f.prior(t, nil), f.config(t, f.oldDMG, newPlist), "plist_file_path")
}

// State written before plist_file_sha256 existed has it null: a plist path
// change then replaces, exactly as before this attribute was introduced.
func TestPlan_PlistPathChange_NullStateSHA_Replaces(t *testing.T) {
	f := newPathIdentityFixture(t)
	newPlist := filepath.Join(f.dir, "new", "app.plist")
	writeFixtureFile(t, newPlist, []byte(basePlist))
	prior := f.prior(t, map[string]tftypes.Value{"plist_file_sha256": tftypes.NewValue(tftypes.String, nil)})
	assertReplace(t, prior, f.config(t, f.oldDMG, newPlist), "plist_file_path")
}

// Old state (plist_file_sha256 null) with an unchanged configuration must
// plan no change at all: the new Computed attribute stays null rather than
// becoming unknown (the framework only marks computed nulls unknown when the
// plan already differs from state) and nothing requires replacement.
func TestPlan_NullPriorPlistSHA_UnchangedConfig_NoDiff(t *testing.T) {
	f := newPathIdentityFixture(t)
	server, _ := newPlanHarnessServer(t)
	prior := f.prior(t, map[string]tftypes.Value{"plist_file_sha256": tftypes.NewValue(tftypes.String, nil)})

	plan := runPlan(t, server, prior, f.config(t, f.oldDMG, f.oldPlist))
	if len(plan.resp.RequiresReplace) != 0 {
		t.Fatalf("expected no RequiresReplace, got %v", plan.resp.RequiresReplace)
	}
	planned, err := plan.resp.PlannedState.Unmarshal(macAppObjectType(t))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !planned.Equal(prior) {
		diffs, _ := planned.Diff(prior)
		t.Fatalf("expected planned state to equal prior state, diffs: %v", diffs)
	}
}

// Import-shaped state (M-4): ImportState never sets plist_file_path, so state
// and config both leave it null and plist_file_sha256 may be null too. A
// dmg-only path change to the same content must still be an in-place update:
// the null Optional+Computed plist_file_path (marked unknown by the framework
// once the plan differs) must not trip plist_file_path's replace rule.
func TestPlan_ImportShapedConfig_DMGPathChange_SameContent_NoReplace(t *testing.T) {
	f := newPathIdentityFixture(t)
	newDMG := filepath.Join(f.dir, "new", "imported.dmg")
	writeFixtureFile(t, newDMG, pathIdentityDMG)

	nullStr := tftypes.NewValue(tftypes.String, nil)
	prior := f.prior(t, map[string]tftypes.Value{
		"plist_file_path":   nullStr,
		"plist_file_sha256": nullStr,
	})
	config := macAppObject(t, map[string]tftypes.Value{
		"org_group_id":    tftypes.NewValue(tftypes.Number, 10),
		"dmg_file_path":   tfStr(newDMG),
		"app_version":     tfStr("1.0.0"),
		"include_content": tftypes.NewValue(tftypes.Bool, false),
	})

	server, counts := newPlanHarnessServer(t)
	plan := runPlan(t, server, prior, config)
	if len(plan.resp.RequiresReplace) != 0 {
		t.Fatalf("expected no replacement for an import-shaped dmg path-only change, got RequiresReplace=%v", plan.resp.RequiresReplace)
	}
	if got, _ := strAttr(t, plan.planned, "dmg_file_path"); got != newDMG {
		t.Fatalf("planned dmg_file_path = %q, want %q", got, newDMG)
	}
	if !plan.planned["plist_file_path"].IsNull() {
		t.Fatalf("planned plist_file_path = %v, want null", plan.planned["plist_file_path"])
	}

	newState := runApply(t, server, prior, config, plan)
	if counts.total() != 0 {
		t.Fatalf("expected zero SDK calls, got %s", counts)
	}
	if got, _ := strAttr(t, newState, "dmg_file_path"); got != newDMG {
		t.Fatalf("new state dmg_file_path = %q, want %q", got, newDMG)
	}
}
