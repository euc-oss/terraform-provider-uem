package state

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// Live-shaped KernelExtension and CustomAttributes reads (placeholder
// values) map field for field into state.
func TestReadAppleOsX_KernelExtensionAndCustomAttributes(t *testing.T) {
	data := &ProfileResourceModel{}
	readAppleOsXIntoState(context.Background(), data, &sdk.AppleOsXDeviceProfileEntityV2{
		KernelExtension: &sdk.MacOsKernelExtensionPayloadV2Entity{
			AllowUserOverrides:      sdk.BoolPtr(true),
			AllowedTeamIdentifiers:  []string{"TEAM1"},
			AllowedKernelExtensions: []sdk.MacOsAllowedKernelExtensionsEntityV2{{TeamIdentifier: "TEAM1", BundleIdentifier: "com.example"}},
		},
		CustomAttributes: []sdk.MacOsCustomAttributePayloadV2Model{
			{AttributeName: "C1", AttributeScript: "ZWNobw==", Schedule: sdk.IntPtr(0), Events: []string{"login"}},
		},
	})
	k := data.KernelExtension
	if k == nil || !k.AllowUserOverrides.ValueBool() || len(k.AllowedTeamIdentifiers.Elements()) != 1 ||
		len(k.AllowedKernelExtensions) != 1 || k.AllowedKernelExtensions[0].BundleIdentifier.ValueString() != "com.example" {
		t.Errorf("kernel_extension = %+v", k)
	}
	if len(data.CustomAttributes) != 1 {
		t.Fatalf("custom_attributes = %+v", data.CustomAttributes)
	}
	c := data.CustomAttributes[0]
	if c.AttributeName.ValueString() != "C1" || c.AttributeScript.ValueString() != "ZWNobw==" || c.Schedule.IsNull() || c.Schedule.ValueInt64() != 0 {
		t.Errorf("custom_attributes[0] = %+v", c)
	}
	readAppleOsXIntoState(context.Background(), data, &sdk.AppleOsXDeviceProfileEntityV2{})
	if data.KernelExtension != nil || data.CustomAttributes != nil {
		t.Error("absent payloads must be null")
	}
}
