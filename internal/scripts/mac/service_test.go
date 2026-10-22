package macscript

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

type testScriptService struct{}

func (s *testScriptService) CreateScriptAsync(ctx context.Context, OrganizationGroupUUID string, request *sdk.CreateScriptV1) (http.Header, error) {
	return nil, nil
}

func (s *testScriptService) GetScriptAsync(ctx context.Context, ScriptUUID string) (http.Header, *sdk.ScriptResourceV1, error) {
	return nil, nil, nil
}

func (s *testScriptService) ScriptBulkDeleteAsync(ctx context.Context, OrganizationGroupUUID string, request *[]string) (http.Header, *sdk.DeleteScriptResourceV1, error) {
	return nil, nil, nil
}

func (s *testScriptService) ReplaceScriptDefinitionAsync(ctx context.Context, ScriptUUID string, request *sdk.UpdateScriptV1) (http.Header, error) {
	return nil, nil
}

func (s *testScriptService) GetScriptsByOrganizationGroupAsync(ctx context.Context, OrganizationGroupUUID string, opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (http.Header, *sdk.ScriptsSearchResultV1, error) {
	return nil, nil, nil
}

func TestResourceTypeErrorDetail(t *testing.T) {
	detail := resourceTypeErrorDetail(123)
	if !strings.Contains(detail, "*resourceConfigData or *sdk.Client") {
		t.Fatalf("unexpected detail: %q", detail)
	}
	if !strings.Contains(detail, "int") {
		t.Fatalf("type not included in detail: %q", detail)
	}
}

func TestMacScriptAppService_NilClient(t *testing.T) {
	r := &macscriptResource{}

	svc, err := r.MacScriptAppService(context.Background())
	if err == nil {
		t.Fatal("expected error for nil client")
	}
	if svc != nil {
		t.Fatalf("expected nil service, got %#v", svc)
	}
	if !strings.Contains(err.Error(), "SDK client is nil") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMacScriptAppService_FactoryOnlyRunsOnce(t *testing.T) {
	client := &sdk.Client{}
	wantSvc := &testScriptService{}
	factoryCalls := 0

	r := &macscriptResource{
		client: client,
		newScriptServiceV1API: func(c *sdk.Client) ScriptServiceV1API {
			factoryCalls++
			if c != client {
				t.Fatalf("factory received unexpected client pointer")
			}
			return wantSvc
		},
	}

	svc1, err := r.MacScriptAppService(context.Background())
	if err != nil {
		t.Fatalf("unexpected first error: %v", err)
	}
	svc2, err := r.MacScriptAppService(context.Background())
	if err != nil {
		t.Fatalf("unexpected second error: %v", err)
	}

	if factoryCalls != 1 {
		t.Fatalf("factory called %d times, want 1", factoryCalls)
	}
	if svc1 != wantSvc || svc2 != wantSvc {
		t.Fatalf("service mismatch: got %p and %p want %p", svc1, svc2, wantSvc)
	}
}

func TestMetadata(t *testing.T) {
	r := &macscriptResource{}
	resp := &resource.MetadataResponse{}

	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "uem"}, resp)

	if resp.TypeName != "uem_mac_script" {
		t.Fatalf("TypeName mismatch: got %q", resp.TypeName)
	}
}

func TestIsNotFoundAPIError_FalseForGenericError(t *testing.T) {
	if isNotFoundAPIError(errors.New("boom")) {
		t.Fatal("expected false for generic error")
	}
}
