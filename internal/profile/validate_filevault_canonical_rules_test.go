package profile

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Tests in this file cover the FileVault2/AirWatch validators added to
// reconcile with the canonical UEM 26.2/26.4 rules report
// (SDK maintainer report 2026-09-25-canonical-262-macos-diskencryption): every new plan-time
// validator in internal/profile/validate.go added or broadened beyond the
// pre-existing filevault2 enable/recovery_type/filevault_user/prompt
// checks, plus the schema-level range/length/regex validators added to
// resource.go for the same reconciliation.

// --- validateDiskEncryptionEnableMustBeTrue ---------------------------

func TestValidateConfig_DiskEncryption_EnableFalse_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"enable": boolVal(false),
		})),
		"airwatch": airwatchVal(minimalAirWatchBaselineFields()),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.enable")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 enable error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_DiskEncryption_EnableTrue_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"enable":        boolVal(true),
			"recovery_type": int64Val(1),
		})),
		"airwatch": airwatchVal(minimalAirWatchBaselineFields()),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.enable"); len(errs) != 0 {
		t.Fatalf("expected no enable error, got: %v", errs)
	}
}

func TestValidateConfig_DiskEncryption_EnableAbsent_NoError(t *testing.T) {
	t.Parallel()

	// filevault2 sub-block entirely absent: UEM's ctor default (Enable =
	// true, a non-nullable bool) means there's nothing to reject.
	de := diskEncryptionVal(map[string]tftypes.Value{
		"mcx": mcxVal(map[string]tftypes.Value{"destroy_fv_key_on_standby": boolVal(true)}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.enable"); len(errs) != 0 {
		t.Fatalf("expected no enable error with filevault2 absent, got: %v", errs)
	}
}

// --- validateDiskEncryptionRequiresPromptToEnableFileVaultAt ------------

func TestValidateConfig_DiskEncryption_PromptMissing_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"filevault_user": int64Val(1),
		}),
		"airwatch": airwatchVal(minimalAirWatchBaselineFields()),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.prompt_to_enable_filevault_at")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 prompt_to_enable_filevault_at error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_DiskEncryption_PromptSet_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(minimalFileVaultBaselineFields()),
		"airwatch":   airwatchVal(minimalAirWatchBaselineFields()),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.prompt_to_enable_filevault_at"); len(errs) != 0 {
		t.Fatalf("expected no prompt_to_enable_filevault_at error, got: %v", errs)
	}
}

// --- validateDiskEncryptionRequiresUseIntelligentHub --------------------

func TestValidateConfig_DiskEncryption_UseIntelligentHubMissing_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(minimalFileVaultBaselineFields()),
		// airwatch left entirely absent.
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.use_intelligent_hub")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 use_intelligent_hub error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_DiskEncryption_UseIntelligentHubSet_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(minimalFileVaultBaselineFields()),
		"airwatch":   airwatchVal(minimalAirWatchBaselineFields()),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.use_intelligent_hub"); len(errs) != 0 {
		t.Fatalf("expected no use_intelligent_hub error, got: %v", errs)
	}
}

// --- validateFileVaultRecoveryTypeCorporateForbidsShowRecoveryKeyAndStoreKey ---

