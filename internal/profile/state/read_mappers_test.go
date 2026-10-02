package state

import (
	"context"
	"testing"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestReadGeneralV4IntoState_WireOne_MapsToProduction(t *testing.T) {
	t.Parallel()

	g := &sdk.GeneralPayloadV4Entity{
		Name:         "Linux Profile",
		ProfileScope: sdk.IntPtr(1),
	}
	data := &profilemodels.ProfileResourceModel{}

	readGeneralV4IntoState(context.Background(), data, g)

	if data.ProfileScope.ValueString() != "Production" {
		t.Errorf("ProfileScope = %q, want %q", data.ProfileScope.ValueString(), "Production")
	}
}

func TestReadGeneralV4IntoState_WireNilOrOther_LeavesProfileScopeUnset(t *testing.T) {
	t.Parallel()

	t.Run("nil wire value", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{
			ProfileScope: types.StringValue("sentinel"),
		}
		g := &sdk.GeneralPayloadV4Entity{
			Name:         "Linux Profile",
			ProfileScope: nil,
		}

		readGeneralV4IntoState(context.Background(), data, g)

		if data.ProfileScope.ValueString() != "sentinel" {
			t.Errorf("ProfileScope = %q, want unchanged sentinel value", data.ProfileScope.ValueString())
		}
	})

	t.Run("unmapped wire value", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{
			ProfileScope: types.StringValue("sentinel"),
		}
		g := &sdk.GeneralPayloadV4Entity{
			Name: "Linux Profile",
			// 99 is not a known ProfileScope wire value (1=Production,
			// 2=Staging, 3=Both per internal-task); readGeneralV4IntoState should
			// leave the prior state value untouched rather than guess.
			ProfileScope: sdk.IntPtr(99),
		}

		readGeneralV4IntoState(context.Background(), data, g)

		if data.ProfileScope.ValueString() != "sentinel" {
			t.Errorf("ProfileScope = %q, want unchanged sentinel value", data.ProfileScope.ValueString())
		}
	})

	t.Run("wire value 2 maps to Staging", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{
			ProfileScope: types.StringValue("sentinel"),
		}
		g := &sdk.GeneralPayloadV4Entity{
			Name:         "Linux Profile",
			ProfileScope: sdk.IntPtr(2),
		}

		readGeneralV4IntoState(context.Background(), data, g)

		if data.ProfileScope.ValueString() != "Staging" {
			t.Errorf("ProfileScope = %q, want %q", data.ProfileScope.ValueString(), "Staging")
		}
	})

	t.Run("wire value 3 maps to Both", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{
			ProfileScope: types.StringValue("sentinel"),
		}
		g := &sdk.GeneralPayloadV4Entity{
			Name:         "Linux Profile",
			ProfileScope: sdk.IntPtr(3),
		}

		readGeneralV4IntoState(context.Background(), data, g)

		if data.ProfileScope.ValueString() != "Both" {
			t.Errorf("ProfileScope = %q, want %q", data.ProfileScope.ValueString(), "Both")
		}
	})
}

// M2: unmapped/unknown Linux V4 profile_scope wire values (outside the known
// 1/2/3 mapping) must not panic and must leave the prior value untouched.
// This package has no log-capture harness, so these assert the no-panic /
// sensible-fallback behavior; the tflog.Warn call itself is confirmed by
// code inspection (internal/profile/state/read_mappers.go).
func TestReadGeneralV4IntoState_UnmappedWireValues_WarnAndFallBack(t *testing.T) {
	t.Parallel()

	for _, wire := range []int{0, 4} {
		t.Run("wire value", func(t *testing.T) {
			t.Parallel()
			data := &profilemodels.ProfileResourceModel{
				ProfileScope: types.StringValue("sentinel"),
			}
			g := &sdk.GeneralPayloadV4Entity{
				Name:         "Linux Profile",
				ProfileScope: sdk.IntPtr(wire),
			}

			readGeneralV4IntoState(context.Background(), data, g)

			if data.ProfileScope.ValueString() != "sentinel" {
				t.Errorf("wire=%d: ProfileScope = %q, want unchanged sentinel value", wire, data.ProfileScope.ValueString())
			}
		})
	}
}

// --- internal-task B1: existing casing must be preserved on a case-insensitive
// server-echo match, and the server's value must win on genuine drift. This
// is the fix that replaces the removed (broken) normalizeEnumCase plan
// modifier. These fail against the OLD readGeneralV2IntoState/
// readGeneralV4IntoState, which always took the server's value verbatim
// (would produce "Auto"/"Production" instead of the preserved "auto" in the
// preserved-casing sub-tests below).

