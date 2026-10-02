package platform

// zf1b: drift-guard tests (internal-task, item 3).
//
// The masked-secret guard's per-platform "modeled section"/"modeled
// General field" maps declared next to each Overlay* function in
// overlay.go are the single source of truth both for the overlay's own
// section-replacement behavior AND for masked.go's detectors. If someone
// adds an assignment to an Overlay* function without updating its key set
// (or vice versa), the guard silently stops protecting (or starts
// over-blocking) that field.
//
// These tests close that gap mechanically: for each platform, build a
// `live` and a `planned` entity where every top-level field (and, within
// General, every General field) is set to a DISTINCT, non-zero value via
// reflection (fillDistinct), run the real Overlay* function, and assert
// that the set of top-level JSON keys whose encoding changed is EXACTLY
// the platform's modeled-sections map (and, within General, EXACTLY the
// modeled-General-keys map). A missed or extra assignment line in the
// Overlay function shows up as a mismatched key set.

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// fillDistinct recursively populates every settable field reachable from v
// with values distinguishable from the other variant's, so that after JSON
// marshaling every top-level field's encoding differs between a "live"-
// variant and a "planned"-variant fill. It intentionally does nothing
// clever for maps or unexported fields: the drift test only needs "at
// least one leaf per top-level field" to differ (see internal-task task spec),
// not exhaustive fuzzing.
//
// The variant parameter selects which of the two distinguishable fills
// this call produces: every leaf's value is a function of (seed, variant) such that
// the same seed with variant=true and variant=false always yields
// different encoded JSON — bool leaves in particular would otherwise
// always land on the same value (there being only two to choose from) if
// derived from seed content alone.
func fillDistinct(v reflect.Value, seed string, variant bool, depth int) {
	if depth > 10 || !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			if !v.CanSet() {
				return
			}
			v.Set(reflect.New(v.Type().Elem()))
		}
		fillDistinct(v.Elem(), seed, variant, depth+1)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if !f.CanSet() {
				continue
			}
			fillDistinct(f, seed+"_"+v.Type().Field(i).Name, variant, depth+1)
		}
	case reflect.Slice:
		if v.CanSet() && v.Len() == 0 {
			elem := reflect.New(v.Type().Elem()).Elem()
			fillDistinct(elem, seed+"0", variant, depth+1)
			v.Set(reflect.Append(v, elem))
		}
	case reflect.String:
		if v.CanSet() {
			v.SetString(seed)
		}
	case reflect.Bool:
		if v.CanSet() {
			v.SetBool(variant)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.CanSet() {
			n := int64(len(seed)) + 1
			if !variant {
				n = -n
			}
			v.SetInt(n)
		}
	}
}

// jsonTopLevel marshals v and decodes it into a map[string]any, for
// top-level-key diffing.
func jsonTopLevel(t *testing.T, v any) map[string]any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return m
}

// changedKeys returns the set of keys whose JSON-encoded value differs
// between before and after (including keys present in only one side).
func changedKeys(before, after map[string]any) map[string]bool {
	out := map[string]bool{}
	seen := map[string]bool{}
	for k := range before {
		seen[k] = true
	}
	for k := range after {
		seen[k] = true
	}
	for k := range seen {
		bv, bok := before[k]
		av, aok := after[k]
		if bok != aok {
			out[k] = true
			continue
		}
		bj, _ := json.Marshal(bv)
		aj, _ := json.Marshal(av)
		if string(bj) != string(aj) {
			out[k] = true
		}
	}
	return out
}

