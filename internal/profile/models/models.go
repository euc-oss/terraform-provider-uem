package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ProfileResourceModel struct {
	ID                  types.String              `tfsdk:"id"`
	Name                types.String              `tfsdk:"name"`
	Description         types.String              `tfsdk:"description"`
	Platform            types.String              `tfsdk:"platform"`
	OrgGroupID          types.String              `tfsdk:"org_group_id"`
	AssignmentType      types.String              `tfsdk:"assignment_type"`
	ProfileScope        types.String              `tfsdk:"profile_scope"`
	IsActive            types.Bool                `tfsdk:"is_active"`
	LockScreenMessage   types.String              `tfsdk:"lock_screen_message"`
	Passcode            *PasscodeModel            `tfsdk:"passcode"`
	CustomSettingsList  []CustomSettingsItemModel `tfsdk:"custom_settings_list"`
	NetworkList         []NetworkItemModel        `tfsdk:"network_list"`
	CredentialsList     []CredentialItemModel     `tfsdk:"credentials_list"`
	DiskEncryption      *DiskEncryptionModel      `tfsdk:"disk_encryption"`
	Gatekeeper          *GatekeeperModel          `tfsdk:"gatekeeper"`
	Restrictions        *RestrictionsModel        `tfsdk:"restrictions"`
	SystemExtensions    *SystemExtensionsModel    `tfsdk:"system_extensions"`
	PrivacyPreferences  []PrivacyPreferenceModel  `tfsdk:"privacy_preferences"`
	ScepList            []ScepItemModel           `tfsdk:"scep_list"`
	WebClipsList        []WebClipItemModel        `tfsdk:"web_clips_list"`
	VpnList             []VPNItemModel            `tfsdk:"vpn_list"`
	EasMicrosoftOutlook *EasMicrosoftOutlookModel `tfsdk:"eas_microsoft_outlook"`
	KernelExtension     *KernelExtensionModel     `tfsdk:"kernel_extension"`
	CustomAttributes    []CustomAttributeModel    `tfsdk:"custom_attributes"`
	UUID                types.String              `tfsdk:"uuid"`
	ProfileContext      types.String              `tfsdk:"profile_context"`
	AssignedSmartGroups []types.String            `tfsdk:"assigned_smart_groups"`
	ExcludedSmartGroups []types.String            `tfsdk:"excluded_smart_groups"`
}

type CustomSettingsItemModel struct {
	CustomSettings types.String `tfsdk:"custom_settings"`
}

type NetworkItemModel struct {
	NetworkInterface                   types.String `tfsdk:"network_interface"`
	ServiceSetIdentifier               types.String `tfsdk:"service_set_identifier"`
	HiddenNetwork                      types.Bool   `tfsdk:"hidden_network"`
	AutoJoin                           types.Bool   `tfsdk:"auto_join"`
	SecurityType                       types.String `tfsdk:"security_type"`
	Password                           types.String `tfsdk:"password"`
	UseAsLoginWindowConfiguration      types.Bool   `tfsdk:"use_as_login_window_configuration"`
	UseDirectoryAuthentication         types.Bool   `tfsdk:"use_directory_authentication"`
	TLS                                types.Bool   `tfsdk:"tls"`
	TTLS                               types.Bool   `tfsdk:"ttls"`
	LEAP                               types.Bool   `tfsdk:"leap"`
	PEAP                               types.Bool   `tfsdk:"peap"`
	EAPFAST                            types.Bool   `tfsdk:"eap_fast"`
	EAPSIM                             types.Bool   `tfsdk:"eap_sim"`
	EAPAKA                             types.Bool   `tfsdk:"eap_aka"`
	TLSMinimumVersion                  types.String `tfsdk:"tls_minimum_version"`
	TLSMaximumVersion                  types.String `tfsdk:"tls_maximum_version"`
	DisableAssociationMACRandomization types.Bool   `tfsdk:"disable_association_mac_randomization"`
	UserName                           types.String `tfsdk:"user_name"`
	UserPassword                       types.String `tfsdk:"user_password"`
	IdentityCertificate                types.String `tfsdk:"identity_certificate"`
	InnerIdentity                      types.String `tfsdk:"inner_identity"`
	OuterIdentity                      types.String `tfsdk:"outer_identity"`
	UsePAC                             types.Bool   `tfsdk:"use_pac"`
	AllowTwoRANDs                      types.Bool   `tfsdk:"allow_two_rands"`
	TrustedCertificates                types.List   `tfsdk:"trusted_certificates"`
	AllowTrustExceptions               types.Bool   `tfsdk:"allow_trust_exceptions"`
	ProxyType                          types.String `tfsdk:"proxy_type"`
	ProxyServer                        types.String `tfsdk:"proxy_server"`
	ProxyServerPort                    types.Int64  `tfsdk:"proxy_server_port"`
	ProxyUsername                      types.String `tfsdk:"proxy_username"`
	ProxyPassword                      types.String `tfsdk:"proxy_password"`
	ProxyUrl                           types.String `tfsdk:"proxy_url"`
	PacFallback                        types.Bool   `tfsdk:"pac_fallback"`
}

