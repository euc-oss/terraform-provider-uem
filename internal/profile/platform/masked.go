package platform

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// UEMMaskedSecret is the literal string UEM's GET endpoints substitute for
// the live value of any ENCRYPTED setting.
//
// CANONICAL SOURCE (UEM 26.2, verified in-source): ProfileServiceV2Helper.cs
// lines 135 and 2087, and ProfileResourceServiceV2Helper.cs lines 51 and
// 614, all render a masked ENCRYPTED setting as exactly this literal — five
// U+002A ASTERISK characters, nothing more, nothing less. The masking is
// driven by a per-setting "is this ENCRYPTED" flag internal to UEM, NOT by
// the setting's field/key name — there is no way to tell from the outside,
// by name alone, which fields on any given entity are subject to masking.
// That is why the comparison below (and everywhere UEMMaskedSecret is used)
// is an EXACT string equality against this constant, never a key-name
// heuristic: "****" (4), "******" (6), and "*****x" must NOT match, and a
// masked value under an innocuous-looking key (e.g. "Server") must.
//
// This also means the guard in this file is PERMANENT, not a workaround:
// UEM's write path performs a full-replace with no sentinel compare, so an
// echoed "*****" is encrypted and persisted as the new secret. Live
// confirmed on a real tenant: macOS EmailList[0].IncomingPassword came back
// masked on GET.
const UEMMaskedSecret = "*****"

// MaskedUnmodeledPaths inspects the JSON representation of live (the entity
// fetched from UEM before an Update's overlay is applied) for string leaves
// that are exactly UEMMaskedSecret, restricted to the parts of the entity
// this provider does NOT model:
//
//   - Any top-level JSON key NOT present (true) in modeledSections is
//     walked in full (nested maps and slices included).
//   - The top-level JSON key named generalKey (if present in the marshaled
//     object and itself a JSON object) is walked too, but only its
//     sub-keys NOT present (true) in modeledGeneral — the rest of General
//     passes through from live untouched by the overlay, exactly like an
//     unmodeled top-level section.
//   - Every other top-level key (i.e. one this provider fully overlays
//     from the plan) is skipped entirely: whatever a fully modeled section
//     contains on live is about to be replaced wholesale by the plan, so a
//     "*****" there is irrelevant — and a legitimate plan value of
//     "*****" must never be flagged (this function only ever looks at
//     live, never at planned).
//
// Returned paths use dotted names for object fields and "[i]" for slice
// indexes, e.g. "EmailList[0].IncomingPassword", "General.Password". Only
// paths are returned, never the underlying (masked) values. The result is
// sorted for deterministic error messages and test assertions.
//
// This is a safety guard and FAILS CLOSED: any failure to fully inspect
// live (a Marshal/Unmarshal error, or a General value that isn't a JSON
// object) is returned as a non-nil error with nil paths, never treated as
// "no masked secrets found". Callers MUST refuse the update when err != nil
// — see refuseIfMaskedUnmodeled in resource_platforms.go.
func MaskedUnmodeledPaths(live any, modeledSections map[string]bool, generalKey string, modeledGeneral map[string]bool) ([]string, error) {
	if live == nil {
		return nil, nil
	}
	data, err := json.Marshal(live)
	if err != nil {
		return nil, fmt.Errorf("marshaling live entity for masked-secret inspection: %w", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("unmarshaling live entity for masked-secret inspection: %w", err)
	}

	var paths []string
	for key, raw := range top {
		var val any
		if err := json.Unmarshal(raw, &val); err != nil {
			return nil, fmt.Errorf("unmarshaling live entity key %q for masked-secret inspection: %w", key, err)
		}
		if key == generalKey {
			if val == nil {
				continue
			}
			obj, ok := val.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("inspecting live entity key %q for masked-secret inspection: expected a JSON object, got %T", key, val)
			}
			for gk, gv := range obj {
				if modeledGeneral[gk] {
					continue
				}
				paths = append(paths, collectMaskedPaths(key+"."+gk, gv)...)
			}
			continue
		}
		if modeledSections[key] {
			continue
		}
		paths = append(paths, collectMaskedPaths(key, val)...)
	}
	sort.Strings(paths)
	return paths, nil
}