func TestValidateConfig_FileVault_RecoveryTypeCorporate_ShowRecoveryKeySet_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"recovery_type":                    int64Val(2),
			"show_recovery_key":                boolVal(true),
			"filevault_enterprise_certificate": stringVal("corp-cert"),
		})),
		"airwatch": airwatchVal(minimalAirWatchBaselineFields()),
	})
	resp := runValidateConfigValues(t, map[string]tftypes.Value{
		"id":               nullString(),
		"name":             stringVal("Test Profile"),
		"platform":         stringVal(sdk.PlatformAppleOsX),
		"disk_encryption":  de,
		"credentials_list": credentialsListVal([]map[string]tftypes.Value{{"credential_name": stringVal("corp-cert")}}),
	})

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.show_recovery_key"); len(errs) != 1 {
		t.Fatalf("expected exactly 1 show_recovery_key error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_FileVault_RecoveryTypeCorporate_StoreKeySet_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"recovery_type":                    int64Val(2),
			"filevault_enterprise_certificate": stringVal("corp-cert"),
		})),
		"airwatch": airwatchVal(mergeValues(minimalAirWatchBaselineFields(), map[string]tftypes.Value{
			"store_key": boolVal(true),
		})),
	})
	resp := runValidateConfigValues(t, map[string]tftypes.Value{
		"id":               nullString(),
		"name":             stringVal("Test Profile"),
		"platform":         stringVal(sdk.PlatformAppleOsX),
		"disk_encryption":  de,
		"credentials_list": credentialsListVal([]map[string]tftypes.Value{{"credential_name": stringVal("corp-cert")}}),
	})

	if errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.store_key"); len(errs) != 1 {
		t.Fatalf("expected exactly 1 store_key error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_FileVault_RecoveryTypeCorporate_KeysUnset_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"recovery_type":                    int64Val(2),
			"filevault_enterprise_certificate": stringVal("corp-cert"),
		})),
		"airwatch": airwatchVal(minimalAirWatchBaselineFields()),
	})
	resp := runValidateConfigValues(t, map[string]tftypes.Value{
		"id":               nullString(),
		"name":             stringVal("Test Profile"),
		"platform":         stringVal(sdk.PlatformAppleOsX),
		"disk_encryption":  de,
		"credentials_list": credentialsListVal([]map[string]tftypes.Value{{"credential_name": stringVal("corp-cert")}}),
	})

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.show_recovery_key"); len(errs) != 0 {
		t.Fatalf("expected no show_recovery_key error, got: %v", errs)
	}
	if errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.store_key"); len(errs) != 0 {
		t.Fatalf("expected no store_key error, got: %v", errs)
	}
}

// --- validateFileVaultRecoveryTypeRequiresCertificate / PersonalForbids ---

func TestValidateConfig_FileVault_RecoveryTypeCorporate_CertificateMissing_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"recovery_type": int64Val(2),
		})),
		"airwatch": airwatchVal(minimalAirWatchBaselineFields()),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.filevault_enterprise_certificate")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 filevault_enterprise_certificate error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_FileVault_RecoveryTypePersonalAndCorporate_CertificateMissing_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"recovery_type":     int64Val(3),
			"show_recovery_key": boolVal(true),
		})),
		"airwatch": airwatchVal(mergeValues(minimalAirWatchBaselineFields(), map[string]tftypes.Value{
			"store_key": boolVal(true),
		})),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.filevault_enterprise_certificate")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 filevault_enterprise_certificate error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_FileVault_RecoveryTypePersonal_CertificateSet_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"recovery_type":                    int64Val(1),
			"show_recovery_key":                boolVal(true),
			"filevault_enterprise_certificate": stringVal("corp-cert"),
		})),
		"airwatch": airwatchVal(mergeValues(minimalAirWatchBaselineFields(), map[string]tftypes.Value{
			"store_key": boolVal(true),
		})),
	})
	resp := runValidateConfigValues(t, map[string]tftypes.Value{
		"id":               nullString(),
		"name":             stringVal("Test Profile"),
		"platform":         stringVal(sdk.PlatformAppleOsX),
		"disk_encryption":  de,
		"credentials_list": credentialsListVal([]map[string]tftypes.Value{{"credential_name": stringVal("corp-cert")}}),
	})

	errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.filevault_enterprise_certificate")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 filevault_enterprise_certificate error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_FileVault_RecoveryTypePersonal_CertificateUnset_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"recovery_type":     int64Val(1),
			"show_recovery_key": boolVal(true),
		})),
		"airwatch": airwatchVal(mergeValues(minimalAirWatchBaselineFields(), map[string]tftypes.Value{
			"store_key": boolVal(true),
		})),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.filevault_enterprise_certificate"); len(errs) != 0 {
		t.Fatalf("expected no filevault_enterprise_certificate error, got: %v", errs)
	}
}

// --- validateFileVaultCertificateReferencesCredential --------------------

