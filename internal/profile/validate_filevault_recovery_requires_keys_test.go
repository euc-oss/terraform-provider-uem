package profile

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Tests in this file cover
// validateFileVaultRecoveryTypeRequiresShowRecoveryKeyAndStoreKey in
// internal/profile/validate.go.
//
// Live-verified behavior being guarded against (as<internal-env> tenant, macOS/AppleOsX):
// discovered live-proving validateFileVaultRequiresRecoveryType's own fix — a
// uem_profile with disk_encryption.filevault2.recovery_type = 1 (Personal) or
// 3 (PersonalAndCorporate) is accepted by `terraform plan` but rejected by
// UEM at apply time with a 422
// (deviceProfile.DiskEncryption.DiskEncryptionFileVault2.ShowRecoveryKey,
// DiskEncryptionAirWatch.StoreKey ... Must provide the ShowRecoveryKey and
// StoreKey when the RecoveryType is Personal or PersonalAndCorporate) unless
// filevault2.show_recovery_key AND disk_encryption.airwatch.store_key are
// both set. recovery_type = 2 (Corporate) does not trigger this.

const showRecoveryKeyRequiredSummary = "show_recovery_key is required when recovery_type is Personal or PersonalAndCorporate"
const storeKeyRequiredSummary = "disk_encryption.airwatch.store_key is required when filevault2.recovery_type is Personal or PersonalAndCorporate"

func TestValidateConfig_FileVaultRecoveryKeys_RecoveryTypePersonal_BothMissing_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"enable":        boolVal(true),
			"recovery_type": int64Val(1),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	showErrs := diagErrorsAtPath(resp, "disk_encryption.filevault2.show_recovery_key")
	if len(showErrs) != 1 || showErrs[0] != showRecoveryKeyRequiredSummary {
		t.Fatalf("expected exactly 1 show_recovery_key error, got %v: %s", showErrs, diagSummaries(resp))
	}
	storeErrs := diagErrorsAtPath(resp, "disk_encryption.airwatch.store_key")
	if len(storeErrs) != 1 || storeErrs[0] != storeKeyRequiredSummary {
		t.Fatalf("expected exactly 1 store_key error, got %v: %s", storeErrs, diagSummaries(resp))
	}
}

func TestValidateConfig_FileVaultRecoveryKeys_RecoveryTypePersonalAndCorporate_BothMissing_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"enable":        boolVal(true),
			"recovery_type": int64Val(3),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.show_recovery_key"); len(errs) != 1 {
		t.Fatalf("expected exactly 1 show_recovery_key error for recovery_type=3, got %v: %s", errs, diagSummaries(resp))
	}
	if errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.store_key"); len(errs) != 1 {
		t.Fatalf("expected exactly 1 store_key error for recovery_type=3, got %v: %s", errs, diagSummaries(resp))
	}
}

func TestValidateConfig_FileVaultRecoveryKeys_BothSet_NoError(t *testing.T) {
	t.Parallel()

	for _, rt := range []int64{1, 3} {
		de := diskEncryptionVal(map[string]tftypes.Value{
			"filevault2": filevaultVal(map[string]tftypes.Value{
				"enable":            boolVal(true),
				"recovery_type":     int64Val(rt),
				"show_recovery_key": boolVal(true),
			}),
			"airwatch": airwatchVal(map[string]tftypes.Value{
				"store_key": boolVal(true),
			}),
		})
		resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

		if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.show_recovery_key"); len(errs) != 0 {
			t.Fatalf("recovery_type=%d: expected no show_recovery_key error, got %v", rt, errs)
		}
		if errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.store_key"); len(errs) != 0 {
			t.Fatalf("recovery_type=%d: expected no store_key error, got %v", rt, errs)
		}
	}
}

// TestValidateConfig_FileVaultRecoveryKeys_BothSetFalse_NoError proves a
// known false value counts as "set" -- the API's own wording is "must
// provide", not "must be true".
func TestValidateConfig_FileVaultRecoveryKeys_BothSetFalse_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"enable":            boolVal(true),
			"recovery_type":     int64Val(1),
			"show_recovery_key": boolVal(false),
		})),
		"airwatch": airwatchVal(mergeValues(minimalAirWatchBaselineFields(), map[string]tftypes.Value{
			"store_key": boolVal(false),
		})),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error with show_recovery_key/store_key known false, got: %s", diagSummaries(resp))
	}
}

