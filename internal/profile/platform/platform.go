package platform

import (
	sdk "github.com/euc-oss/terraform-sdk-uem"
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