func TestValidateConfig_FileVault_CertificateNoMatchingCredential_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"recovery_type":                    int64Val(2),
			"filevault_enterprise_certificate": stringVal("corp-cert"),
		})),
		"airwatch": airwatchVal(minimalAirWatchBaselineFields()),
	})
	resp := runValidateConfigValues(t, map[string]tftypes.Value{
		"id":               nullString(),
		"name":             stringVal("Test Profile"),
		"platform":         stringVal(sdk.PlatformAppleOsX),
		"disk_encryption":  de,
		"credentials_list": credentialsListVal([]map[string]tftypes.Value{{"credential_name": stringVal("other-cert")}}),
	})

	errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.filevault_enterprise_certificate")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 filevault_enterprise_certificate error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_FileVault_CertificateMatchesCredential_CaseInsensitive_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"recovery_type":                    int64Val(2),
			"filevault_enterprise_certificate": stringVal("Corp-Cert"),
		})),
		"airwatch": airwatchVal(minimalAirWatchBaselineFields()),
	})
	resp := runValidateConfigValues(t, map[string]tftypes.Value{
		"id":               nullString(),
		"name":             stringVal("Test Profile"),
		"platform":         stringVal(sdk.PlatformAppleOsX),
		"disk_encryption":  de,
		"credentials_list": credentialsListVal([]map[string]tftypes.Value{{"credential_name": stringVal("corp-cert")}}),
	})

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.filevault_enterprise_certificate"); len(errs) != 0 {
		t.Fatalf("expected no filevault_enterprise_certificate error, got: %v", errs)
	}
}

func TestValidateConfig_FileVault_CertificateSet_CredentialsListUnknown_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"recovery_type":                    int64Val(2),
			"filevault_enterprise_certificate": stringVal("corp-cert"),
		})),
		"airwatch": airwatchVal(minimalAirWatchBaselineFields()),
	})
	resp := runValidateConfigValues(t, map[string]tftypes.Value{
		"id":               nullString(),
		"name":             stringVal("Test Profile"),
		"platform":         stringVal(sdk.PlatformAppleOsX),
		"disk_encryption":  de,
		"credentials_list": tftypes.NewValue(tftypes.List{ElementType: credentialsListItemType()}, tftypes.UnknownValue),
	})

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.filevault_enterprise_certificate"); len(errs) != 0 {
		t.Fatalf("expected no filevault_enterprise_certificate error with credentials_list unknown, got: %v", errs)
	}
}

// --- validateFileVaultUserNotSpecificForbidsUsername ----------------------

func TestValidateConfig_FileVault_UserCurrentOrNext_UsernameSet_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"filevault_user": int64Val(1),
			"username":       stringVal("od-user"),
		})),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.username")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 username error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_FileVault_UserCurrentOrNext_UsernameUnset_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"filevault_user": int64Val(1),
		})),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.username"); len(errs) != 0 {
		t.Fatalf("expected no username error, got: %v", errs)
	}
}

// --- validateFileVaultPromptLogoutOnlyForbidsBypass -----------------------

func TestValidateConfig_FileVault_PromptLogoutOnly_BypassSet_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"prompt_to_enable_filevault_at":   int64Val(2),
			"number_of_times_user_can_bypass": int64Val(3),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.number_of_times_user_can_bypass")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 number_of_times_user_can_bypass error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_FileVault_PromptLogoutOnly_BypassUnset_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"prompt_to_enable_filevault_at": int64Val(2),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.number_of_times_user_can_bypass"); len(errs) != 0 {
		t.Fatalf("expected no number_of_times_user_can_bypass error, got: %v", errs)
	}
}

// --- validateDiskEncryptionUseIntelligentHubGatesHubFlags -----------------

