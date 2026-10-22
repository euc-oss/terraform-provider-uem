package state

import (
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
)

// Read mappers for the macOS KernelExtension payload (F13): stored straight from what UEM returns.

func mapKernelExtension(it *sdk.MacOsKernelExtensionPayloadV2Entity) profilemodels.KernelExtensionModel {
	return profilemodels.KernelExtensionModel{
		AllowUserOverrides:      boolPtrToTF(it.AllowUserOverrides),
		AllowedKernelExtensions: mapKernelExtensionAllowedKernelExtensionsList(it.AllowedKernelExtensions),
		AllowedTeamIdentifiers:  stringSliceToTFList(it.AllowedTeamIdentifiers),
	}
}

func mapKernelExtensionPtr(it *sdk.MacOsKernelExtensionPayloadV2Entity) *profilemodels.KernelExtensionModel {
	if it == nil {
		return nil
	}
	m := mapKernelExtension(it)
	return &m
}
func mapKernelExtensionAllowedKernelExtensions(it *sdk.MacOsAllowedKernelExtensionsEntityV2) profilemodels.KernelExtensionAllowedKernelExtensionsModel {
	return profilemodels.KernelExtensionAllowedKernelExtensionsModel{
		BundleIdentifier: stringToTF(it.BundleIdentifier),
		TeamIdentifier:   stringToTF(it.TeamIdentifier),
	}
}

func mapKernelExtensionAllowedKernelExtensionsList(items []sdk.MacOsAllowedKernelExtensionsEntityV2) []profilemodels.KernelExtensionAllowedKernelExtensionsModel {
	if len(items) == 0 {
		return nil
	}
	result := make([]profilemodels.KernelExtensionAllowedKernelExtensionsModel, 0, len(items))
	for i := range items {
		result = append(result, mapKernelExtensionAllowedKernelExtensions(&items[i]))
	}
	return result
}
