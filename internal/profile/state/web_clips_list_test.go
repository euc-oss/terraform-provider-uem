package state

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// A live-shaped Web Clip read (placeholder values) maps field for field;
// false is a value, an empty or missing value is null.
func TestMapAppleOsXWebClipsList(t *testing.T) {
	got := mapAppleOsXWebClipsList([]sdk.MacOsWebClipsPayloadV2Entity{
		{Label: "Portal", URL: "https://example.invalid", ShowInAppCatalog: sdk.BoolPtr(true), Icon: sdk.IntPtr(3001)},
		{Label: "Other", ShowInAppCatalog: sdk.BoolPtr(false)},
	})
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	a, b := got[0], got[1]
	if a.Label.ValueString() != "Portal" || a.URL.ValueString() != "https://example.invalid" || !a.ShowInAppCatalog.ValueBool() || a.Icon.ValueInt64() != 3001 {
		t.Errorf("entry 0 = %+v", a)
	}
	if !b.URL.IsNull() || !b.Icon.IsNull() || b.ShowInAppCatalog.IsNull() || b.ShowInAppCatalog.ValueBool() {
		t.Errorf("entry 1 = %+v", b)
	}
	if mapAppleOsXWebClipsList(nil) != nil {
		t.Error("no Web Clips must map to null")
	}
	data := &ProfileResourceModel{}
	readAppleOsXIntoState(context.Background(), data, &sdk.AppleOsXDeviceProfileEntityV2{WebClipsList: []sdk.MacOsWebClipsPayloadV2Entity{{Label: "L"}}})
	if len(data.WebClipsList) != 1 || data.WebClipsList[0].Label.ValueString() != "L" {
		t.Errorf("web_clips_list = %+v", data.WebClipsList)
	}
}