// collectMaskedPaths recursively walks a decoded JSON value (map, slice, or
// scalar) collecting the dotted/indexed paths of string leaves that are
// exactly UEMMaskedSecret.
func collectMaskedPaths(prefix string, v any) []string {
	switch t := v.(type) {
	case string:
		if t == UEMMaskedSecret {
			return []string{prefix}
		}
		return nil
	case map[string]any:
		var out []string
		for k, vv := range t {
			out = append(out, collectMaskedPaths(prefix+"."+k, vv)...)
		}
		return out
	case []any:
		var out []string
		for i, vv := range t {
			out = append(out, collectMaskedPaths(fmt.Sprintf("%s[%d]", prefix, i), vv)...)
		}
		return out
	default:
		return nil
	}
}

// walkKeptPath consumes a dot-separated path (segments already split, each
// optionally suffixed "[]" to mean "this key's value is a slice: iterate
// every element and continue matching the remaining segments against each
// element") against v, a decoded JSON tree (map[string]any / []any /
// scalar), and returns the masked-secret paths found at (or, since the
// terminal call defers to collectMaskedPaths, recursively under) the path's
// terminus. Prefix accumulates the concrete, index-resolved path built so
// far for reporting.
//
// This is the D4 (01o task D2) walk used to catch a masked "*****" inside a
// section-level KEPT leaf/subtree — one this provider's overlay carries
// over from live even though the section it lives in (AndroidForWorkCustomMessages,
// CredentialsList) is otherwise "modeled" and so skipped whole by
// MaskedUnmodeledPaths above. A missing/nil/wrong-shaped step along the
// path is not an error here: it just means there's nothing to check (e.g.
// live has no CredentialsList at all).
func walkKeptPath(v any, segments []string, prefix string) []string {
	if len(segments) == 0 {
		return collectMaskedPaths(prefix, v)
	}
	seg := segments[0]
	isSlice := strings.HasSuffix(seg, "[]")
	name := strings.TrimSuffix(seg, "[]")

	obj, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	child, present := obj[name]
	if !present || child == nil {
		return nil
	}
	path := name
	if prefix != "" {
		path = prefix + "." + name
	}
	if !isSlice {
		return walkKeptPath(child, segments[1:], path)
	}
	items, ok := child.([]any)
	if !ok {
		return nil
	}
	var out []string
	for i, item := range items {
		out = append(out, walkKeptPath(item, segments[1:], fmt.Sprintf("%s[%d]", path, i))...)
	}
	return out
}

// maskedKeptPaths reports masked-secret paths found under any of keptPaths
// (dotted paths, "[]"-suffixed segments meaning "iterate this slice") in
// live. Unlike MaskedUnmodeledPaths, this deliberately looks INSIDE
// modeled top-level sections, restricted to the specific sub-paths the
// overlay is known to carry over from live unmodified (see
// androidKeptLiveLeaves / appleOsXKeptLiveSubtrees in overlay.go).
func maskedKeptPaths(live any, keptPaths []string) ([]string, error) {
	if live == nil || len(keptPaths) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(live)
	if err != nil {
		return nil, fmt.Errorf("marshaling live entity for masked-secret inspection: %w", err)
	}
	var tree map[string]any
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("unmarshaling live entity for masked-secret inspection: %w", err)
	}
	var out []string
	for _, p := range keptPaths {
		out = append(out, walkKeptPath(tree, strings.Split(p, "."), "")...)
	}
	return out, nil
}

