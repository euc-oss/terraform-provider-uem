package assignment

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestScriptAssignmentConfigureUnsupportedType(t *testing.T) {
	r := &scriptAssignmentResource{}
	req := resource.ConfigureRequest{ProviderData: 42}
	resp := &resource.ConfigureResponse{}

	r.Configure(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error for unsupported provider data type")
	}
}

func TestScriptAssignmentConfigureWithSDKClient(t *testing.T) {
	r := &scriptAssignmentResource{}
	req := resource.ConfigureRequest{ProviderData: &sdk.Client{}}
	resp := &resource.ConfigureResponse{}

	r.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("did not expect diagnostics error: %v", resp.Diagnostics)
	}
	if r.client == nil {
		t.Fatal("expected client to be set")
	}
	if r.newScriptAssignmentService == nil {
		t.Fatal("expected script assignment service factory to be set")
	}
}
