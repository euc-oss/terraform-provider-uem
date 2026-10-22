package platform

import (
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
)

// Builders for the macOS CustomAttributes payload (F13), generated field for field from the SDK structs.

func buildCustomAttribute(it *profilemodels.CustomAttributeModel) sdk.MacOsCustomAttributePayloadV2Model {
	var out sdk.MacOsCustomAttributePayloadV2Model
	setStringIfKnown(&out.AttributeName, it.AttributeName)
	setStringIfKnown(&out.AttributeScript, it.AttributeScript)
	out.Events = stringSliceFromTFList(it.Events)
	if !it.Schedule.IsNull() && !it.Schedule.IsUnknown() {
		out.Schedule = sdk.IntPtr(int(it.Schedule.ValueInt64()))
	}
	return out
}

func buildCustomAttributeList(items []profilemodels.CustomAttributeModel) []sdk.MacOsCustomAttributePayloadV2Model {
	if len(items) == 0 {
		return nil
	}
	result := make([]sdk.MacOsCustomAttributePayloadV2Model, 0, len(items))
	for i := range items {
		result = append(result, buildCustomAttribute(&items[i]))
	}
	return result
}
