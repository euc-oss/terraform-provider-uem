package state

import (
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
)

// Read mappers for the macOS CustomAttributes payload (F13): stored straight from what UEM returns.

func mapCustomAttribute(it *sdk.MacOsCustomAttributePayloadV2Model) profilemodels.CustomAttributeModel {
	return profilemodels.CustomAttributeModel{
		AttributeName:   stringToTF(it.AttributeName),
		AttributeScript: stringToTF(it.AttributeScript),
		Events:          stringSliceToTFList(it.Events),
		Schedule:        int64PtrToTF(it.Schedule),
	}
}

func mapCustomAttributeList(items []sdk.MacOsCustomAttributePayloadV2Model) []profilemodels.CustomAttributeModel {
	if len(items) == 0 {
		return nil
	}
	result := make([]profilemodels.CustomAttributeModel, 0, len(items))
	for i := range items {
		result = append(result, mapCustomAttribute(&items[i]))
	}
	return result
}