type CredentialItemModel struct {
	CredentialSource             types.String `tfsdk:"credential_source"`
	CredentialName               types.String `tfsdk:"credential_name"`
	CertificatePayload           types.String `tfsdk:"certificate_payload"`
	CertificatePassword          types.String `tfsdk:"certificate_password"`
	CertificateID                types.Int64  `tfsdk:"certificate_id"`
	CertificateAuthority         types.Int64  `tfsdk:"certificate_authority"`
	CertificateTemplate          types.Int64  `tfsdk:"certificate_template"`
	AllowAccessToAllApplications types.Bool   `tfsdk:"allow_access_to_all_applications"`
	KeyIsExtractable             types.Bool   `tfsdk:"key_is_extractable"`
}

type PasscodeModel struct {
	RequirePasscodeOnDevice          types.Bool   `tfsdk:"require_passcode_on_device"`
	AllowSimpleValue                 types.Bool   `tfsdk:"allow_simple_value"`
	RequireAlphanumericValue         types.Bool   `tfsdk:"require_alphanumeric_value"`
	MinimumPasscodeLength            types.Int64  `tfsdk:"minimum_passcode_length"`
	MinimumNumberOfComplexCharacters types.String `tfsdk:"minimum_number_of_complex_characters"`
	MaximumPasscodeAge               types.String `tfsdk:"maximum_passcode_age"`
	AutoLock                         types.String `tfsdk:"auto_lock"`
	GracePeriod                      types.Int64  `tfsdk:"grace_period"`
	MaxFailedAttempts                types.String `tfsdk:"max_failed_attempts"`
	PinHistory                       types.String `tfsdk:"pin_history"`
	MinutesUntilFailedLoginReset     types.Int64  `tfsdk:"minutes_until_failed_login_reset"`
}

type DiskEncryptionModel struct {
	AirWatch  *DiskEncryptionAirWatchModel  `tfsdk:"airwatch"`
	FileVault *DiskEncryptionFileVaultModel `tfsdk:"filevault2"`
	// MCX is a types.Object, not a *DiskEncryptionMCXModel pointer (B42
	// follow-up): the framework marks a fresh Optional+Computed
	// SingleNestedAttribute unknown on create when configuration omits it,
	// and a raw Go pointer struct field cannot decode an unknown value ("cannot
	// handle unknown values"). types.Object holds null/unknown/known directly,
	// so decoding req.Plan.Get into this field never panics regardless of
	// which of the three states the plan carries. See
	// DiskEncryptionMCXAttrTypes for the attribute types used to construct and
	// decode this value, matching the "mcx" schema in resource.go.
	MCX types.Object `tfsdk:"mcx"`
}

// DiskEncryptionMCXAttrTypes is disk_encryption.mcx's attribute types,
// matching the SingleNestedAttribute schema in resource.go. Used wherever a
// DiskEncryptionModel.MCX types.Object value needs to be constructed
// (types.ObjectValueMust/types.ObjectNull) — the read mapper building state
// from UEM's response, and tests.
var DiskEncryptionMCXAttrTypes = map[string]attr.Type{
	"destroy_fv_key_on_standby": types.BoolType,
}

type DiskEncryptionAirWatchModel struct {
	StoreKey                                    types.Bool   `tfsdk:"store_key"`
	RotateKeyAfter                              types.Int64  `tfsdk:"rotate_key_after"`
	UseIntelligentHub                           types.Bool   `tfsdk:"use_intelligent_hub"`
	NotifyUserForEncryption                     types.Bool   `tfsdk:"notify_user_for_encryption"`
	EncryptionNotificationTitle                 types.String `tfsdk:"encryption_notification_title"`
	EncryptionNotificationMessage               types.String `tfsdk:"encryption_notification_message"`
	EncryptionMaxNotifyAttempts                 types.Int64  `tfsdk:"encryption_max_notify_attempts"`
	EncryptionNotificationRetryIntervalInHours  types.Int64  `tfsdk:"encryption_notification_retry_interval_in_hours"`
	EncryptionActionAfterLastNotification       types.Int64  `tfsdk:"encryption_action_after_last_notification"`
	EnableRecoveryKey                           types.Bool   `tfsdk:"enable_recovery_key"`
	RecoveryKeyNotificationTitle                types.String `tfsdk:"recovery_key_notification_title"`
	RecoveryKeyNotificationMessage              types.String `tfsdk:"recovery_key_notification_message"`
	RecoveryKeyNotificationRetryIntervalInHours types.Int64  `tfsdk:"recovery_key_notification_retry_interval_in_hours"`
	RecoveryKeyPromptTitle                      types.String `tfsdk:"recovery_key_prompt_title"`
	RecoveryKeyPromptMessage                    types.String `tfsdk:"recovery_key_prompt_message"`
	RecoveryKeySuccessTitle                     types.String `tfsdk:"recovery_key_success_title"`
	RecoveryKeySuccessMessage                   types.String `tfsdk:"recovery_key_success_message"`
	RecoveryKeyErrorTitle                       types.String `tfsdk:"recovery_key_error_title"`
	RecoveryKeyErrorMessage                     types.String `tfsdk:"recovery_key_error_message"`
	RecoveryKeyMaxFailureCount                  types.Int64  `tfsdk:"recovery_key_max_failure_count"`
}