func TestReadGeneralV2IntoState_AssignmentType_CasePreservation(t *testing.T) {
	t.Parallel()

	t.Run("existing case-insensitively matches server: existing preserved", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{AssignmentType: types.StringValue("auto")}
		g := &sdk.GeneralPayloadV2Entity{AssignmentType: "Auto"}

		readGeneralV2IntoState(data, g)

		if got := data.AssignmentType.ValueString(); got != "auto" {
			t.Errorf("AssignmentType = %q, want existing %q preserved", got, "auto")
		}
	})

	t.Run("existing genuinely differs: server value wins (drift)", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{AssignmentType: types.StringValue("Optional")}
		g := &sdk.GeneralPayloadV2Entity{AssignmentType: "Auto"}

		readGeneralV2IntoState(data, g)

		if got := data.AssignmentType.ValueString(); got != "Auto" {
			t.Errorf("AssignmentType = %q, want server's %q (drift)", got, "Auto")
		}
	})

	t.Run("no existing value (import): server's canonical value used", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{}
		g := &sdk.GeneralPayloadV2Entity{AssignmentType: "Auto"}

		readGeneralV2IntoState(data, g)

		if got := data.AssignmentType.ValueString(); got != "Auto" {
			t.Errorf("AssignmentType = %q, want server's canonical %q (import)", got, "Auto")
		}
	})
}

// internal-ticket removed profile_scope's casing preservation (row #56 of the
// B16 audit: internal-task only covers AssignmentType, no source for
// ProfileScope). ProfileScope now always takes the server's literal value,
// even when the prior value case-insensitively matched it.
func TestReadGeneralV2IntoState_ProfileScope_AlwaysServerValue(t *testing.T) {
	t.Parallel()

	t.Run("existing case-insensitively matches server: server value wins anyway", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{ProfileScope: types.StringValue("production")}
		g := &sdk.GeneralPayloadV2Entity{ProfileScope: "Production"}

		readGeneralV2IntoState(data, g)

		if got := data.ProfileScope.ValueString(); got != "Production" {
			t.Errorf("ProfileScope = %q, want server's %q (no casing preservation)", got, "Production")
		}
	})

	t.Run("existing genuinely differs: server value wins (drift)", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{ProfileScope: types.StringValue("Staging")}
		g := &sdk.GeneralPayloadV2Entity{ProfileScope: "Production"}

		readGeneralV2IntoState(data, g)

		if got := data.ProfileScope.ValueString(); got != "Production" {
			t.Errorf("ProfileScope = %q, want server's %q (drift)", got, "Production")
		}
	})

	t.Run("no existing value (import): server's canonical value used", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{}
		g := &sdk.GeneralPayloadV2Entity{ProfileScope: "Production"}

		readGeneralV2IntoState(data, g)

		if got := data.ProfileScope.ValueString(); got != "Production" {
			t.Errorf("ProfileScope = %q, want server's canonical %q (import)", got, "Production")
		}
	})
}

func TestReadGeneralV4IntoState_AssignmentType_CasePreservation(t *testing.T) {
	t.Parallel()

	t.Run("existing case-insensitively matches server: existing preserved", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{AssignmentType: types.StringValue("auto")}
		g := &sdk.GeneralPayloadV4Entity{AssignmentType: "Auto"}

		readGeneralV4IntoState(context.Background(), data, g)

		if got := data.AssignmentType.ValueString(); got != "auto" {
			t.Errorf("AssignmentType = %q, want existing %q preserved", got, "auto")
		}
	})

	t.Run("existing genuinely differs: server value wins (drift)", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{AssignmentType: types.StringValue("Optional")}
		g := &sdk.GeneralPayloadV4Entity{AssignmentType: "Auto"}

		readGeneralV4IntoState(context.Background(), data, g)

		if got := data.AssignmentType.ValueString(); got != "Auto" {
			t.Errorf("AssignmentType = %q, want server's %q (drift)", got, "Auto")
		}
	})

	t.Run("no existing value (import): server's canonical value used", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{}
		g := &sdk.GeneralPayloadV4Entity{AssignmentType: "Auto"}

		readGeneralV4IntoState(context.Background(), data, g)

		if got := data.AssignmentType.ValueString(); got != "Auto" {
			t.Errorf("AssignmentType = %q, want server's canonical %q (import)", got, "Auto")
		}
	})
}

