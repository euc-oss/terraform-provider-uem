package smartgroup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/euc-oss/terraform-sdk-uem/client"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// --- helpers ---

// createTestClient creates a *client.Client backed by the given httptest.Server.
func createTestClient(t *testing.T, handler http.HandlerFunc) (*client.Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)

	cfg := &client.Config{
		InstanceURL: server.URL,
		TenantCode:  "test-tenant",
		AuthMethod:  "basic",
		Username:    "test-user",
		Password:    "test-pass",
	}

	c, err := client.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	return c, server
}

// getDSSchema returns the smart groups data source schema.
func getDSSchema(t *testing.T) datasource.SchemaResponse {
	t.Helper()
	ds := &SmartGroupsDataSource{}
	var resp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	return resp
}

// createDSConfig builds a tfsdk.Config for the data source from the given attribute values.
func createDSConfig(t *testing.T, values map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	schemaResp := getDSSchema(t)
	ctx := context.Background()
	configType := schemaResp.Schema.Type().TerraformType(ctx)
	configValue := tftypes.NewValue(configType, values)
	return tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    configValue,
	}
}

// emptyDSState returns a tfsdk.State with all-null values for the data source schema.
func emptyDSState(t *testing.T) tfsdk.State {
	t.Helper()
	schemaResp := getDSSchema(t)
	ctx := context.Background()
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)
	objType, ok := schemaType.(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an Object")
	}
	values := make(map[string]tftypes.Value)
	for name, attrType := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}
	return tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaType, values),
	}
}

// smartGroupNestedType returns the tftypes.Object type for one smart_groups list element.
func smartGroupNestedType(t *testing.T) tftypes.Object {
	t.Helper()
	schemaResp := getDSSchema(t)
	ctx := context.Background()
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)
	objType, ok := schemaType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected schema type tftypes.Object, got %T", schemaType)
	}
	listType, ok := objType.AttributeTypes["smart_groups"].(tftypes.List)
	if !ok {
		t.Fatalf("expected smart_groups to be tftypes.List, got %T", objType.AttributeTypes["smart_groups"])
	}
	elem, ok := listType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatalf("expected list element tftypes.Object, got %T", listType.ElementType)
	}
	return elem
}

// --- Metadata ---

func TestSmartGroupsDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := &SmartGroupsDataSource{}
	var resp datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "uem"}, &resp)

	if resp.TypeName != "uem_smart_groups" {
		t.Errorf("expected TypeName 'uem_smart_groups', got '%s'", resp.TypeName)
	}
}

// --- Schema ---

func TestSmartGroupsDataSourceSchema(t *testing.T) {
	t.Parallel()
	schemaResp := getDSSchema(t)
	s := schemaResp.Schema

	for _, name := range []string{"name", "organization_group_id", "smart_groups"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing expected attribute: %s", name)
		}
	}
}

// --- Configure ---

func TestSmartGroupsDataSourceConfigure_NilProviderData(t *testing.T) {
	t.Parallel()
	ds := &SmartGroupsDataSource{}
	req := datasource.ConfigureRequest{ProviderData: nil}
	var resp datasource.ConfigureResponse
	ds.Configure(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatal("unexpected error for nil ProviderData")
	}
}

func TestSmartGroupsDataSourceConfigure_ValidClient(t *testing.T) {
	t.Parallel()
	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {})
	defer server.Close()

	ds := &SmartGroupsDataSource{}
	req := datasource.ConfigureRequest{ProviderData: c}
	var resp datasource.ConfigureResponse
	ds.Configure(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatal("unexpected error for valid client")
	}
	if ds.client != c {
		t.Error("expected client to be set")
	}
}

func TestSmartGroupsDataSourceConfigure_WrongType(t *testing.T) {
	t.Parallel()
	ds := &SmartGroupsDataSource{}
	req := datasource.ConfigureRequest{ProviderData: "not-a-client"}
	var resp datasource.ConfigureResponse
	ds.Configure(context.Background(), req, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for wrong ProviderData type")
	}
}