func TestValidateConfig_AirWatch_HubOff_NotifySet_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(minimalFileVaultBaselineFields()),
		"airwatch": airwatchVal(map[string]tftypes.Value{
			"use_intelligent_hub":        boolVal(false),
			"notify_user_for_encryption": boolVal(true),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.notify_user_for_encryption")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 notify_user_for_encryption error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_AirWatch_HubTrue_BothFlagsMissing_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(minimalFileVaultBaselineFields()),
		"airwatch": airwatchVal(map[string]tftypes.Value{
			"use_intelligent_hub": boolVal(true),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.notify_user_for_encryption"); len(errs) != 1 {
		t.Fatalf("expected exactly 1 notify_user_for_encryption error, got %d: %s", len(errs), diagSummaries(resp))
	}
	if errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.enable_recovery_key"); len(errs) != 1 {
		t.Fatalf("expected exactly 1 enable_recovery_key error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

// TestValidateConfig_AirWatch_HubTrue_BothFlagsFalse_Rejected is also
// SCOPE's drift-trap #2.
func TestValidateConfig_AirWatch_HubTrue_BothFlagsFalse_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(minimalFileVaultBaselineFields()),
		"airwatch": airwatchVal(map[string]tftypes.Value{
			"use_intelligent_hub":        boolVal(true),
			"notify_user_for_encryption": boolVal(false),
			"enable_recovery_key":        boolVal(false),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.use_intelligent_hub")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 use_intelligent_hub error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_AirWatch_HubTrue_NotifyTrue_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"recovery_type":     int64Val(1),
			"show_recovery_key": boolVal(true),
		})),
		"airwatch": airwatchVal(mergeValues(notifyClusterCompleteFields(), map[string]tftypes.Value{
			"store_key":                  boolVal(true),
			"use_intelligent_hub":        boolVal(true),
			"notify_user_for_encryption": boolVal(true),
			"enable_recovery_key":        boolVal(false),
		})),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got: %s", diagSummaries(resp))
	}
}

// --- validateDiskEncryptionNotifyGatesEncryptionNotificationFields --------

// notifyClusterCompleteFields returns every field
// notify_user_for_encryption = true requires, so tests can isolate one
// other check.
func notifyClusterCompleteFields() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"encryption_notification_title":                   stringVal("Encrypt now"),
		"encryption_notification_message":                 stringVal("Please encrypt this device"),
		"encryption_max_notify_attempts":                  int64Val(3),
		"encryption_notification_retry_interval_in_hours": int64Val(4),
		"encryption_action_after_last_notification":       int64Val(1),
	}
}

