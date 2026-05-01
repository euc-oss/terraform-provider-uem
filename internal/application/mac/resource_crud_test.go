package macapplication

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestMacConfigureUnsupportedType(t *testing.T) {
	r := &macapplicationResource{}
	req := resource.ConfigureRequest{ProviderData: 42}
	resp := &resource.ConfigureResponse{}

	r.Configure(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error for unsupported provider data type")
	}
}

func TestMacConfigureWithSDKClient(t *testing.T) {
	r := &macapplicationResource{}
	req := resource.ConfigureRequest{ProviderData: &sdk.Client{}}
	resp := &resource.ConfigureResponse{}

	r.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("did not expect diagnostics error: %v", resp.Diagnostics)
	}
	if r.client == nil {
		t.Fatal("expected client to be set")
	}
	if r.newBlobResourceService == nil || r.newMacAppResourceService == nil || r.newInternalAppResourceService == nil {
		t.Fatal("expected all service factories to be set")
	}
}