type DiskEncryptionFileVaultModel struct {
	Enable                         types.Bool   `tfsdk:"enable"`
	ShowRecoveryKey                types.Bool   `tfsdk:"show_recovery_key"`
	RecoveryType                   types.Int64  `tfsdk:"recovery_type"`
	FileVaultEnterpriseCertificate types.String `tfsdk:"filevault_enterprise_certificate"`
	FileVaultUser                  types.Int64  `tfsdk:"filevault_user"`
	Username                       types.String `tfsdk:"username"`
	PromptToEnableFileVaultAt      types.Int64  `tfsdk:"prompt_to_enable_filevault_at"`
	NumberOfTimesUserCanBypass     types.Int64  `tfsdk:"number_of_times_user_can_bypass"`
}

// GatekeeperModel maps to MacOsGatekeeperPayloadV2Entity, surfaced in the UEM
// console as "Security & Privacy". The seven fields cover auto-unlock, Touch
// ID, Handoff, screen capture, and software-update deferral.
type GatekeeperModel struct {
	AllowAutoUnlock              types.Bool  `tfsdk:"allow_auto_unlock"`
	AllowFingerprintForUnlock    types.Bool  `tfsdk:"allow_fingerprint_for_unlock"`
	AllowHandoff                 types.Bool  `tfsdk:"allow_handoff"`
	AllowScreenCapture           types.Bool  `tfsdk:"allow_screen_capture"`
	EnableAppSoftwareUpdateDelay types.Bool  `tfsdk:"enable_app_software_update_delay"`
	EnableSoftwareUpdateDelay    types.Bool  `tfsdk:"enable_software_update_delay"`
	EnforcedSoftwareUpdateDelay  types.Int64 `tfsdk:"enforced_software_update_delay"`
}

type RestrictionsModel struct {
	Applications  *RestrictionsApplicationsModel  `tfsdk:"applications"`
	Desktop       *RestrictionsDesktopModel       `tfsdk:"desktop"`
	Functionality *RestrictionsFunctionalityModel `tfsdk:"functionality"`
	Media         *RestrictionsMediaModel         `tfsdk:"media"`
	Preferences   *RestrictionsPreferencesModel   `tfsdk:"preferences"`
	Sharing       *RestrictionsSharingModel       `tfsdk:"sharing"`
	Widgets       *RestrictionsWidgetsModel       `tfsdk:"widgets"`
}

type RestrictionsApplicationsModel struct {
	AllowApplication                            types.List                   `tfsdk:"allow_application"`
	AllowFolders                                types.List                   `tfsdk:"allow_folders"`
	DisallowFolders                             types.List                   `tfsdk:"disallow_folders"`
	RestrictWhichApplicationsAreAllowedToLaunch types.Bool                   `tfsdk:"restrict_which_applications_are_allowed_to_launch"`
	AppStore                                    *RestrictionsAppStoreModel   `tfsdk:"app_store"`
	AppleMusic                                  *RestrictionsAppleMusicModel `tfsdk:"apple_music"`
	Camera                                      *RestrictionsCameraModel     `tfsdk:"camera"`
	GameCentre                                  *RestrictionsGameCentreModel `tfsdk:"game_centre"`
	Safari                                      *RestrictionsSafariModel     `tfsdk:"safari"`
}

type RestrictionsAppStoreModel struct {
	AllowAppStoreAppAdoption                 types.Bool `tfsdk:"allow_app_store_app_adoption"`
	RequireAdminPasswordToInstallOrUpdateApp types.Bool `tfsdk:"require_admin_password_to_install_or_update_app"`
	RestrictAppStoreToSoftwareUpdatesOnly    types.Bool `tfsdk:"restrict_app_store_to_software_updates_only"`
}

type RestrictionsAppleMusicModel struct {
	AllowMusicService types.Bool `tfsdk:"allow_music_service"`
}

type RestrictionsCameraModel struct {
	AllowUseOfBuiltInCamera types.Bool `tfsdk:"allow_use_of_built_in_camera"`
}

type RestrictionsGameCentreModel struct {
	AllowAddingGameCenterFriends types.Bool `tfsdk:"allow_adding_game_center_friends"`
	AllowGameCenterModification  types.Bool `tfsdk:"allow_game_center_modification"`
	AllowMultiplayerGaming       types.Bool `tfsdk:"allow_multiplayer_gaming"`
	AllowUseOfGameCenter         types.Bool `tfsdk:"allow_use_of_game_center"`
}