func TestReadGeneralV4IntoState_ProfileScope_CasePreservation(t *testing.T) {
	t.Parallel()

	t.Run("existing case-insensitively matches server: existing preserved", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{ProfileScope: types.StringValue("production")}
		g := &sdk.GeneralPayloadV4Entity{ProfileScope: sdk.IntPtr(1)} // wire 1 -> "Production"

		readGeneralV4IntoState(context.Background(), data, g)

		if got := data.ProfileScope.ValueString(); got != "production" {
			t.Errorf("ProfileScope = %q, want existing %q preserved", got, "production")
		}
	})

	t.Run("existing genuinely differs: server value wins (drift)", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{ProfileScope: types.StringValue("Optional")}
		g := &sdk.GeneralPayloadV4Entity{ProfileScope: sdk.IntPtr(1)} // wire 1 -> "Production"

		readGeneralV4IntoState(context.Background(), data, g)

		if got := data.ProfileScope.ValueString(); got != "Production" {
			t.Errorf("ProfileScope = %q, want server's %q (drift)", got, "Production")
		}
	})

	t.Run("no existing value (import): server's canonical value used", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{}
		g := &sdk.GeneralPayloadV4Entity{ProfileScope: sdk.IntPtr(1)} // wire 1 -> "Production"

		readGeneralV4IntoState(context.Background(), data, g)

		if got := data.ProfileScope.ValueString(); got != "Production" {
			t.Errorf("ProfileScope = %q, want server's canonical %q (import)", got, "Production")
		}
	})
}

// --- Description Optional+Computed tests (internal-task; behavior removed internal-ticket) ---
//
// internal-ticket removed setDescriptionFromAPI's prior-state-dependent
// null/"" disambiguation (row #57 of the B16 audit: never observed live, no
// source). UEM's Description field is a non-pointer string with omitempty,
// so "the field was omitted" and "the field was sent as an explicit empty
// string" are indistinguishable on the wire; both now store as "" in state
// regardless of what was there before the call.

func TestSetDescriptionFromAPI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		apiValue   string
		priorValue types.String
		want       types.String
	}{
		{
			name:       "API empty, prior null -> empty string (no prior-state disambiguation)",
			apiValue:   "",
			priorValue: types.StringNull(),
			want:       types.StringValue(""),
		},
		{
			name:       "API empty, prior explicit empty string -> stays empty string",
			apiValue:   "",
			priorValue: types.StringValue(""),
			want:       types.StringValue(""),
		},
		{
			name:       "API empty, prior non-empty -> empty string (server value as-is)",
			apiValue:   "",
			priorValue: types.StringValue("Live Description"),
			want:       types.StringValue(""),
		},
		{
			name:       "API non-empty always wins over a null prior",
			apiValue:   "x",
			priorValue: types.StringNull(),
			want:       types.StringValue("x"),
		},
		{
			name:       "API non-empty always wins over a non-empty prior",
			apiValue:   "x",
			priorValue: types.StringValue("Live Description"),
			want:       types.StringValue("x"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data := &profilemodels.ProfileResourceModel{Description: tc.priorValue}
			setDescriptionFromAPI(data, tc.apiValue)
			if !data.Description.Equal(tc.want) {
				t.Errorf("Description = %v, want %v", data.Description, tc.want)
			}
		})
	}
}

func TestReadGeneralV2IntoState_Description(t *testing.T) {
	t.Parallel()

	t.Run("API empty, prior null -> empty string (no null defaulting)", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{}
		g := &sdk.GeneralPayloadV2Entity{Name: "P", Description: ""}
		readGeneralV2IntoState(data, g)
		if data.Description.IsNull() || data.Description.ValueString() != "" {
			t.Errorf("Description = %v, want empty string", data.Description)
		}
	})

	t.Run("API empty, prior explicit empty string -> stays empty string", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{Description: types.StringValue("")}
		g := &sdk.GeneralPayloadV2Entity{Name: "P", Description: ""}
		readGeneralV2IntoState(data, g)
		if data.Description.IsNull() || data.Description.ValueString() != "" {
			t.Errorf("Description = %v, want explicit empty string", data.Description)
		}
	})

	t.Run("API non-empty always wins", func(t *testing.T) {
		t.Parallel()
		data := &profilemodels.ProfileResourceModel{Description: types.StringNull()}
		g := &sdk.GeneralPayloadV2Entity{Name: "P", Description: "x"}
		readGeneralV2IntoState(data, g)
		if data.Description.ValueString() != "x" {
			t.Errorf("Description = %v, want %q", data.Description, "x")
		}
	})
}

