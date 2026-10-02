package provider

import (
	"context"
	"testing"

	applicationdatasource "github.com/euc-oss/terraform-provider-uem/internal/application/datasource"
	purchasedappdatasource "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app"
	scriptsdatasource "github.com/euc-oss/terraform-provider-uem/internal/scripts"
	sensorsdatasource "github.com/euc-oss/terraform-provider-uem/internal/sensors"
	updatesdatasource "github.com/euc-oss/terraform-provider-uem/internal/updates"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
)

// TestGeneratedDataSources_EndToEndProviderWiring proves that this epic's 4
// stage-iii data sources — scripts, sensors, and mac_applications (generator-
// produced, per internal/gends/generate.go's conceptDir) plus
// update_deployments (hand-written, like the pre-existing profile/
// smartgroup/application concepts — see conceptDir's "update" entry for why
// the two-level catalog+deployments fan-out it needs isn't generator-
// expressible) — are correctly wired through the real, registered provider,
// not just individually unit-tested in their own packages. It exercises the
// exact code path terraform-core would invoke: construct the provider via
// the public New() entry point, pull each constructor out of the real
// DataSources() list, and drive the real Metadata()/Schema() calls on the
// instances it produces.
func TestGeneratedDataSources_EndToEndProviderWiring(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// 1. Construct the real provider through the same entry point the
	// Terraform plugin SDK/CLI invokes.
	p := New("test")()

	// 2. Confirm DataSources() returns exactly 8 constructors: the 3
	// pre-existing hand-written data sources (smartgroup/profile/
	// application, already covered by TestProviderDataSources) plus the 4
	// new stage-iii ones this test drills into below — scripts/sensors/
	// mac_applications (generator-produced) and update_deployments
	// (hand-written) — plus purchased_applications (hand-written, internal-task)
	// and organization_groups (hand-written, internal-task).
	ctors := p.DataSources(ctx)
	if len(ctors) != 9 {
		t.Fatalf("expected 9 data source constructors, got %d", len(ctors))
	}
	registered := map[string]bool{}
	for _, ctor := range ctors {
		var mr datasource.MetadataResponse
		ctor().Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "uem"}, &mr)
		registered[mr.TypeName] = true
	}

	type expectation struct {
		name             string
		ctor             func() datasource.DataSource
		expectedTypeName string
		requiredAttrs    []string
		optionalAttrs    []string
	}

	cases := []expectation{
		{
			name:             "scripts",
			ctor:             scriptsdatasource.NewScriptDataSource,
			expectedTypeName: "uem_scripts",
			requiredAttrs:    []string{"organization_group_uuid"},
			optionalAttrs:    []string{"name"},
		},
		{
			name:             "sensors",
			ctor:             sensorsdatasource.NewSensorDataSource,
			expectedTypeName: "uem_sensors",
			requiredAttrs:    []string{"organization_group_uuid"},
			optionalAttrs:    []string{"name"},
		},
		{
			name:             "update_deployments",
			ctor:             updatesdatasource.NewUpdateDataSource,
			expectedTypeName: "uem_update_deployments",
			requiredAttrs:    []string{"organization_group_uuid"},
			optionalAttrs:    []string{"platform", "update_name"},
		},
		{
			name:             "mac_applications",
			ctor:             applicationdatasource.NewMacApplicationDataSource,
			expectedTypeName: "uem_mac_applications",
			optionalAttrs:    []string{"organization_group_uuid"},
		},
		{
			name:             "purchased_applications",
			ctor:             purchasedappdatasource.NewPurchasedApplicationsDataSource,
			expectedTypeName: "uem_purchased_applications",
			requiredAttrs:    []string{"organization_group_uuid"},
		},
	}

	for _, tc := range cases {

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ds := tc.ctor()
			if ds == nil {
				t.Fatalf("constructor for %s returned a nil datasource.DataSource", tc.name)
			}

			// 3. Drive the real Metadata() call the way terraform-core does,
			// with the provider's actual registered type name prefix.
			metaReq := datasource.MetadataRequest{ProviderTypeName: "uem"}
			var metaResp datasource.MetadataResponse
			ds.Metadata(ctx, metaReq, &metaResp)

			if metaResp.TypeName != tc.expectedTypeName {
				t.Errorf("Metadata: expected TypeName %q, got %q", tc.expectedTypeName, metaResp.TypeName)
			}
			if !registered[tc.expectedTypeName] {
				t.Errorf("%s is not registered in the provider's DataSources() list", tc.expectedTypeName)
			}

			// 4. Drive the real Schema() call and confirm it is well-formed.
			schemaReq := datasource.SchemaRequest{}
			var schemaResp datasource.SchemaResponse
			ds.Schema(ctx, schemaReq, &schemaResp)

			if schemaResp.Diagnostics.HasError() {
				t.Fatalf("Schema() produced diagnostics errors: %s", schemaResp.Diagnostics)
			}

			attrs := schemaResp.Schema.Attributes
			if len(attrs) == 0 {
				t.Fatalf("Schema() returned zero attributes for %s", tc.name)
			}

			for _, attrName := range tc.requiredAttrs {
				attr, ok := attrs[attrName]
				if !ok {
					t.Errorf("expected attribute %q to be present in %s schema", attrName, tc.name)
					continue
				}
				if !attr.IsRequired() {
					t.Errorf("expected attribute %q in %s schema to be Required", attrName, tc.name)
				}
			}

			for _, attrName := range tc.optionalAttrs {
				attr, ok := attrs[attrName]
				if !ok {
					t.Errorf("expected attribute %q to be present in %s schema", attrName, tc.name)
					continue
				}
				if !attr.IsOptional() {
					t.Errorf("expected attribute %q in %s schema to be Optional", attrName, tc.name)
				}
			}
		})
	}
}

// compile-time assertion that the provider's public entry point has the
// exact shape this test (and terraform-core) relies on.
var _ func(string) func() fwprovider.Provider = New