type RestrictionsSafariModel struct {
	AllowDeprecatedWebKitTls types.Bool `tfsdk:"allow_deprecated_web_kit_tls"`
	AllowSafariAutoFill      types.Bool `tfsdk:"allow_safari_auto_fill"`
}

type RestrictionsDesktopModel struct {
	DesktopPicturePath types.String `tfsdk:"desktop_picture_path"`
	LockDesktopPicture types.Bool   `tfsdk:"lock_desktop_picture"`
}

type RestrictionsFunctionalityModel struct {
	AirPrint       *RestrictionsAirPrintModel       `tfsdk:"air_print"`
	ContentCaching *RestrictionsContentCachingModel `tfsdk:"content_caching"`
	ICloud         *RestrictionsICloudModel         `tfsdk:"icloud"`
	Passwords      *RestrictionsPasswordsModel      `tfsdk:"passwords"`
	Spotlight      *RestrictionsSpotlightModel      `tfsdk:"spotlight"`
}

type RestrictionsAirPrintModel struct {
	AllowAirPrint                      types.Bool `tfsdk:"allow_air_print"`
	AllowAirPrintiBeaconDiscovery      types.Bool `tfsdk:"allow_air_print_ibeacon_discovery"`
	ForceAirPrintTrustedTLSRequirement types.Bool `tfsdk:"force_air_print_trusted_tls_requirement"`
}

type RestrictionsContentCachingModel struct {
	AllowContentCaching types.Bool `tfsdk:"allow_content_caching"`
}

type RestrictionsICloudModel struct {
	AllowAirPrint                          types.Bool `tfsdk:"allow_air_print"`
	AllowAirPrintiBeaconDiscovery          types.Bool `tfsdk:"allow_air_print_ibeacon_discovery"`
	AllowCloudDesktopAndDocuments          types.Bool `tfsdk:"allow_cloud_desktop_and_documents"`
	AllowDeprecatedWebKitTls               types.Bool `tfsdk:"allow_deprecated_web_kit_tls"`
	AllowICloudFMM                         types.Bool `tfsdk:"allow_icloud_fmm"`
	AllowIcloudAddressBook                 types.Bool `tfsdk:"allow_icloud_address_book"`
	AllowIcloudBTMM                        types.Bool `tfsdk:"allow_icloud_btmm"`
	AllowIcloudBookmarks                   types.Bool `tfsdk:"allow_icloud_bookmarks"`
	AllowIcloudCalendar                    types.Bool `tfsdk:"allow_icloud_calendar"`
	AllowIcloudDocumentsAndData            types.Bool `tfsdk:"allow_icloud_documents_and_data"`
	AllowIcloudKeychainSync                types.Bool `tfsdk:"allow_icloud_keychain_sync"`
	AllowIcloudMail                        types.Bool `tfsdk:"allow_icloud_mail"`
	AllowIcloudNotes                       types.Bool `tfsdk:"allow_icloud_notes"`
	AllowIcloudReminders                   types.Bool `tfsdk:"allow_icloud_reminders"`
	AllowPasswordAutoFill                  types.Bool `tfsdk:"allow_password_auto_fill"`
	AllowPasswordProximityRequests         types.Bool `tfsdk:"allow_password_proximity_requests"`
	AllowPasswordSharing                   types.Bool `tfsdk:"allow_password_sharing"`
	AllowUseIcloudPasswordForLocalAccounts types.Bool `tfsdk:"allow_use_icloud_password_for_local_accounts"`
	ForceAirPrintTrustedTLSRequirement     types.Bool `tfsdk:"force_air_print_trusted_tls_requirement"`
}

type RestrictionsPasswordsModel struct {
	AllowPasswordAutoFill          types.Bool `tfsdk:"allow_password_auto_fill"`
	AllowPasswordProximityRequests types.Bool `tfsdk:"allow_password_proximity_requests"`
	AllowPasswordSharing           types.Bool `tfsdk:"allow_password_sharing"`
}

type RestrictionsSpotlightModel struct {
	AllowSpotlightSuggestions types.Bool `tfsdk:"allow_spotlight_suggestions"`
}

type RestrictionsMediaModel struct {
	AutoEjectMedia              types.Bool                      `tfsdk:"auto_eject_media"`
	DiskMediaCDs                *RestrictionsMediaAccessModel   `tfsdk:"disk_media_cds"`
	DiskMediaDVDs               *RestrictionsMediaAccessModel   `tfsdk:"disk_media_dvds"`
	ExternalHardDiskMediaAccess *RestrictionsMediaAccessModel   `tfsdk:"external_hard_disk_media_access"`
	HardDiskDvdRam              *RestrictionsMediaAccessModel   `tfsdk:"hard_disk_dvd_ram"`
	HardDiskImages              *RestrictionsMediaAccessModel   `tfsdk:"hard_disk_images"`
	InternalHardDiskMediaAccess *RestrictionsMediaAccessModel   `tfsdk:"internal_hard_disk_media_access"`
	NetworkAccess               *RestrictionsNetworkAccessModel `tfsdk:"network_access"`
	RecordableDisc              *RestrictionsBurnSupportModel   `tfsdk:"recordable_disc"`
}