// TestValidateConfig_FileVaultRecoveryKeys_RecoveryTypeCorporate_NoError
// also exercises validateFileVaultRecoveryTypeRequiresCertificate and
// validateFileVaultCertificateReferencesCredential (recovery_type = 2
// requires a filevault_enterprise_certificate that names a credentials_list
// entry): both are satisfied here so this test still isolates "no
// show_recovery_key/store_key error for recovery_type = 2".
func TestValidateConfig_FileVaultRecoveryKeys_RecoveryTypeCorporate_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"enable":                           boolVal(true),
			"recovery_type":                    int64Val(2),
			"filevault_enterprise_certificate": stringVal("corp-cert"),
		})),
		"airwatch": airwatchVal(minimalAirWatchBaselineFields()),
	})
	resp := runValidateConfigValues(t, map[string]tftypes.Value{
		"id":              nullString(),
		"name":            stringVal("Test Profile"),
		"platform":        stringVal(sdk.PlatformAppleOsX),
		"disk_encryption": de,
		"credentials_list": credentialsListVal([]map[string]tftypes.Value{
			{"credential_name": stringVal("corp-cert")},
		}),
	})

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for recovery_type = 2 (Corporate), got: %s", diagSummaries(resp))
	}
}

func TestValidateConfig_FileVaultRecoveryKeys_RecoveryTypeUnknown_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"enable":        boolVal(true),
			"recovery_type": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		})),
		"airwatch": airwatchVal(minimalAirWatchBaselineFields()),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error with recovery_type unknown, got: %s", diagSummaries(resp))
	}
}

func TestValidateConfig_FileVaultRecoveryKeys_ShowRecoveryKeyUnknown_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"enable":            boolVal(true),
			"recovery_type":     int64Val(1),
			"show_recovery_key": tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		}),
		"airwatch": airwatchVal(map[string]tftypes.Value{
			"store_key": boolVal(true),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.show_recovery_key"); len(errs) != 0 {
		t.Fatalf("expected no show_recovery_key error when unknown, got %v", errs)
	}
}

func TestValidateConfig_FileVaultRecoveryKeys_StoreKeyUnknown_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"enable":            boolVal(true),
			"recovery_type":     int64Val(1),
			"show_recovery_key": boolVal(true),
		}),
		"airwatch": airwatchVal(map[string]tftypes.Value{
			"store_key": tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.store_key"); len(errs) != 0 {
		t.Fatalf("expected no store_key error when unknown, got %v", errs)
	}
}

// TestValidateConfig_FileVaultRecoveryKeys_AirwatchBlockUnknown_NoStoreKeyError
// proves a wholly unknown airwatch block skips the store_key check (can't
// decide whether it will end up null).
func TestValidateConfig_FileVaultRecoveryKeys_AirwatchBlockUnknown_NoStoreKeyError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"enable":            boolVal(true),
			"recovery_type":     int64Val(1),
			"show_recovery_key": boolVal(true),
		}),
		"airwatch": tftypes.NewValue(diskEncryptionAirWatchType(), tftypes.UnknownValue),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.store_key"); len(errs) != 0 {
		t.Fatalf("expected no store_key error when the whole airwatch block is unknown, got %v", errs)
	}
}

// TestValidateConfig_FileVaultRecoveryKeys_AirwatchBlockAbsent_StoreKeyError
// proves an entirely absent (null) airwatch block still fires the store_key
// diagnostic: store_key is definitely not set in that case.
func TestValidateConfig_FileVaultRecoveryKeys_AirwatchBlockAbsent_StoreKeyError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"enable":            boolVal(true),
			"recovery_type":     int64Val(1),
			"show_recovery_key": boolVal(true),
		}),
		// airwatch left at diskEncryptionVal's null default.
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.store_key"); len(errs) != 1 {
		t.Fatalf("expected exactly 1 store_key error with airwatch absent, got %v: %s", errs, diagSummaries(resp))
	}
}

func TestValidateConfig_FileVaultRecoveryKeys_OtherPlatform_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"enable":        boolVal(true),
			"recovery_type": int64Val(1),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleiOS, de)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error on %q, got: %s", sdk.PlatformAppleiOS, diagSummaries(resp))
	}
}