func TestValidateConfig_AirWatch_NotifyOff_TitleSet_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(minimalFileVaultBaselineFields()),
		"airwatch": airwatchVal(map[string]tftypes.Value{
			"use_intelligent_hub":           boolVal(false),
			"encryption_notification_title": stringVal("Encrypt now"),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.encryption_notification_title")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 encryption_notification_title error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_AirWatch_NotifyTrue_TitleMissing_Rejected(t *testing.T) {
	t.Parallel()

	fields := mergeValues(notifyClusterCompleteFields(), map[string]tftypes.Value{
		"use_intelligent_hub":           boolVal(true),
		"notify_user_for_encryption":    boolVal(true),
		"encryption_notification_title": nullString(),
	})
	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(minimalFileVaultBaselineFields()),
		"airwatch":   airwatchVal(fields),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.encryption_notification_title")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 encryption_notification_title error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_AirWatch_NotifyTrue_ClusterComplete_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(minimalFileVaultBaselineFields()),
		"airwatch": airwatchVal(mergeValues(notifyClusterCompleteFields(), map[string]tftypes.Value{
			"use_intelligent_hub":        boolVal(true),
			"notify_user_for_encryption": boolVal(true),
			"enable_recovery_key":        boolVal(false),
		})),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	for _, f := range notifyUserForEncryptionGatedFields {
		if errs := diagErrorsAtPath(resp, "disk_encryption.airwatch."+f); len(errs) != 0 {
			t.Errorf("field %s: expected no error, got: %v", f, errs)
		}
	}
}

// --- validateDiskEncryptionEnableRecoveryKeyGatesRecoveryKeyFields --------

// recoveryKeyClusterCompleteFields returns every field
// enable_recovery_key = true requires (plus store_key, which a separate
// rule also requires when enable_recovery_key is true), so tests can
// isolate one other check.
func recoveryKeyClusterCompleteFields() map[string]tftypes.Value {
	fields := map[string]tftypes.Value{
		"store_key": boolVal(true),
	}
	for _, f := range enableRecoveryKeyGatedFields {
		switch f {
		case "recovery_key_notification_retry_interval_in_hours":
			fields[f] = int64Val(4)
		default:
			fields[f] = stringVal("value")
		}
	}
	return fields
}

func TestValidateConfig_AirWatch_RecoveryKeyOff_PromptTitleSet_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(minimalFileVaultBaselineFields()),
		"airwatch": airwatchVal(map[string]tftypes.Value{
			"use_intelligent_hub":       boolVal(false),
			"recovery_key_prompt_title": stringVal("Rotate key"),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.recovery_key_prompt_title")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 recovery_key_prompt_title error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_AirWatch_RecoveryKeyTrue_PromptTitleMissing_Rejected(t *testing.T) {
	t.Parallel()

	fields := mergeValues(recoveryKeyClusterCompleteFields(), map[string]tftypes.Value{
		"use_intelligent_hub":       boolVal(true),
		"enable_recovery_key":       boolVal(true),
		"recovery_key_prompt_title": nullString(),
	})
	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(minimalFileVaultBaselineFields()),
		"airwatch":   airwatchVal(fields),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.recovery_key_prompt_title")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 recovery_key_prompt_title error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_AirWatch_RecoveryKeyTrue_ClusterComplete_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(minimalFileVaultBaselineFields()),
		"airwatch": airwatchVal(mergeValues(recoveryKeyClusterCompleteFields(), map[string]tftypes.Value{
			"use_intelligent_hub": boolVal(true),
			"enable_recovery_key": boolVal(true),
		})),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	for _, f := range enableRecoveryKeyGatedFields {
		if errs := diagErrorsAtPath(resp, "disk_encryption.airwatch."+f); len(errs) != 0 {
			t.Errorf("field %s: expected no error, got: %v", f, errs)
		}
	}
}

// --- validateDiskEncryptionEnableRecoveryKeyRequiresStoreKey --------------

func TestValidateConfig_AirWatch_EnableRecoveryKeyTrue_StoreKeyFalse_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(minimalFileVaultBaselineFields()),
		"airwatch": airwatchVal(mergeValues(recoveryKeyClusterCompleteFields(), map[string]tftypes.Value{
			"use_intelligent_hub": boolVal(true),
			"enable_recovery_key": boolVal(true),
			"store_key":           boolVal(false),
		})),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.store_key")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 store_key error, got %d: %s", len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_AirWatch_EnableRecoveryKeyTrue_StoreKeyTrue_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(minimalFileVaultBaselineFields()),
		"airwatch": airwatchVal(mergeValues(recoveryKeyClusterCompleteFields(), map[string]tftypes.Value{
			"use_intelligent_hub": boolVal(true),
			"enable_recovery_key": boolVal(true),
		})),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.store_key"); len(errs) != 0 {
		t.Fatalf("expected no store_key error, got: %v", errs)
	}
}

// --- validateFileVaultRecoveryTypeCorporateDriftTrapEnableRecoveryKey -----

func TestValidateConfig_FileVault_RecoveryTypeCorporate_EnableRecoveryKeyTrue_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"recovery_type":                    int64Val(2),
			"filevault_enterprise_certificate": stringVal("corp-cert"),
		})),
		"airwatch": airwatchVal(map[string]tftypes.Value{
			"use_intelligent_hub": boolVal(true),
			"enable_recovery_key": boolVal(true),
		}),
	})
	resp := runValidateConfigValues(t, map[string]tftypes.Value{
		"id":               nullString(),
		"name":             stringVal("Test Profile"),
		"platform":         stringVal(sdk.PlatformAppleOsX),
		"disk_encryption":  de,
		"credentials_list": credentialsListVal([]map[string]tftypes.Value{{"credential_name": stringVal("corp-cert")}}),
	})

	errs := diagErrorsAtPath(resp, "disk_encryption.airwatch.enable_recovery_key")
	if len(errs) == 0 {
		t.Fatalf("expected at least 1 enable_recovery_key error, got: %s", diagSummaries(resp))
	}
	found := false
	for _, e := range errs {
		if e == "enable_recovery_key silently reset when recovery_type is Corporate" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the drift-trap summary among enable_recovery_key errors, got: %v", errs)
	}
}

