package macsensor

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// nameValidators returns the validators attached to the "name" attribute so
// tests can exercise them the same way the framework does during
// ValidateResourceConfig (i.e. at plan/validate time, before apply).
func nameValidators(t *testing.T) []validator.String {
	t.Helper()

	r := &macsensorResource{}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)

	attr, ok := schemaResp.Schema.Attributes["name"]
	if !ok {
		t.Fatal("schema has no \"name\" attribute")
	}

	strAttr, ok := attr.(schema.StringAttribute)
	if !ok {
		t.Fatalf("\"name\" attribute is not a schema.StringAttribute, got %T", attr)
	}

	return strAttr.Validators
}

// runNameValidators simulates what terraform-plugin-framework does during
// ValidateResourceConfig: it invokes every configured validator.String
// against the candidate value and aggregates diagnostics. This happens at
// plan/validate time, well before any Create/apply-time logic runs.
func runNameValidators(t *testing.T, name string) []validator.StringResponse {
	t.Helper()

	validators := nameValidators(t)
	responses := make([]validator.StringResponse, 0, len(validators))

	for _, v := range validators {
		req := validator.StringRequest{
			Path:        path.Root("name"),
			ConfigValue: types.StringValue(name),
		}
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), req, resp)
		responses = append(responses, *resp)
	}

	return responses
}

func TestNameAttribute_PlanTimeValidation_RejectsHyphenatedName(t *testing.T) {
	responses := runNameValidators(t, "my-sensor-name")

	hasError := false
	for _, resp := range responses {
		if resp.Diagnostics.HasError() {
			hasError = true
		}
	}

	if !hasError {
		t.Fatal("expected a plan-time validation error for hyphenated name \"my-sensor-name\", got none")
	}
}

func TestNameAttribute_PlanTimeValidation_AcceptsUnderscoreName(t *testing.T) {
	responses := runNameValidators(t, "my_sensor_name")

	for _, resp := range responses {
		if resp.Diagnostics.HasError() {
			t.Fatalf("expected no plan-time validation error for underscore name \"my_sensor_name\", got: %v", resp.Diagnostics)
		}
	}
}

// executionArchitectureValidators mirrors nameValidators: it pulls the
// "execution_architecture" attribute's validators out of the resource
// schema so tests can exercise the OneOf validator the same way the
// framework does during ValidateResourceConfig (plan/validate time, before
// apply).
func executionArchitectureValidators(t *testing.T) []validator.String {
	t.Helper()

	r := &macsensorResource{}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)

	attr, ok := schemaResp.Schema.Attributes["execution_architecture"]
	if !ok {
		t.Fatal("schema has no \"execution_architecture\" attribute")
	}

	strAttr, ok := attr.(schema.StringAttribute)
	if !ok {
		t.Fatalf("\"execution_architecture\" attribute is not a schema.StringAttribute, got %T", attr)
	}

	return strAttr.Validators
}

func runExecutionArchitectureValidators(t *testing.T, value string) []validator.StringResponse {
	t.Helper()

	validators := executionArchitectureValidators(t)
	responses := make([]validator.StringResponse, 0, len(validators))

	for _, v := range validators {
		req := validator.StringRequest{
			Path:        path.Root("execution_architecture"),
			ConfigValue: types.StringValue(value),
		}
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), req, resp)
		responses = append(responses, *resp)
	}

	return responses
}

func TestExecutionArchitectureAttribute_PlanTimeValidation_RejectsSixtyFourBit(t *testing.T) {
	responses := runExecutionArchitectureValidators(t, "64BIT")

	hasError := false
	for _, resp := range responses {
		if resp.Diagnostics.HasError() {
			hasError = true
		}
	}

	if !hasError {
		t.Fatal("expected a plan-time validation error for \"64BIT\", got none")
	}
}

func TestExecutionArchitectureAttribute_PlanTimeValidation_RejectsThirtyTwoBit(t *testing.T) {
	responses := runExecutionArchitectureValidators(t, "32BIT")

	hasError := false
	for _, resp := range responses {
		if resp.Diagnostics.HasError() {
			hasError = true
		}
	}

	if !hasError {
		t.Fatal("expected a plan-time validation error for \"32BIT\", got none")
	}
}

func TestExecutionArchitectureAttribute_PlanTimeValidation_AcceptsEitherOr(t *testing.T) {
	responses := runExecutionArchitectureValidators(t, "EITHER64OR32BIT")

	for _, resp := range responses {
		if resp.Diagnostics.HasError() {
			t.Fatalf("expected no plan-time validation error for \"EITHER64OR32BIT\", got: %v", resp.Diagnostics)
		}
	}
}