type RestrictionsMediaAccessModel struct {
	Allow        types.Bool `tfsdk:"allow"`
	Authenticate types.Bool `tfsdk:"authenticate"`
	ReadOnly     types.Bool `tfsdk:"read_only"`
}

type RestrictionsNetworkAccessModel struct {
	AirDrop types.Bool `tfsdk:"air_drop"`
}

type RestrictionsBurnSupportModel struct {
	BurnSupport *RestrictionsMediaAccessModel `tfsdk:"burn_support"`
}

type RestrictionsPreferencesModel struct {
	Accessibility          types.Bool   `tfsdk:"accessibility"`
	AppStore               types.Bool   `tfsdk:"app_store"`
	Bluetooth              types.Bool   `tfsdk:"bluetooth"`
	CDsAndDVDs             types.Bool   `tfsdk:"cds_and_dvds"`
	DateAndTime            types.Bool   `tfsdk:"date_and_time"`
	DesktopAndScreenSaver  types.Bool   `tfsdk:"desktop_and_screen_saver"`
	DictationAndSpeech     types.Bool   `tfsdk:"dictation_and_speech"`
	Displays               types.Bool   `tfsdk:"displays"`
	Dock                   types.Bool   `tfsdk:"dock"`
	EnabledPreferencePanes types.Bool   `tfsdk:"enabled_preference_panes"`
	EnergySaver            types.Bool   `tfsdk:"energy_saver"`
	Extensions             types.Bool   `tfsdk:"extensions"`
	FibreChannel           types.Bool   `tfsdk:"fibre_channel"`
	FlashPlayer            types.Bool   `tfsdk:"flash_player"`
	General                types.Bool   `tfsdk:"general"`
	Ink                    types.Bool   `tfsdk:"ink"`
	InternetAccounts       types.Bool   `tfsdk:"internet_accounts"`
	Keyboard               types.Bool   `tfsdk:"keyboard"`
	LanguageAndText        types.Bool   `tfsdk:"language_and_text"`
	MissionControl         types.Bool   `tfsdk:"mission_control"`
	MobileMe               types.Bool   `tfsdk:"mobile_me"`
	Mouse                  types.Bool   `tfsdk:"mouse"`
	Network                types.Bool   `tfsdk:"network"`
	Notifications          types.Bool   `tfsdk:"notifications"`
	ParentalControls       types.Bool   `tfsdk:"parental_controls"`
	PreferenceBehavior     types.String `tfsdk:"preference_behavior"`
	PrintAndScan           types.Bool   `tfsdk:"print_and_scan"`
	Profiles               types.Bool   `tfsdk:"profiles"`
	SecurityAndPrivacy     types.Bool   `tfsdk:"security_and_privacy"`
	Sharing                types.Bool   `tfsdk:"sharing"`
	SoftwareUpdate         types.Bool   `tfsdk:"software_update"`
	Sound                  types.Bool   `tfsdk:"sound"`
	Spotlight              types.Bool   `tfsdk:"spotlight"`
	StartupDisk            types.Bool   `tfsdk:"startup_disk"`
	TimeMachine            types.Bool   `tfsdk:"time_machine"`
	Trackpad               types.Bool   `tfsdk:"trackpad"`
	UsersAndGroups         types.Bool   `tfsdk:"users_and_groups"`
	Xsan                   types.Bool   `tfsdk:"xsan"`
	ICloud                 types.Bool   `tfsdk:"icloud"`
}

type RestrictionsSharingModel struct {
	AddtoAperture                          types.Bool `tfsdk:"add_to_aperture"`
	AddtoReadingList                       types.Bool `tfsdk:"add_to_reading_list"`
	AddtoiPhoto                            types.Bool `tfsdk:"add_to_iphoto"`
	AirDrop                                types.Bool `tfsdk:"air_drop"`
	AutomaticallyEnableNewSharingServices  types.Bool `tfsdk:"automatically_enable_new_sharing_services"`
	Facebook                               types.Bool `tfsdk:"facebook"`
	Mail                                   types.Bool `tfsdk:"mail"`
	Messages                               types.Bool `tfsdk:"messages"`
	RestrictWhichSharingServicesAreEnabled types.Bool `tfsdk:"restrict_which_sharing_services_are_enabled"`
	SinaWeibo                              types.Bool `tfsdk:"sina_weibo"`
	Twitter                                types.Bool `tfsdk:"twitter"`
	VideoServices                          types.Bool `tfsdk:"video_services"`
}

