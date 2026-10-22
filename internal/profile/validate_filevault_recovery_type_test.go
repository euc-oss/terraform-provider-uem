package profile

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Tests in this file cover validateFileVaultRequiresRecoveryType in
// internal/profile/validate.go, plus the schema-level
// int64validator.OneOf(1, 2, 3) on disk_encryption.filevault2.recovery_type.
//
// Live-verified behavior being guarded against (as<internal-env> tenant, macOS/AppleOsX):
// a uem_profile with disk_encryption.filevault2.enable = true and
// recovery_type omitted is accepted by `terraform plan` but rejected by UEM
// at apply time with a 422 (deviceProfile.DiskEncryption.DiskEncryptionFileVault2.RecoveryType
// ... Invalid RecoveryType, must be either 1-RecoveryTypePersonal,
// 2-RecoveryTypeCorporate or 3-RecoveryTypePersonalAndCorporate).

const fileVaultRecoveryTypeSummary = "recovery_type is required when disk_encryption is configured"

// runValidateConfigWithDiskEncryption builds a tfsdk.Config with the given
// platform and disk_encryption value (everything else defaults to null) and
// runs it through ProfileResource.ValidateConfig.
func runValidateConfigWithDiskEncryption(t *testing.T, platform string, diskEncryption tftypes.Value) *resource.ValidateConfigResponse {
	t.Helper()
	return runValidateConfigValues(t, map[string]tftypes.Value{
		"id":              nullString(),
		"name":            stringVal("Test Profile"),
		"platform":        stringVal(platform),
		"disk_encryption": diskEncryption,
	})
}

func TestValidateConfig_FileVault_EnableTrue_RecoveryTypeNull_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"enable": boolVal(true),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	wantPath := "disk_encryption.filevault2.recovery_type"
	errs := diagErrorsAtPath(resp, wantPath)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error at %s, got %d: %s", wantPath, len(errs), diagSummaries(resp))
	}
	if errs[0] != fileVaultRecoveryTypeSummary {
		t.Errorf("expected summary %q, got %q", fileVaultRecoveryTypeSummary, errs[0])
	}
}

func TestValidateConfig_FileVault_EnableTrue_RecoveryTypeSet_NoError(t *testing.T) {
	t.Parallel()

	for _, rt := range []int64{1, 2, 3} {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			de := diskEncryptionVal(map[string]tftypes.Value{
				"filevault2": filevaultVal(map[string]tftypes.Value{
					"enable":        boolVal(true),
					"recovery_type": int64Val(rt),
				}),
			})
			resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

			if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.recovery_type"); len(errs) != 0 {
				t.Fatalf("expected no error with recovery_type = %d, got: %v", rt, errs)
			}
		})
	}
}

func TestValidateConfig_FileVault_EnableTrue_RecoveryTypeUnknown_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"enable":        boolVal(true),
			"recovery_type": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		})),
		"airwatch": airwatchVal(minimalAirWatchBaselineFields()),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.recovery_type"); len(errs) != 0 {
		t.Fatalf("expected no recovery_type error with recovery_type unknown, got: %v", errs)
	}
}

// TestValidateConfig_FileVault_RecoveryTypeRequiredRegardlessOfEnable proves
// recovery_type is required whenever disk_encryption is present (canonical
// 26.2 rules: RecoveryType is a non-nullable int defaulting to 0, and 0 is
// out of range), whether enable is unknown, false, null, or the whole
// filevault2 sub-block is absent.
func TestValidateConfig_FileVault_RecoveryTypeRequiredRegardlessOfEnable(t *testing.T) {
	t.Parallel()

	cases := map[string]tftypes.Value{
		"enable unknown": diskEncryptionVal(map[string]tftypes.Value{
			"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
				"enable": tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
			})),
			"airwatch": airwatchVal(minimalAirWatchBaselineFields()),
		}),
		"enable false": diskEncryptionVal(map[string]tftypes.Value{
			"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
				"enable": boolVal(false),
			})),
			"airwatch": airwatchVal(minimalAirWatchBaselineFields()),
		}),
		"enable null": diskEncryptionVal(map[string]tftypes.Value{
			"filevault2": filevaultVal(minimalFileVaultBaselineFields()),
			"airwatch":   airwatchVal(minimalAirWatchBaselineFields()),
		}),
		"filevault2 absent": diskEncryptionVal(map[string]tftypes.Value{
			"airwatch": airwatchVal(minimalAirWatchBaselineFields()),
		}),
	}
	for name, de := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)
			errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.recovery_type")
			if len(errs) != 1 || errs[0] != fileVaultRecoveryTypeSummary {
				t.Fatalf("expected exactly 1 %q error, got %v", fileVaultRecoveryTypeSummary, errs)
			}
		})
	}
}