// MaskedUnmodeledAppleOsX reports masked-secret paths in the parts of a
// live macOS entity that OverlayAppleOsXUpdateEntity does not overlay from
// the plan. See appleOsXModeledSections / modeledGeneralV2Keys in
// overlay.go for the source-of-truth key sets. Also reports (D4, 01o task
// D2) masked-secret paths in CredentialsList[].CertificateMetadata /
// CertificatePreference / IdentityPreference — sub-objects the overlay
// keeps from live (see overlayAppleOsXCredentialsList) despite
// CredentialsList itself being a modeled, otherwise plan-owned section.
// This scan is conservative: it checks every live CredentialsList entry's
// kept sub-objects, even entries that won't end up matching any planned
// entry (and so would never actually be carried over) — at guard time,
// before the plan/live match runs, we don't yet know which will match, and
// refusing on a masked value that would NOT have been carried over is a
// false positive we accept in exchange for never missing one that would.
func MaskedUnmodeledAppleOsX(live *sdk.AppleOsXDeviceProfileEntityV2) ([]string, error) {
	if live == nil {
		return nil, nil
	}
	paths, err := MaskedUnmodeledPaths(live, appleOsXModeledSections, "General", modeledGeneralV2Keys)
	if err != nil {
		return nil, err
	}
	kept, err := maskedKeptPaths(live, appleOsXKeptLiveSubtrees)
	if err != nil {
		return nil, err
	}
	paths = append(paths, kept...)
	sort.Strings(paths)
	return paths, nil
}

// MaskedUnmodeledAndroid reports masked-secret paths in the parts of a live
// Android entity that OverlayAndroidUpdateEntity does not overlay from the
// plan. See androidModeledSections / modeledGeneralV2Keys in overlay.go.
// Also reports (D4, 01o task D2) masked-secret paths in
// AndroidForWorkCustomMessages.LongSupportMessage/ShortSupportMessage —
// leaves the overlay keeps from live (see
// overlayAndroidForWorkCustomMessages) despite AndroidForWorkCustomMessages
// itself being a modeled section.
func MaskedUnmodeledAndroid(live *sdk.AndroidDeviceProfileV2Entity) ([]string, error) {
	if live == nil {
		return nil, nil
	}
	paths, err := MaskedUnmodeledPaths(live, androidModeledSections, "General", modeledGeneralV2Keys)
	if err != nil {
		return nil, err
	}
	kept, err := maskedKeptPaths(live, androidKeptLiveLeaves)
	if err != nil {
		return nil, err
	}
	paths = append(paths, kept...)
	sort.Strings(paths)
	return paths, nil
}

// MaskedUnmodeledAppleiOS reports masked-secret paths in the parts of a
// live Apple iOS entity that OverlayAppleiOSUpdateEntity does not overlay
// from the plan. See appleiOSModeledSections / modeledGeneralV2Keys in
// overlay.go.
func MaskedUnmodeledAppleiOS(live *sdk.AppleDeviceProfileV2Entity) ([]string, error) {
	if live == nil {
		return nil, nil
	}
	return MaskedUnmodeledPaths(live, appleiOSModeledSections, "General", modeledGeneralV2Keys)
}

// MaskedUnmodeledWindows10 reports masked-secret paths in the parts of a
// live Windows 10 entity that OverlayWindows10UpdateEntity does not overlay
// from the plan. See windows10ModeledSections / modeledGeneralV2Keys in
// overlay.go.
func MaskedUnmodeledWindows10(live *sdk.WinRTDeviceProfileV2Entity) ([]string, error) {
	if live == nil {
		return nil, nil
	}
	return MaskedUnmodeledPaths(live, windows10ModeledSections, "General", modeledGeneralV2Keys)
}

// MaskedUnmodeledWindowsRugged reports masked-secret paths in the parts of
// a live Windows Rugged (QNX) entity that OverlayWindowsRuggedUpdateEntity
// does not overlay from the plan. See windowsRuggedModeledSections /
// modeledGeneralV2Keys in overlay.go.
func MaskedUnmodeledWindowsRugged(live *sdk.QnxDeviceProfileEntityV2) ([]string, error) {
	if live == nil {
		return nil, nil
	}
	return MaskedUnmodeledPaths(live, windowsRuggedModeledSections, "General", modeledGeneralV2Keys)
}

// MaskedUnmodeledLinux reports masked-secret paths in the parts of a live
// Linux entity that OverlayLinuxUpdateEntity does not overlay from the
// plan. See linuxModeledSections / modeledGeneralV4Keys in overlay.go. Note
// the lowercase "general" generalKey — see linuxModeledSections' comment.
func MaskedUnmodeledLinux(live *sdk.LinuxDeviceProfileEntity1V4) ([]string, error) {
	if live == nil {
		return nil, nil
	}
	return MaskedUnmodeledPaths(live, linuxModeledSections, "general", modeledGeneralV4Keys)
}