// SystemExtensionsModel maps to MacOsSystemExtensionsPayloadV2Model
// (macOS System Extensions, distinct from the older/legacy KernelExtension
// payload, which stays unmodeled). AllowUserOverrides is a single toggle;
// the two lists are independent allow-list collections — one scoped by
// team identifier + extension TYPE, one scoped by a specific bundle+team
// identifier pair.
type SystemExtensionsModel struct {
	AllowUserOverrides          types.Bool                        `tfsdk:"allow_user_overrides"`
	AllowedSystemExtensionTypes []AllowedSystemExtensionTypeModel `tfsdk:"allowed_system_extension_types"`
	AllowedSystemExtensions     []AllowedSystemExtensionModel     `tfsdk:"allowed_system_extensions"`
}

// AllowedSystemExtensionTypeModel maps to
// MacOsAllowedSystemExtensionTypesV2Model: which system-extension TYPES
// (driver, endpoint security, network) a given team identifier is allowed
// to install, without naming a specific extension.
type AllowedSystemExtensionTypeModel struct {
	TeamIdentifier                     types.String `tfsdk:"team_identifier"`
	AllowDriverExtensionType           types.Bool   `tfsdk:"allow_driver_extension_type"`
	AllowEndpointSecurityExtensionType types.Bool   `tfsdk:"allow_endpoint_security_extension_type"`
	AllowNetworkExtensionType          types.Bool   `tfsdk:"allow_network_extension_type"`
}

// AllowedSystemExtensionModel maps to MacOsAllowedSystemExtensionV2Model: a
// specific system extension (by bundle identifier and/or team identifier)
// that is always approved. Both fields are Optional on the SDK side with
// ambiguous doc coverage of whether at least one is required — see the
// design doc's SDK-gap note; no cross-field validator until live-confirmed.
type AllowedSystemExtensionModel struct {
	BundleIdentifier types.String `tfsdk:"bundle_identifier"`
	TeamIdentifier   types.String `tfsdk:"team_identifier"`
}

// PrivacyPreferenceModel maps to one entry of
// MacOsPrivacyPreferencesPayloadV2Model.Identities
// (MacOsPrivacyPreferencesV2Model): a single PPPC (Privacy Preferences
// Policy Control) rule scoped to one target binary (identified by
// identifier/identifier_type), granting or denying access to a fixed set of
// privacy-sensitive resources plus an optional list of Apple Events it may
// send to other processes.
type PrivacyPreferenceModel struct {
	Identifier                   types.String      `tfsdk:"identifier"`
	IdentifierType               types.String      `tfsdk:"identifier_type"`
	CodeRequirement              types.String      `tfsdk:"code_requirement"`
	Comment                      types.String      `tfsdk:"comment"`
	AppleEventsList              []AppleEventModel `tfsdk:"apple_events_list"`
	StaticCode                   types.Bool        `tfsdk:"static_code"`
	Accessibility                types.String      `tfsdk:"accessibility"`
	AddressBook                  types.String      `tfsdk:"address_book"`
	Calendar                     types.String      `tfsdk:"calendar"`
	Camera                       types.String      `tfsdk:"camera"`
	FileProviderPresence         types.String      `tfsdk:"file_provider_presence"`
	ListenEvent                  types.String      `tfsdk:"listen_event"`
	MediaLibrary                 types.String      `tfsdk:"media_library"`
	Microphone                   types.String      `tfsdk:"microphone"`
	Photos                       types.String      `tfsdk:"photos"`
	PostEvent                    types.String      `tfsdk:"post_event"`
	Reminders                    types.String      `tfsdk:"reminders"`
	ScreenCapture                types.String      `tfsdk:"screen_capture"`
	SpeechRecognition            types.String      `tfsdk:"speech_recognition"`
	SystemPolicyAllFiles         types.String      `tfsdk:"system_policy_all_files"`
	SystemPolicyDesktopFolder    types.String      `tfsdk:"system_policy_desktop_folder"`
	SystemPolicyDocumentsFolder  types.String      `tfsdk:"system_policy_documents_folder"`
	SystemPolicyDownloadsFolder  types.String      `tfsdk:"system_policy_downloads_folder"`
	SystemPolicyNetworkVolumes   types.String      `tfsdk:"system_policy_network_volumes"`
	SystemPolicyRemovableVolumes types.String      `tfsdk:"system_policy_removable_volumes"`
	SystemPolicySysAdminFiles    types.String      `tfsdk:"system_policy_sys_admin_files"`
}

// AppleEventModel maps to AppleEventV2: one entry in a PrivacyPreferenceModel's
// apple_events_list, naming a specific receiver process and whether this
// identity is allowed to send it Apple Events.
type AppleEventModel struct {
	CodeRequirement types.String `tfsdk:"code_requirement"`
	Identifier      types.String `tfsdk:"identifier"`
	IdentifierType  types.String `tfsdk:"identifier_type"`
	Permission      types.String `tfsdk:"permission"`
}