// --- Read: organization_group_id input handling ---

func TestRead_NonNumericOrgGroupID_IsIgnored(t *testing.T) {
	t.Parallel()

	var capturedURL string
	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		_ = json.NewEncoder(w).Encode(sdk.SmartGroupSearchResultV1{
			SmartGroups: nil,
			Total:       intPtr(0),
		})
	})
	defer server.Close()

	ds := &SmartGroupsDataSource{client: c}
	ctx := context.Background()

	config := createDSConfig(t, map[string]tftypes.Value{
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"organization_group_id": tftypes.NewValue(tftypes.String, "not-a-number"),
		"smart_groups":          tftypes.NewValue(tftypes.List{ElementType: smartGroupNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics.Errors())
	}

	// The non-numeric value should be silently ignored — no organizationgroupid query param.
	if containsParam(capturedURL, "organizationgroupid") {
		t.Errorf("expected organizationgroupid param to be absent for non-numeric input, URL was: %s", capturedURL)
	}
}

func TestRead_NumericOrgGroupID_IsSent(t *testing.T) {
	t.Parallel()

	var capturedURL string
	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		_ = json.NewEncoder(w).Encode(sdk.SmartGroupSearchResultV1{
			SmartGroups: nil,
			Total:       intPtr(0),
		})
	})
	defer server.Close()

	ds := &SmartGroupsDataSource{client: c}
	ctx := context.Background()

	config := createDSConfig(t, map[string]tftypes.Value{
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"organization_group_id": tftypes.NewValue(tftypes.String, "12345"),
		"smart_groups":          tftypes.NewValue(tftypes.List{ElementType: smartGroupNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics.Errors())
	}

	if !containsParam(capturedURL, "organizationgroupid") {
		t.Errorf("expected organizationgroupid param for numeric input, URL was: %s", capturedURL)
	}
}

// --- Read: pagination ---

func TestRead_Pagination_TotalNil_StopsAfterFirstPage(t *testing.T) {
	t.Parallel()

	requestCount := 0
	id1 := 1
	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		_ = json.NewEncoder(w).Encode(sdk.SmartGroupSearchResultV1{
			SmartGroups: []sdk.SmartGroupSearchModelV1{
				{SmartGroupID: &id1, Name: "Group-1", SmartGroupUUID: "uuid-1"},
			},
			Total: nil, // Total omitted
		})
	})
	defer server.Close()

	ds := &SmartGroupsDataSource{client: c}
	ctx := context.Background()

	config := createDSConfig(t, map[string]tftypes.Value{
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"organization_group_id": tftypes.NewValue(tftypes.String, nil),
		"smart_groups":          tftypes.NewValue(tftypes.List{ElementType: smartGroupNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics.Errors())
	}

	// When Total is nil, total defaults to 0, and len(allGroups) >= 0 is true
	// after the first page, so pagination should stop after exactly 1 request.
	if requestCount != 1 {
		t.Errorf("expected 1 request when Total is nil, got %d", requestCount)
	}
}

func TestRead_Pagination_MultiplePages(t *testing.T) {
	t.Parallel()

	totalGroups := 3
	requestCount := 0
	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		requestCount++

		var groups []sdk.SmartGroupSearchModelV1
		switch page {
		case 0:
			id1, id2 := 1, 2
			groups = []sdk.SmartGroupSearchModelV1{
				{SmartGroupID: &id1, Name: "Group-1", SmartGroupUUID: "uuid-1"},
				{SmartGroupID: &id2, Name: "Group-2", SmartGroupUUID: "uuid-2"},
			}
		case 1:
			id3 := 3
			groups = []sdk.SmartGroupSearchModelV1{
				{SmartGroupID: &id3, Name: "Group-3", SmartGroupUUID: "uuid-3"},
			}
		}

		_ = json.NewEncoder(w).Encode(sdk.SmartGroupSearchResultV1{
			SmartGroups: groups,
			Total:       &totalGroups,
		})
	})
	defer server.Close()

	ds := &SmartGroupsDataSource{client: c}
	ctx := context.Background()

	config := createDSConfig(t, map[string]tftypes.Value{
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"organization_group_id": tftypes.NewValue(tftypes.String, nil),
		"smart_groups":          tftypes.NewValue(tftypes.List{ElementType: smartGroupNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics.Errors())
	}

	if requestCount != 2 {
		t.Errorf("expected 2 paginated requests, got %d", requestCount)
	}

	// Verify all 3 groups were collected.
	var model SmartGroupsDataSourceModel
	diags := readResp.State.Get(ctx, &model)
	if diags.HasError() {
		t.Fatalf("failed to read state: %v", diags.Errors())
	}
	if len(model.SmartGroups) != 3 {
		t.Errorf("expected 3 smart groups, got %d", len(model.SmartGroups))
	}
}

func TestRead_EmptyResult(t *testing.T) {
	t.Parallel()

	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(sdk.SmartGroupSearchResultV1{
			SmartGroups: nil,
			Total:       intPtr(0),
		})
	})
	defer server.Close()

	ds := &SmartGroupsDataSource{client: c}
	ctx := context.Background()

	config := createDSConfig(t, map[string]tftypes.Value{
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"organization_group_id": tftypes.NewValue(tftypes.String, nil),
		"smart_groups":          tftypes.NewValue(tftypes.List{ElementType: smartGroupNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics.Errors())
	}

	var model SmartGroupsDataSourceModel
	diags := readResp.State.Get(ctx, &model)
	if diags.HasError() {
		t.Fatalf("failed to read state: %v", diags.Errors())
	}
	if model.SmartGroups == nil {
		// nil slice is acceptable for empty results
		return
	}
	if len(model.SmartGroups) != 0 {
		t.Errorf("expected 0 smart groups, got %d", len(model.SmartGroups))
	}
}

func TestRead_APIError_ReturnsDiagnostic(t *testing.T) {
	t.Parallel()

	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintln(w, `{"message": "server error"}`)
	})
	defer server.Close()

	ds := &SmartGroupsDataSource{client: c}
	ctx := context.Background()

	config := createDSConfig(t, map[string]tftypes.Value{
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"organization_group_id": tftypes.NewValue(tftypes.String, nil),
		"smart_groups":          tftypes.NewValue(tftypes.List{ElementType: smartGroupNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic for API failure")
	}
}

func TestRead_NameFilter_IsSent(t *testing.T) {
	t.Parallel()

	var capturedURL string
	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		_ = json.NewEncoder(w).Encode(sdk.SmartGroupSearchResultV1{
			SmartGroups: nil,
			Total:       intPtr(0),
		})
	})
	defer server.Close()

	ds := &SmartGroupsDataSource{client: c}
	ctx := context.Background()

	config := createDSConfig(t, map[string]tftypes.Value{
		"name":                  tftypes.NewValue(tftypes.String, "All Devices"),
		"organization_group_id": tftypes.NewValue(tftypes.String, nil),
		"smart_groups":          tftypes.NewValue(tftypes.List{ElementType: smartGroupNestedType(t)}, nil),
	})

	readReq := datasource.ReadRequest{Config: config}
	readResp := datasource.ReadResponse{State: emptyDSState(t)}

	ds.Read(ctx, readReq, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", readResp.Diagnostics.Errors())
	}

	if !containsParam(capturedURL, "name") {
		t.Errorf("expected name query param, URL was: %s", capturedURL)
	}
}

// --- helpers ---

func intPtr(v int) *int { return &v }

func containsParam(rawURL, param string) bool {
	// Simple check: the query string contains the param name followed by "=".
	return len(rawURL) > 0 && contains(rawURL, param+"=")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
