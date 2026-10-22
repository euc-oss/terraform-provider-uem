package platform

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// TestNormalizeAPIPlatform_MapsShortFormToLongForm covers internal-ticket's 4
// resolved short->long pairs: the /api/mdm/profiles/search v2 endpoint emits
// short-form platform strings, but ImportState validates long-form.
func TestNormalizeAPIPlatform_MapsShortFormToLongForm(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		shortForm string
		wantLong  string
	}{
		{"Apple to Apple iOS", "Apple", sdk.PlatformAppleiOS},
		{"WinRT to Windows 10", "WinRT", sdk.PlatformWindows10},
		{"Android identity", "Android", sdk.PlatformAndroid},
		{"AppleOsX identity", "AppleOsX", sdk.PlatformAppleOsX},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeAPIPlatform(tc.shortForm)
			if got != tc.wantLong {
				t.Errorf("NormalizeAPIPlatform(%q) = %q, want %q", tc.shortForm, got, tc.wantLong)
			}
		})
	}
}

// TestNormalizeAPIPlatform_UnmappedValuesPassThroughUnchanged asserts the
// required identity fallback: any Platform value not in ShortToLongPlatform
// (including the unresolved Windows_Rugged and Linux platforms, which are
// deliberately NOT mapped pending a maintainer decision) must survive
// unchanged, preserving today's behavior with zero regression risk.
func TestNormalizeAPIPlatform_UnmappedValuesPassThroughUnchanged(t *testing.T) {
	t.Parallel()

	unmapped := []string{
		"Windows_Rugged",
		"Linux",
		"SomeFuturePlatform",
		"",
	}

	for _, shortForm := range unmapped {
		t.Run(shortForm, func(t *testing.T) {
			t.Parallel()
			got := NormalizeAPIPlatform(shortForm)
			if got != shortForm {
				t.Errorf("NormalizeAPIPlatform(%q) = %q, want unchanged %q", shortForm, got, shortForm)
			}
		})
	}
}

// TestNormalizeAPIPlatform_MappedOutputsAreValidImportPlatforms is the
// round-trip guarantee: every long-form value NormalizeAPIPlatform can
// produce from a mapped short-form input must be a member of
// SupportedImportPlatforms, so it passes IsValidImportPlatform / ImportState
// unchanged.
func TestNormalizeAPIPlatform_MappedOutputsAreValidImportPlatforms(t *testing.T) {
	t.Parallel()

	for shortForm, longForm := range ShortToLongPlatform {
		got := NormalizeAPIPlatform(shortForm)
		if got != longForm {
			t.Fatalf("NormalizeAPIPlatform(%q) = %q, want %q", shortForm, got, longForm)
		}
		if !IsValidImportPlatform(got) {
			t.Errorf("NormalizeAPIPlatform(%q) = %q, which is NOT a member of SupportedImportPlatforms", shortForm, got)
		}
	}
}