func TestMapAppleOsXGatekeeper_AllFieldsSet(t *testing.T) {
	t.Parallel()

	g := &sdk.MacOsGatekeeperPayloadV2Entity{
		AllowAutoUnlock:              sdk.BoolPtr(false),
		AllowFingerprintForUnlock:    sdk.BoolPtr(true),
		AllowHandoff:                 sdk.BoolPtr(false),
		AllowScreenCapture:           sdk.BoolPtr(true),
		EnableAppSoftwareUpdateDelay: sdk.BoolPtr(true),
		EnableSoftwareUpdateDelay:    sdk.BoolPtr(true),
		EnforcedSoftwareUpdateDelay:  sdk.IntPtr(60),
	}

	got := mapAppleOsXGatekeeper(g)
	if got == nil {
		t.Fatal("expected non-nil GatekeeperModel")
	}
	if got.AllowAutoUnlock.ValueBool() != false {
		t.Errorf("AllowAutoUnlock = %v, want false", got.AllowAutoUnlock)
	}
	if got.AllowFingerprintForUnlock.ValueBool() != true {
		t.Errorf("AllowFingerprintForUnlock = %v, want true", got.AllowFingerprintForUnlock)
	}
	if got.AllowHandoff.ValueBool() != false {
		t.Errorf("AllowHandoff = %v, want false", got.AllowHandoff)
	}
	if got.AllowScreenCapture.ValueBool() != true {
		t.Errorf("AllowScreenCapture = %v, want true", got.AllowScreenCapture)
	}
	if got.EnableAppSoftwareUpdateDelay.ValueBool() != true {
		t.Errorf("EnableAppSoftwareUpdateDelay = %v, want true", got.EnableAppSoftwareUpdateDelay)
	}
	if got.EnableSoftwareUpdateDelay.ValueBool() != true {
		t.Errorf("EnableSoftwareUpdateDelay = %v, want true", got.EnableSoftwareUpdateDelay)
	}
	if got.EnforcedSoftwareUpdateDelay.ValueInt64() != 60 {
		t.Errorf("EnforcedSoftwareUpdateDelay = %d, want 60", got.EnforcedSoftwareUpdateDelay.ValueInt64())
	}
}

func TestMapAppleOsXGatekeeper_NilOrOmittedFields(t *testing.T) {
	t.Parallel()

	t.Run("nil entity returns nil model", func(t *testing.T) {
		t.Parallel()
		if got := mapAppleOsXGatekeeper(nil); got != nil {
			t.Errorf("expected nil GatekeeperModel for nil entity, got %+v", got)
		}
	})

	// internal-ticket removed the hasContent gate (row #63 of the B16 audit): a
	// non-nil entity with every field unset now returns a non-nil model with
	// every field null, matching every other block's nil-pointer-only gate.
	t.Run("entity with no fields set returns non-nil model with null fields", func(t *testing.T) {
		t.Parallel()
		g := &sdk.MacOsGatekeeperPayloadV2Entity{}
		got := mapAppleOsXGatekeeper(g)
		if got == nil {
			t.Fatal("expected non-nil GatekeeperModel for a non-nil (but empty) entity")
		}
		if !got.AllowAutoUnlock.IsNull() || !got.AllowFingerprintForUnlock.IsNull() ||
			!got.AllowHandoff.IsNull() || !got.AllowScreenCapture.IsNull() ||
			!got.EnableAppSoftwareUpdateDelay.IsNull() || !got.EnableSoftwareUpdateDelay.IsNull() ||
			!got.EnforcedSoftwareUpdateDelay.IsNull() {
			t.Errorf("expected every field null, got %+v", got)
		}
	})

	t.Run("one field set, rest nil -> unset fields map to null", func(t *testing.T) {
		t.Parallel()
		g := &sdk.MacOsGatekeeperPayloadV2Entity{
			AllowScreenCapture: sdk.BoolPtr(false),
		}
		got := mapAppleOsXGatekeeper(g)
		if got == nil {
			t.Fatal("expected non-nil GatekeeperModel")
		}
		if got.AllowScreenCapture.ValueBool() != false {
			t.Errorf("AllowScreenCapture = %v, want false", got.AllowScreenCapture)
		}
		if !got.AllowAutoUnlock.IsNull() {
			t.Errorf("AllowAutoUnlock = %v, want null", got.AllowAutoUnlock)
		}
		if !got.AllowFingerprintForUnlock.IsNull() {
			t.Errorf("AllowFingerprintForUnlock = %v, want null", got.AllowFingerprintForUnlock)
		}
		if !got.AllowHandoff.IsNull() {
			t.Errorf("AllowHandoff = %v, want null", got.AllowHandoff)
		}
		if !got.EnableAppSoftwareUpdateDelay.IsNull() {
			t.Errorf("EnableAppSoftwareUpdateDelay = %v, want null", got.EnableAppSoftwareUpdateDelay)
		}
		if !got.EnableSoftwareUpdateDelay.IsNull() {
			t.Errorf("EnableSoftwareUpdateDelay = %v, want null", got.EnableSoftwareUpdateDelay)
		}
		if !got.EnforcedSoftwareUpdateDelay.IsNull() {
			t.Errorf("EnforcedSoftwareUpdateDelay = %v, want null", got.EnforcedSoftwareUpdateDelay)
		}
	})
}
