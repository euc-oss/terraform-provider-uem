package platform

// SDK 26.2 changed General.AfwOemType (GeneralPayloadV2Entity, and the Linux
// GeneralPayloadV4Entity) from string to *int (0 = Samsung, 1 = Zebra). The provider does not model the field: it
// passes through from live on the Update read-modify-write, and the
// masked-secret guard scans it as an unmodeled General sub-key. These tests
// pin that the new numeric shape decodes, survives the overlay unchanged
// (including the nil and explicit-0 cases), and never trips or breaks the
// masked-secret scan.

import (
	"encoding/json"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
)

func decodeAndroidLive(t *testing.T, body string) *sdk.AndroidDeviceProfileV2Entity {
	t.Helper()
	var live sdk.AndroidDeviceProfileV2Entity
	if err := json.Unmarshal([]byte(body), &live); err != nil {
		t.Fatalf("decode live Android entity: %v", err)
	}
	return &live
}

func marshaledGeneral(t *testing.T, e *sdk.AndroidDeviceProfileV2Entity) map[string]json.RawMessage {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		t.Fatalf("unmarshal top: %v", err)
	}
	var general map[string]json.RawMessage
	if err := json.Unmarshal(top["General"], &general); err != nil {
		t.Fatalf("unmarshal General: %v", err)
	}
	return general
}

func TestAfwOemType_PassesThroughAndroidOverlay(t *testing.T) {
	cases := []struct {
		name    string
		live    string
		wantRaw string // "" means the key must be absent from the Update body.
	}{
		{"samsung_zero", `{"General":{"Name":"live","AfwOemSettingsEnabled":true,"AfwOemType":0}}`, "0"},
		{"zebra_one", `{"General":{"Name":"live","AfwOemSettingsEnabled":true,"AfwOemType":1}}`, "1"},
		{"absent", `{"General":{"Name":"live"}}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			live := decodeAndroidLive(t, tc.live)
			planned := &sdk.AndroidDeviceProfileV2Entity{General: &sdk.GeneralPayloadV2Entity{Name: "planned"}}

			got := OverlayAndroidUpdateEntity(live, planned)

			if got.General.Name != "planned" {
				t.Fatalf("modeled Name not overlaid: %q", got.General.Name)
			}
			raw, present := marshaledGeneral(t, got)["AfwOemType"]
			if tc.wantRaw == "" {
				if present {
					t.Fatalf("AfwOemType should be omitted when absent live, got %s", raw)
				}
				return
			}
			if !present || string(raw) != tc.wantRaw {
				t.Fatalf("AfwOemType = %s (present=%v), want %s", raw, present, tc.wantRaw)
			}
		})
	}
}

func TestAfwOemType_MaskedScanIgnoresNumericAndStillFindsMasks(t *testing.T) {
	live := decodeAndroidLive(t, `{"General":{"Name":"live","AfwOemSettingsEnabled":true,"AfwOemType":1}}`)
	paths, err := MaskedUnmodeledAndroid(live)
	if err != nil {
		t.Fatalf("MaskedUnmodeledAndroid: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("numeric AfwOemType must not be reported as masked, got %v", paths)
	}

	// Positive control: with AfwOemType present, a masked unmodeled General
	// string is still detected.
	live.General.Password = UEMMaskedSecret
	paths, err = MaskedUnmodeledAndroid(live)
	if err != nil {
		t.Fatalf("MaskedUnmodeledAndroid: %v", err)
	}
	if len(paths) != 1 || paths[0] != "General.Password" {
		t.Fatalf("expected [General.Password], got %v", paths)
	}
}

func decodeLinuxLive(t *testing.T, body string) *sdk.LinuxDeviceProfileEntity1V4 {
	t.Helper()
	var live sdk.LinuxDeviceProfileEntity1V4
	if err := json.Unmarshal([]byte(body), &live); err != nil {
		t.Fatalf("decode live Linux entity: %v", err)
	}
	return &live
}

// marshaledLinuxGeneral returns the Linux entity's General sub-keys. Note the
// lowercase "general" top-level key (LinuxDeviceProfileEntity1V4's JSON tag).
func marshaledLinuxGeneral(t *testing.T, e *sdk.LinuxDeviceProfileEntity1V4) map[string]json.RawMessage {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		t.Fatalf("unmarshal top: %v", err)
	}
	var general map[string]json.RawMessage
	if err := json.Unmarshal(top["general"], &general); err != nil {
		t.Fatalf("unmarshal general: %v", err)
	}
	return general
}

func TestAfwOemType_PassesThroughLinuxV4Overlay(t *testing.T) {
	cases := []struct {
		name    string
		live    string
		wantRaw string // "" means the key must be absent from the Update body.
	}{
		{"samsung_zero", `{"general":{"Name":"live","AfwOemSettingsEnabled":true,"AfwOemType":0}}`, "0"},
		{"zebra_one", `{"general":{"Name":"live","AfwOemSettingsEnabled":true,"AfwOemType":1}}`, "1"},
		{"absent", `{"general":{"Name":"live"}}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			live := decodeLinuxLive(t, tc.live)
			plannedGeneral, err := BuildGeneralV4Create(&profilemodels.ProfileResourceModel{
				Name:         types.StringValue("planned"),
				ProfileScope: types.StringValue("Production"),
			})
			if err != nil {
				t.Fatalf("BuildGeneralV4Create: %v", err)
			}
			if plannedGeneral.AfwOemType != nil {
				t.Fatalf("BuildGeneralV4Create must not model AfwOemType, got %d", *plannedGeneral.AfwOemType)
			}
			planned := &sdk.LinuxDeviceProfileEntity1V4{General: plannedGeneral}

			got := OverlayLinuxUpdateEntity(live, planned)

			if got.General.Name != "planned" {
				t.Fatalf("modeled Name not overlaid: %q", got.General.Name)
			}
			raw, present := marshaledLinuxGeneral(t, got)["AfwOemType"]
			if tc.wantRaw == "" {
				if present {
					t.Fatalf("AfwOemType should be omitted when absent live, got %s", raw)
				}
				return
			}
			if !present || string(raw) != tc.wantRaw {
				t.Fatalf("AfwOemType = %s (present=%v), want %s", raw, present, tc.wantRaw)
			}
		})
	}
}

func TestAfwOemType_LinuxMaskedScanIgnoresNumericAndStillFindsMasks(t *testing.T) {
	live := decodeLinuxLive(t, `{"general":{"Name":"live","AfwOemSettingsEnabled":true,"AfwOemType":1}}`)
	paths, err := MaskedUnmodeledLinux(live)
	if err != nil {
		t.Fatalf("MaskedUnmodeledLinux: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("numeric AfwOemType must not be reported as masked, got %v", paths)
	}

	// Positive control: with AfwOemType present, a masked unmodeled general
	// string is still detected (lowercase "general" path, see
	// linuxModeledSections).
	live.General.Password = UEMMaskedSecret
	paths, err = MaskedUnmodeledLinux(live)
	if err != nil {
		t.Fatalf("MaskedUnmodeledLinux: %v", err)
	}
	if len(paths) != 1 || paths[0] != "general.Password" {
		t.Fatalf("expected [general.Password], got %v", paths)
	}
}