// TestValidateConfig_FileVault_OtherPlatform_NoError proves the check is
// macOS-only: buildAppleOsXDiskEncryptionFileVault2Entity is the only
// builder that ever sends this block, so an equivalent config on another
// platform is never flagged.
func TestValidateConfig_FileVault_OtherPlatform_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"enable": boolVal(true),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleiOS, de)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error on %q, got: %s", sdk.PlatformAppleiOS, diagSummaries(resp))
	}
}

// TestValidateConfig_FileVault_UnknownDiskEncryptionBlock_NoError proves a
// wholly unknown disk_encryption block produces no diagnostics, even with
// filevault2.enable = true baked into a real config elsewhere being
// impossible to express (the whole object is unknown).
func TestValidateConfig_FileVault_UnknownDiskEncryptionBlock_NoError(t *testing.T) {
	t.Parallel()

	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, tftypes.NewValue(diskEncryptionObjectType(), tftypes.UnknownValue))

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a wholly-unknown disk_encryption block, got: %s", diagSummaries(resp))
	}
}

// TestValidateConfig_FileVault_UnknownFileVaultBlock_NoError proves a wholly
// unknown filevault2 sub-object (disk_encryption itself known) produces no
// diagnostics.
func TestValidateConfig_FileVault_UnknownFileVaultBlock_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": tftypes.NewValue(diskEncryptionFileVaultType(), tftypes.UnknownValue),
		// airwatch supplied at its baseline so the (filevault2-independent)
		// use_intelligent_hub-required check doesn't also fire here and
		// muddy an assertion that's specifically about the unknown
		// filevault2 block.
		"airwatch": airwatchVal(minimalAirWatchBaselineFields()),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a wholly-unknown filevault2 block, got: %s", diagSummaries(resp))
	}
}

// --- schema-level int64validator.OneOf(1, 2, 3) on recovery_type -----------

// recoveryTypeValidators returns the validators attached to
// disk_encryption.filevault2.recovery_type so tests can exercise them the
// same way the framework does during ValidateResourceConfig (i.e. at
// plan/validate time, before apply).
func recoveryTypeValidators(t *testing.T) []validator.Int64 {
	t.Helper()

	schemaResp := getResourceSchema(t)

	deAttr, ok := schemaResp.Schema.Attributes["disk_encryption"]
	if !ok {
		t.Fatal("schema has no \"disk_encryption\" attribute")
	}
	deNested, ok := deAttr.(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("\"disk_encryption\" is not a schema.SingleNestedAttribute, got %T", deAttr)
	}

	fvAttr, ok := deNested.Attributes["filevault2"]
	if !ok {
		t.Fatal("disk_encryption schema has no \"filevault2\" attribute")
	}
	fvNested, ok := fvAttr.(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("\"filevault2\" is not a schema.SingleNestedAttribute, got %T", fvAttr)
	}

	rtAttr, ok := fvNested.Attributes["recovery_type"]
	if !ok {
		t.Fatal("filevault2 schema has no \"recovery_type\" attribute")
	}
	rtInt64, ok := rtAttr.(schema.Int64Attribute)
	if !ok {
		t.Fatalf("\"recovery_type\" is not a schema.Int64Attribute, got %T", rtAttr)
	}

	return rtInt64.Validators
}

func runRecoveryTypeValidators(t *testing.T, n int64) []validator.Int64Response {
	t.Helper()

	validators := recoveryTypeValidators(t)
	responses := make([]validator.Int64Response, 0, len(validators))
	for _, v := range validators {
		req := validator.Int64Request{
			Path:        path.Root("disk_encryption").AtName("filevault2").AtName("recovery_type"),
			ConfigValue: types.Int64Value(n),
		}
		resp := &validator.Int64Response{}
		v.ValidateInt64(context.Background(), req, resp)
		responses = append(responses, *resp)
	}
	return responses
}

func TestRecoveryTypeAttribute_PlanTimeValidation_RejectsOutOfRange(t *testing.T) {
	t.Parallel()

	for _, n := range []int64{0, 4, -1} {
		responses := runRecoveryTypeValidators(t, n)
		hasError := false
		for _, r := range responses {
			if r.Diagnostics.HasError() {
				hasError = true
			}
		}
		if !hasError {
			t.Errorf("expected a plan-time validation error for recovery_type = %d, got none", n)
		}
	}
}

func TestRecoveryTypeAttribute_PlanTimeValidation_AcceptsDocumentedValues(t *testing.T) {
	t.Parallel()

	for _, n := range []int64{1, 2, 3} {
		responses := runRecoveryTypeValidators(t, n)
		for _, r := range responses {
			if r.Diagnostics.HasError() {
				t.Errorf("expected no plan-time validation error for recovery_type = %d, got: %v", n, r.Diagnostics)
			}
		}
	}
}
