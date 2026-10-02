package platform

import (
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

const PlatformLinuxUser = "Linux"

var SupportedImportPlatforms = []string{
	sdk.PlatformAndroid,
	sdk.PlatformAppleiOS,
	sdk.PlatformAppleOsX,
	sdk.PlatformWindows10,
	sdk.PlatformWindowsRugged,
	PlatformLinuxUser,
}

var supportedImportPlatformSet = map[string]struct{}{
	sdk.PlatformAndroid:       {},
	sdk.PlatformAppleiOS:      {},
	sdk.PlatformAppleOsX:      {},
	sdk.PlatformWindows10:     {},
	sdk.PlatformWindowsRugged: {},
	PlatformLinuxUser:         {},
}

func IsValidImportPlatform(platform string) bool {
	_, ok := supportedImportPlatformSet[platform]
	return ok
}

func ForSDK(platform string) string {
	if platform == PlatformLinuxUser {
		return sdk.PlatformLinux
	}
	return platform
}

// ShortToLongPlatform maps the SHORT-FORM platform values returned by the
// UEM API's GET /api/mdm/profiles/search v2 endpoint to the LONG-FORM vocab
// in SupportedImportPlatforms above, so a value read from that endpoint can
// be fed straight into `terraform import` (validated by IsValidImportPlatform)
// without a mismatch. See internal-ticket.
//
// Only the 4 pairs below are resolved with ground truth; Windows_Rugged and
// Linux are intentionally NOT mapped here (pending maintainer decision) —
// unmapped values fall through unchanged via NormalizeAPIPlatform.
var ShortToLongPlatform = map[string]string{
	// terraform-sdk-uem testdata/mock-responses/profiles/profiles_search_v2.json —
	// live-captured GET /api/mdm/profiles/search v2 against UEM 26.2.0, 2026-07-30;
	// ProfileList[0].Platform="Apple"
	"Apple": sdk.PlatformAppleiOS, // "Apple iOS"

	// same fixture, ProfileList[1].Platform="WinRT"
	"WinRT": sdk.PlatformWindows10, // "Windows 10"

	// identity; SDK sdk.go:1247 facade comment + get_profile_12345.json General.Platform="Android"
	"Android": sdk.PlatformAndroid, // "Android"

	// identity; SDK sdk.go:1249 + profiles_appleosx_get_after_create.json metadata
	"AppleOsX": sdk.PlatformAppleOsX, // "AppleOsX"
}

// NormalizeAPIPlatform translates a short-form platform value (as returned by
// GET /api/mdm/profiles/search v2) to its long-form equivalent from
// SupportedImportPlatforms, using ShortToLongPlatform. Any value not present
// in the map (including the unresolved Windows_Rugged and Linux platforms)
// is returned UNCHANGED — this is a deliberate identity fallback that
// preserves today's behavior for unmapped/unknown values with zero
// regression risk. See internal-ticket.
func NormalizeAPIPlatform(shortForm string) string {
	if longForm, ok := ShortToLongPlatform[shortForm]; ok {
		return longForm
	}
	return shortForm
}
