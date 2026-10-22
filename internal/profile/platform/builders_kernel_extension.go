package platform

import (
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
)

// Builders for the macOS KernelExtension payload (F13), generated field for field from the SDK structs.

func buildKernelExtension(it *profilemodels.KernelExtensionModel) sdk.MacOsKernelExtensionPayloadV2Entity {
	var out sdk.MacOsKernelExtensionPayloadV2Entity
	out.AllowUserOverrides = boolPtrFromTF(it.AllowUserOverrides)
	out.AllowedKernelExtensions = buildKernelExtensionAllowedKernelExtensionsList(it.AllowedKernelExtensions)
	out.AllowedTeamIdentifiers = stringSliceFromTFList(it.AllowedTeamIdentifiers)
	return out
}

func buildKernelExtensionAllowedKernelExtensions(it *profilemodels.KernelExtensionAllowedKernelExtensionsModel) sdk.MacOsAllowedKernelExtensionsEntityV2 {
	var out sdk.MacOsAllowedKernelExtensionsEntityV2
	setStringIfKnown(&out.BundleIdentifier, it.BundleIdentifier)
	setStringIfKnown(&out.TeamIdentifier, it.TeamIdentifier)
	return out
}

func buildKernelExtensionAllowedKernelExtensionsList(items []profilemodels.KernelExtensionAllowedKernelExtensionsModel) []sdk.MacOsAllowedKernelExtensionsEntityV2 {
	if len(items) == 0 {
		return nil
	}
	result := make([]sdk.MacOsAllowedKernelExtensionsEntityV2, 0, len(items))
	for i := range items {
		result = append(result, buildKernelExtensionAllowedKernelExtensions(&items[i]))
	}
	return result
}