func TestValidateConfig_FileVault_RecoveryTypePersonal_EnableRecoveryKeyTrue_NoDriftTrapError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(mergeValues(minimalFileVaultBaselineFields(), map[string]tftypes.Value{
			"recovery_type":     int64Val(1),
			"show_recovery_key": boolVal(true),
		})),
		"airwatch": airwatchVal(mergeValues(recoveryKeyClusterCompleteFields(), map[string]tftypes.Value{
			"use_intelligent_hub": boolVal(true),
			"enable_recovery_key": boolVal(true),
		})),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	for _, e := range diagErrorsAtPath(resp, "disk_encryption.airwatch.enable_recovery_key") {
		if e == "enable_recovery_key silently reset when recovery_type is Corporate" {
			t.Fatalf("did not expect the Corporate drift-trap error for recovery_type = 1, got: %s", diagSummaries(resp))
		}
	}
}

// --- schema-level range validators ----------------------------------------

func int64AttrValidators(t *testing.T, names ...string) []validator.Int64 {
	t.Helper()

	schemaResp := getResourceSchema(t)
	attrs := schemaResp.Schema.Attributes
	var found schema.Attribute
	for i, name := range names {
		if i == 0 {
			deAttr, ok := attrs["disk_encryption"].(schema.SingleNestedAttribute)
			if !ok {
				t.Fatalf("disk_encryption is not a SingleNestedAttribute")
			}
			attrs = deAttr.Attributes
		}
		a, ok := attrs[name]
		if !ok {
			t.Fatalf("no attribute named %q at this level", name)
		}
		if i == len(names)-1 {
			found = a
			break
		}
		nestedA, ok := a.(schema.SingleNestedAttribute)
		if !ok {
			t.Fatalf("%q is not a SingleNestedAttribute", name)
		}
		attrs = nestedA.Attributes
	}
	int64Attr, ok := found.(schema.Int64Attribute)
	if !ok {
		t.Fatalf("attribute is not a schema.Int64Attribute, got %T", found)
	}
	return int64Attr.Validators
}

func runInt64Validators(t *testing.T, attrPath path.Path, validators []validator.Int64, n int64) bool {
	t.Helper()
	for _, v := range validators {
		req := validator.Int64Request{Path: attrPath, ConfigValue: types.Int64Value(n)}
		resp := &validator.Int64Response{}
		v.ValidateInt64(context.Background(), req, resp)
		if resp.Diagnostics.HasError() {
			return true
		}
	}
	return false
}

func TestBypassAttribute_RangeValidator(t *testing.T) {
	t.Parallel()

	validators := int64AttrValidators(t, "filevault2", "number_of_times_user_can_bypass")
	p := path.Root("disk_encryption").AtName("filevault2").AtName("number_of_times_user_can_bypass")

	for _, n := range []int64{-1, 11} {
		if !runInt64Validators(t, p, validators, n) {
			t.Errorf("expected a range error for number_of_times_user_can_bypass = %d, got none", n)
		}
	}
	for _, n := range []int64{0, 5, 10} {
		if runInt64Validators(t, p, validators, n) {
			t.Errorf("expected no range error for number_of_times_user_can_bypass = %d", n)
		}
	}
}

func TestEncryptionMaxNotifyAttemptsAttribute_RangeValidator(t *testing.T) {
	t.Parallel()

	validators := int64AttrValidators(t, "airwatch", "encryption_max_notify_attempts")
	p := path.Root("disk_encryption").AtName("airwatch").AtName("encryption_max_notify_attempts")

	for _, n := range []int64{-1, 101} {
		if !runInt64Validators(t, p, validators, n) {
			t.Errorf("expected a range error for encryption_max_notify_attempts = %d, got none", n)
		}
	}
	for _, n := range []int64{0, 50, 100} {
		if runInt64Validators(t, p, validators, n) {
			t.Errorf("expected no range error for encryption_max_notify_attempts = %d", n)
		}
	}
}