func sortedKeysOf(m map[string]bool) []string {
	var out []string
	for k, v := range m {
		if v {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// assertDriftMatches is the shared assertion for every platform's drift
// test: it diffs beforeTop/afterTop and beforeGeneral/afterGeneral against
// the platform's modeled-sections and modeled-General-keys maps.
func assertDriftMatches(t *testing.T, beforeTop, afterTop map[string]any, modeledSections map[string]bool, generalKey string, beforeGeneral, afterGeneral map[string]any, modeledGeneral map[string]bool) {
	t.Helper()

	gotTop := sortedKeysOf(changedKeys(beforeTop, afterTop))
	wantTop := sortedKeysOf(modeledSections)
	if !reflect.DeepEqual(gotTop, wantTop) {
		t.Errorf("top-level keys changed by Overlay = %v, want exactly the modeled-sections set %v", gotTop, wantTop)
	}

	if generalKey == "" {
		return
	}
	gotGeneral := sortedKeysOf(changedKeys(beforeGeneral, afterGeneral))
	wantGeneral := sortedKeysOf(modeledGeneral)
	if !reflect.DeepEqual(gotGeneral, wantGeneral) {
		t.Errorf("%s sub-keys changed by Overlay = %v, want exactly the modeled-General-keys set %v", generalKey, gotGeneral, wantGeneral)
	}
}

func asObj(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	v, ok := m[key]
	if !ok || v == nil {
		return map[string]any{}
	}
	obj, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected %q to decode as an object, got %T", key, v)
	}
	return obj
}

func TestOverlayAppleOsXUpdateEntity_DriftGuard(t *testing.T) {
	t.Parallel()
	live := &sdk.AppleOsXDeviceProfileEntityV2{}
	fillDistinct(reflect.ValueOf(live).Elem(), "live", true, 0)
	beforeTop := jsonTopLevel(t, live)
	beforeGeneral := asObj(t, beforeTop, "General")

	planned := &sdk.AppleOsXDeviceProfileEntityV2{}
	fillDistinct(reflect.ValueOf(planned).Elem(), "planned", false, 0)

	out := OverlayAppleOsXUpdateEntity(live, planned)
	afterTop := jsonTopLevel(t, out)
	afterGeneral := asObj(t, afterTop, "General")

	assertDriftMatches(t, beforeTop, afterTop, appleOsXModeledSections, "General", beforeGeneral, afterGeneral, modeledGeneralV2Keys)
}

func TestOverlayAndroidUpdateEntity_DriftGuard(t *testing.T) {
	t.Parallel()
	live := &sdk.AndroidDeviceProfileV2Entity{}
	fillDistinct(reflect.ValueOf(live).Elem(), "live", true, 0)
	beforeTop := jsonTopLevel(t, live)
	beforeGeneral := asObj(t, beforeTop, "General")

	planned := &sdk.AndroidDeviceProfileV2Entity{}
	fillDistinct(reflect.ValueOf(planned).Elem(), "planned", false, 0)

	out := OverlayAndroidUpdateEntity(live, planned)
	afterTop := jsonTopLevel(t, out)
	afterGeneral := asObj(t, afterTop, "General")

	assertDriftMatches(t, beforeTop, afterTop, androidModeledSections, "General", beforeGeneral, afterGeneral, modeledGeneralV2Keys)
}

func TestOverlayAppleiOSUpdateEntity_DriftGuard(t *testing.T) {
	t.Parallel()
	live := &sdk.AppleDeviceProfileV2Entity{}
	fillDistinct(reflect.ValueOf(live).Elem(), "live", true, 0)
	beforeTop := jsonTopLevel(t, live)
	beforeGeneral := asObj(t, beforeTop, "General")

	planned := &sdk.AppleDeviceProfileV2Entity{}
	fillDistinct(reflect.ValueOf(planned).Elem(), "planned", false, 0)

	out := OverlayAppleiOSUpdateEntity(live, planned)
	afterTop := jsonTopLevel(t, out)
	afterGeneral := asObj(t, afterTop, "General")

	assertDriftMatches(t, beforeTop, afterTop, appleiOSModeledSections, "General", beforeGeneral, afterGeneral, modeledGeneralV2Keys)
}

func TestOverlayWindows10UpdateEntity_DriftGuard(t *testing.T) {
	t.Parallel()
	live := &sdk.WinRTDeviceProfileV2Entity{}
	fillDistinct(reflect.ValueOf(live).Elem(), "live", true, 0)
	beforeTop := jsonTopLevel(t, live)
	beforeGeneral := asObj(t, beforeTop, "General")

	planned := &sdk.WinRTDeviceProfileV2Entity{}
	fillDistinct(reflect.ValueOf(planned).Elem(), "planned", false, 0)

	out := OverlayWindows10UpdateEntity(live, planned)
	afterTop := jsonTopLevel(t, out)
	afterGeneral := asObj(t, afterTop, "General")

	assertDriftMatches(t, beforeTop, afterTop, windows10ModeledSections, "General", beforeGeneral, afterGeneral, modeledGeneralV2Keys)
}

func TestOverlayWindowsRuggedUpdateEntity_DriftGuard(t *testing.T) {
	t.Parallel()
	live := &sdk.QnxDeviceProfileEntityV2{}
	fillDistinct(reflect.ValueOf(live).Elem(), "live", true, 0)
	beforeTop := jsonTopLevel(t, live)
	beforeGeneral := asObj(t, beforeTop, "General")

	planned := &sdk.QnxDeviceProfileEntityV2{}
	fillDistinct(reflect.ValueOf(planned).Elem(), "planned", false, 0)

	out := OverlayWindowsRuggedUpdateEntity(live, planned)
	afterTop := jsonTopLevel(t, out)
	afterGeneral := asObj(t, afterTop, "General")

	assertDriftMatches(t, beforeTop, afterTop, windowsRuggedModeledSections, "General", beforeGeneral, afterGeneral, modeledGeneralV2Keys)
}

func TestOverlayLinuxUpdateEntity_DriftGuard(t *testing.T) {
	t.Parallel()
	live := &sdk.LinuxDeviceProfileEntity1V4{}
	fillDistinct(reflect.ValueOf(live).Elem(), "live", true, 0)
	beforeTop := jsonTopLevel(t, live)
	beforeGeneral := asObj(t, beforeTop, "general")

	planned := &sdk.LinuxDeviceProfileEntity1V4{}
	fillDistinct(reflect.ValueOf(planned).Elem(), "planned", false, 0)

	out := OverlayLinuxUpdateEntity(live, planned)
	afterTop := jsonTopLevel(t, out)
	afterGeneral := asObj(t, afterTop, "general")

	assertDriftMatches(t, beforeTop, afterTop, linuxModeledSections, "general", beforeGeneral, afterGeneral, modeledGeneralV4Keys)
}