type RestrictionsWidgetsModel struct {
	AllowOnlyConfiguredWidgets types.Bool `tfsdk:"allow_only_configured_widgets"`
	AllowedWidgets             types.List `tfsdk:"allowed_widgets"`
}

// ScepItemModel maps to AppleOsXScepPayloadEntityV2 (terraform-sdk-uem
// internal/mdm/v2/models.go:4238), one macOS SCEP payload. Every field is
// sent by the SDK create/update body and read back, so each is
// Optional+Computed pass-through: no validator or default has a UEM source.
type ScepItemModel struct {
	Name                    types.String                 `tfsdk:"name"`
	CredentialSource        types.String                 `tfsdk:"credential_source"`
	CertificateAuthorityID  types.Int64                  `tfsdk:"certificate_authority_id"`
	CertificateTemplateID   types.Int64                  `tfsdk:"certificate_template_id"`
	AllowExportFromKeyChain types.Bool                   `tfsdk:"allow_export_from_key_chain"`
	IdentityPreference      *ScepIdentityPreferenceModel `tfsdk:"identity_preference"`
}

// ScepIdentityPreferenceModel maps to MacOsScepIdentityPreferencePayloadV2Model
// (models.go:5717).
type ScepIdentityPreferenceModel struct {
	Names types.List `tfsdk:"names"`
}

// WebClipItemModel maps to MacOsWebClipsPayloadV2Entity (terraform-sdk-uem
// internal/mdm/v2/models.go:5822), one macOS Web Clip. All four fields are
// sent by the SDK create/update body and read back as-is. Icon is the id of
// an image already uploaded to UEM; this provider doesn't upload it.
type WebClipItemModel struct {
	Label            types.String `tfsdk:"label"`
	URL              types.String `tfsdk:"url"`
	ShowInAppCatalog types.Bool   `tfsdk:"show_in_app_catalog"`
	Icon             types.Int64  `tfsdk:"icon"`
}

// Generated from the SDK structs (F13): every field is sent by the SDK
// create/update body and read back; the secrets are write-only (UEM returns
// them masked as *****).
// VPNProxyNone is the wire value UEM uses for "no proxy" on a macOS VPN
// payload's Proxy (C# HTTPProxyType). UEM requires Proxy on every create and
// update, for every ConnectionType: [NotNullValidation(ErrorMessage = "VPN.Proxy
// cannot be null")] at AirWatch.ServiceModel/Profiles/V2/Resources/AppleOsX/
// AppleOsXVpnPayloadEntity.cs:466-473, rejecting null, "" and whitespace
// (Framework/Source/AirWatchCore/Validation/NotNullValidationAttribute.cs:43-50).
// But a GET omits Proxy when it is unset (ProfileServiceV2Helper.cs:2091-2095
// skips empty settings; JsonNetFormatter uses NullValueHandling.Ignore), which
// is the normal case for console-created AirwatchTunnel profiles. "None" is
// the picklist value for no proxy (setting 3192, line 1098 of
// deviceProfile.DevicePlatformSettingOption.seed.sql) and the platform default. Same in 26.2 and
// 26.4 (canonical report 2026-09-26, macOS VPN proxy).
const VPNProxyNone = "None"