func TestEncryptionNotificationRetryIntervalAttribute_RangeValidator(t *testing.T) {
	t.Parallel()

	validators := int64AttrValidators(t, "airwatch", "encryption_notification_retry_interval_in_hours")
	p := path.Root("disk_encryption").AtName("airwatch").AtName("encryption_notification_retry_interval_in_hours")

	for _, n := range []int64{0, 169} {
		if !runInt64Validators(t, p, validators, n) {
			t.Errorf("expected a range error for retry_interval = %d, got none", n)
		}
	}
	for _, n := range []int64{1, 168} {
		if runInt64Validators(t, p, validators, n) {
			t.Errorf("expected no range error for retry_interval = %d", n)
		}
	}
}

func TestRecoveryKeyMaxFailureCountAttribute_RangeValidator(t *testing.T) {
	t.Parallel()

	validators := int64AttrValidators(t, "airwatch", "recovery_key_max_failure_count")
	p := path.Root("disk_encryption").AtName("airwatch").AtName("recovery_key_max_failure_count")

	for _, n := range []int64{0, 6} {
		if !runInt64Validators(t, p, validators, n) {
			t.Errorf("expected a range error for recovery_key_max_failure_count = %d, got none", n)
		}
	}
	for _, n := range []int64{1, 5} {
		if runInt64Validators(t, p, validators, n) {
			t.Errorf("expected no range error for recovery_key_max_failure_count = %d", n)
		}
	}
}

// --- schema-level string length/regex validators --------------------------

func stringAttrValidators(t *testing.T, names ...string) []validator.String {
	t.Helper()

	schemaResp := getResourceSchema(t)
	attrs := schemaResp.Schema.Attributes
	var found schema.Attribute
	for i, name := range names {
		if i == 0 {
			deAttr, ok := attrs["disk_encryption"].(schema.SingleNestedAttribute)
			if !ok {
				t.Fatalf("disk_encryption is not a SingleNestedAttribute")
			}
			attrs = deAttr.Attributes
		}
		a, ok := attrs[name]
		if !ok {
			t.Fatalf("no attribute named %q at this level", name)
		}
		if i == len(names)-1 {
			found = a
			break
		}
		nestedA, ok := a.(schema.SingleNestedAttribute)
		if !ok {
			t.Fatalf("%q is not a SingleNestedAttribute", name)
		}
		attrs = nestedA.Attributes
	}
	strAttr, ok := found.(schema.StringAttribute)
	if !ok {
		t.Fatalf("attribute is not a schema.StringAttribute, got %T", found)
	}
	return strAttr.Validators
}

func fileVaultStringValidatorsReject(t *testing.T, attrPath path.Path, validators []validator.String, s string) bool {
	t.Helper()
	for _, v := range validators {
		req := validator.StringRequest{Path: attrPath, ConfigValue: types.StringValue(s)}
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), req, resp)
		if resp.Diagnostics.HasError() {
			return true
		}
	}
	return false
}

func TestEncryptionNotificationTitleAttribute_LengthAndCharsetValidators(t *testing.T) {
	t.Parallel()

	validators := stringAttrValidators(t, "airwatch", "encryption_notification_title")
	p := path.Root("disk_encryption").AtName("airwatch").AtName("encryption_notification_title")

	tooLong := ""
	for i := 0; i < 30; i++ {
		tooLong += "a"
	}
	if !fileVaultStringValidatorsReject(t, p, validators, tooLong) {
		t.Errorf("expected a length error for a 30-char title, got none")
	}
	if !fileVaultStringValidatorsReject(t, p, validators, "bad$char") {
		t.Errorf("expected a charset error for \"bad$char\", got none")
	}
	if fileVaultStringValidatorsReject(t, p, validators, "Encrypt now, please!") {
		t.Errorf("expected no error for a valid 20-char title")
	}
}

func TestRecoveryKeyPromptMessageAttribute_LengthAndCharsetValidators(t *testing.T) {
	t.Parallel()

	validators := stringAttrValidators(t, "airwatch", "recovery_key_prompt_message")
	p := path.Root("disk_encryption").AtName("airwatch").AtName("recovery_key_prompt_message")

	tooLong := ""
	for i := 0; i < 151; i++ {
		tooLong += "a"
	}
	if !fileVaultStringValidatorsReject(t, p, validators, tooLong) {
		t.Errorf("expected a length error for a 151-char message, got none")
	}
	if !fileVaultStringValidatorsReject(t, p, validators, "bad*char") {
		t.Errorf("expected a charset error for \"bad*char\", got none")
	}
	if fileVaultStringValidatorsReject(t, p, validators, "Please rotate your recovery key.") {
		t.Errorf("expected no error for a valid message")
	}
}

// --- fail-on-revert sample ---
//
// The following 8 validators were each manually disabled in turn (editing
// internal/profile/validate.go via Edit to insert an unconditional `return`
// as the first line of the function body), the one targeted reject test
// below was run and confirmed to FAIL with the check disabled, then the
// function was restored (Edit again) and the suite re-confirmed green:
//   - validateDiskEncryptionEnableMustBeTrue -- TestValidateConfig_DiskEncryption_EnableFalse_Rejected
//   - validateFileVaultRequiresFileVaultUser (broadened) -- TestValidateConfig_FileVault_EnableFalse_FileVaultUserNull_Rejected
//   - validateDiskEncryptionRequiresPromptToEnableFileVaultAt -- TestValidateConfig_DiskEncryption_PromptMissing_Rejected
//   - validateDiskEncryptionRequiresUseIntelligentHub -- TestValidateConfig_DiskEncryption_UseIntelligentHubMissing_Rejected
//   - validateFileVaultRecoveryTypeCorporateForbidsShowRecoveryKeyAndStoreKey -- TestValidateConfig_FileVault_RecoveryTypeCorporate_ShowRecoveryKeySet_Rejected
//   - validateFileVaultUserNotSpecificForbidsUsername -- TestValidateConfig_FileVault_UserCurrentOrNext_UsernameSet_Rejected
//   - validateDiskEncryptionUseIntelligentHubGatesHubFlags -- TestValidateConfig_AirWatch_HubOff_NotifySet_Rejected
//   - validateFileVaultRecoveryTypeCorporateDriftTrapEnableRecoveryKey -- TestValidateConfig_FileVault_RecoveryTypeCorporate_EnableRecoveryKeyTrue_Rejected

// --- minimum valid live-shaped config (offline proof before any live apply) ---

// TestValidateConfig_FileVault_MinimumValidLiveConfig_ZeroDiagnostics proves,
// offline, that the minimum enabled FileVault 2 config this package's own
// live-e2e run applies (recovery_type = 1/Personal, filevault_user =
// 1/CurrentOrNextLoginUser, prompt_to_enable_filevault_at =
// 1/BothLoginAndLogout with bypass = 3, show_recovery_key = true,
// airwatch.store_key = true, airwatch.use_intelligent_hub = false) produces
// ZERO diagnostics of any kind against every validator in this package —
// the precondition the maintainer's task requires before that config is
// ever sent to a live tenant.
func TestValidateConfig_FileVault_MinimumValidLiveConfig_ZeroDiagnostics(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"enable":                          boolVal(true),
			"recovery_type":                   int64Val(1),
			"show_recovery_key":               boolVal(true),
			"filevault_user":                  int64Val(1),
			"prompt_to_enable_filevault_at":   int64Val(1),
			"number_of_times_user_can_bypass": int64Val(3),
		}),
		"airwatch": airwatchVal(map[string]tftypes.Value{
			"store_key":           boolVal(true),
			"use_intelligent_hub": boolVal(false),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected ZERO diagnostics for the minimum valid live config, got: %s", diagSummaries(resp))
	}
	if len(resp.Diagnostics) != 0 {
		t.Fatalf("expected ZERO diagnostics (including warnings) for the minimum valid live config, got %d: %v", len(resp.Diagnostics), resp.Diagnostics)
	}
}