// VPNItemModel maps to AppleOsXVpnPayloadEntityV2 (terraform-sdk-uem internal/mdm/v2/models.go:4264).
type VPNItemModel struct {
	Account                       types.String              `tfsdk:"account"`
	AppMapping                    types.Bool                `tfsdk:"app_mapping"`
	ApplicationBundleID           types.List                `tfsdk:"application_bundle_id"`
	AssociatedDomains             types.List                `tfsdk:"associated_domains"`
	CalendarDomains               types.List                `tfsdk:"calendar_domains"`
	ConnectAutomatically          types.Bool                `tfsdk:"connect_automatically"`
	ConnectionName                types.String              `tfsdk:"connection_name"`
	ConnectionType                types.String              `tfsdk:"connection_type"`
	ContactsDomains               types.List                `tfsdk:"contacts_domains"`
	CustomDatas                   []VPNItemCustomDatasModel `tfsdk:"custom_datas"`
	EnableSafariDomains           types.Bool                `tfsdk:"enable_safari_domains"`
	EnableVPNOnDemand             types.Bool                `tfsdk:"enable_vpn_on_demand"`
	EncryptionLevel               types.Int64               `tfsdk:"encryption_level"`
	ExcludeLocalNetworks          types.Bool                `tfsdk:"exclude_local_networks"`
	ExcludedDomains               types.List                `tfsdk:"excluded_domains"`
	GroupName                     types.String              `tfsdk:"group_name"`
	IdentityCertificate           types.String              `tfsdk:"identity_certificate"`
	IncludeAllNetworks            types.Bool                `tfsdk:"include_all_networks"`
	IncludeUserPIN                types.Bool                `tfsdk:"include_user_pin"`
	MachineAuthentication         types.Int64               `tfsdk:"machine_authentication"`
	MailDomains                   types.List                `tfsdk:"mail_domains"`
	MdmAssignedID                 types.String              `tfsdk:"mdm_assigned_id"`
	MdmDeviceSerialNumber         types.String              `tfsdk:"mdm_device_serial_number"`
	MdmDeviceUniqueID             types.String              `tfsdk:"mdm_device_unique_id"`
	MdmDeviceWifiMACAddress       types.String              `tfsdk:"mdm_device_wifi_mac_address"`
	Password                      types.String              `tfsdk:"password"`
	PerAppVPN                     types.Bool                `tfsdk:"per_app_vpn"`
	Port                          types.Int64               `tfsdk:"port"`
	PromptForPassword             types.Bool                `tfsdk:"prompt_for_password"`
	ProviderDesignatedRequirement types.String              `tfsdk:"provider_designated_requirement"`
	ProviderType                  types.String              `tfsdk:"provider_type"`
	Proxy                         types.String              `tfsdk:"proxy"`
	ProxyServer                   types.String              `tfsdk:"proxy_server"`
	ProxyServerAutoConfigURL      types.String              `tfsdk:"proxy_server_auto_config_url"`
	SafariDomains                 types.List                `tfsdk:"safari_domains"`
	SendAllTraffic                types.Bool                `tfsdk:"send_all_traffic"`
	Server                        types.String              `tfsdk:"server"`
	SharedSecret                  types.String              `tfsdk:"shared_secret"`
	UseHybridAuthentication       types.Bool                `tfsdk:"use_hybrid_authentication"`
	UserAuthentication            types.String              `tfsdk:"user_authentication"`
	UserName                      types.String              `tfsdk:"user_name"`
	VPNOnDemand                   []VPNItemVPNOnDemandModel `tfsdk:"vpn_on_demand"`
	VPNPassword                   types.String              `tfsdk:"vpn_password"`
	WebLogon                      types.Bool                `tfsdk:"web_logon"`
}

// VPNItemCustomDatasModel maps to CustomDataV2 (terraform-sdk-uem internal/mdm/v2/models.go:5120).
type VPNItemCustomDatasModel struct {
	Key   types.String `tfsdk:"key"`
	Value types.String `tfsdk:"value"`
}

// VPNItemVPNOnDemandModel maps to AppleOsXVpnOnDemandEntityV2 (terraform-sdk-uem internal/mdm/v2/models.go:4255).
type VPNItemVPNOnDemandModel struct {
	Domain         types.String `tfsdk:"domain"`
	OnDemandAction types.String `tfsdk:"on_demand_action"`
}

// EasMicrosoftOutlookModel maps to AppleOsXEasMicrosoftOutlookPayloadEntityV2 (terraform-sdk-uem internal/mdm/v2/models.go:3676).
type EasMicrosoftOutlookModel struct {
	AccountName                types.String `tfsdk:"account_name"`
	DirectoryServer            types.String `tfsdk:"directory_server"`
	DirectoryServerPort        types.String `tfsdk:"directory_server_port"`
	DirectoryServerRequiresSSL types.Bool   `tfsdk:"directory_server_requires_ssl"`
	Domain                     types.String `tfsdk:"domain"`
	EmailAddress               types.String `tfsdk:"email_address"`
	ExchangeHost               types.String `tfsdk:"exchange_host"`
	ExchangePort               types.String `tfsdk:"exchange_port"`
	Password                   types.String `tfsdk:"password"`
	SearchBase                 types.String `tfsdk:"search_base"`
	UseSSL                     types.Bool   `tfsdk:"use_ssl"`
	UserName                   types.String `tfsdk:"user_name"`
}

// KernelExtensionModel maps to MacOsKernelExtensionPayloadV2Entity (terraform-sdk-uem internal/mdm/v2/models.go:5634).
type KernelExtensionModel struct {
	AllowUserOverrides      types.Bool                                    `tfsdk:"allow_user_overrides"`
	AllowedKernelExtensions []KernelExtensionAllowedKernelExtensionsModel `tfsdk:"allowed_kernel_extensions"`
	AllowedTeamIdentifiers  types.List                                    `tfsdk:"allowed_team_identifiers"`
}

// KernelExtensionAllowedKernelExtensionsModel maps to MacOsAllowedKernelExtensionsEntityV2 (terraform-sdk-uem internal/mdm/v2/models.go:5548).
type KernelExtensionAllowedKernelExtensionsModel struct {
	BundleIdentifier types.String `tfsdk:"bundle_identifier"`
	TeamIdentifier   types.String `tfsdk:"team_identifier"`
}

// CustomAttributeModel maps to MacOsCustomAttributePayloadV2Model (terraform-sdk-uem internal/mdm/v2/models.go:5602).
type CustomAttributeModel struct {
	AttributeName   types.String `tfsdk:"attribute_name"`
	AttributeScript types.String `tfsdk:"attribute_script"`
	Events          types.List   `tfsdk:"events"`
	Schedule        types.Int64  `tfsdk:"schedule"`
}
