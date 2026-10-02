package state

import (
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
)

// Read mappers for the macOS VPN and Microsoft Outlook (EAS) payloads (F13),
// field for field. The write-only secrets are never read: UEM returns them
// masked as ***** (live-confirmed on paul-2609, VPN profile and EAS profile
// reads, 2026-09-26), so they are carried from prior state like the
// network_list passwords (carryNetworkPasswords).

// carryVPNSecrets copies password, shared_secret and vpn_password from
// prior state by list position; on import (no prior) they stay null.
func carryVPNSecrets(apiList, prior []profilemodels.VPNItemModel) []profilemodels.VPNItemModel {
	for i := range apiList {
		if i >= len(prior) {
			break
		}
		apiList[i].Password = knownOrNull(prior[i].Password)
		apiList[i].SharedSecret = knownOrNull(prior[i].SharedSecret)
		apiList[i].VPNPassword = knownOrNull(prior[i].VPNPassword)
	}
	return apiList
}

// carryEasMicrosoftOutlookPassword copies password from prior state.
func carryEasMicrosoftOutlookPassword(api, prior *profilemodels.EasMicrosoftOutlookModel) *profilemodels.EasMicrosoftOutlookModel {
	if api != nil && prior != nil {
		api.Password = knownOrNull(prior.Password)
	}
	return api
}

func mapVPNItem(it *sdk.AppleOsXVpnPayloadEntityV2) profilemodels.VPNItemModel {
	return profilemodels.VPNItemModel{
		Account:                       stringToTF(it.Account),
		AppMapping:                    boolPtrToTF(it.AppMapping),
		ApplicationBundleID:           stringSliceToTFList(it.ApplicationBundleID),
		AssociatedDomains:             stringSliceToTFList(it.AssociatedDomains),
		CalendarDomains:               stringSliceToTFList(it.CalendarDomains),
		ConnectAutomatically:          boolPtrToTF(it.ConnectAutomatically),
		ConnectionName:                stringToTF(it.ConnectionName),
		ConnectionType:                stringToTF(it.ConnectionType),
		ContactsDomains:               stringSliceToTFList(it.ContactsDomains),
		CustomDatas:                   mapVPNItemCustomDatasList(it.CustomDatas),
		EnableSafariDomains:           boolPtrToTF(it.EnableSafariDomains),
		EnableVPNOnDemand:             boolPtrToTF(it.EnableVPNOnDemand),
		EncryptionLevel:               int64PtrToTF(it.EncryptionLevel),
		ExcludeLocalNetworks:          boolPtrToTF(it.ExcludeLocalNetworks),
		ExcludedDomains:               stringSliceToTFList(it.ExcludedDomains),
		GroupName:                     stringToTF(it.GroupName),
		IdentityCertificate:           stringToTF(it.IdentityCertificate),
		IncludeAllNetworks:            boolPtrToTF(it.IncludeAllNetworks),
		IncludeUserPIN:                boolPtrToTF(it.IncludeUserPIN),
		MachineAuthentication:         int64PtrToTF(it.MachineAuthentication),
		MailDomains:                   stringSliceToTFList(it.MailDomains),
		MdmAssignedID:                 stringToTF(it.MdmAssignedID),
		MdmDeviceSerialNumber:         stringToTF(it.MdmDeviceSerialNumber),
		MdmDeviceUniqueID:             stringToTF(it.MdmDeviceUniqueID),
		MdmDeviceWifiMACAddress:       stringToTF(it.MdmDeviceWifiMacAddress),
		PerAppVPN:                     boolPtrToTF(it.PerAppVpn),
		Port:                          int64PtrToTF(it.Port),
		PromptForPassword:             boolPtrToTF(it.PromptForPassword),
		ProviderDesignatedRequirement: stringToTF(it.ProviderDesignatedRequirement),
		ProviderType:                  stringToTF(it.ProviderType),
		Proxy:                         vpnProxyToTF(it.Proxy),
		ProxyServer:                   stringToTF(it.ProxyServer),
		ProxyServerAutoConfigURL:      stringToTF(it.ProxyServerAutoConfigURL),
		SafariDomains:                 stringSliceToTFList(it.SafariDomains),
		SendAllTraffic:                boolPtrToTF(it.SendAllTraffic),
		Server:                        stringToTF(it.Server),
		UseHybridAuthentication:       boolPtrToTF(it.UseHybridAuthentication),
		UserAuthentication:            stringToTF(it.UserAuthentication),
		UserName:                      stringToTF(it.UserName),
		VPNOnDemand:                   mapVPNItemVPNOnDemandList(it.VpnOnDemand),
		WebLogon:                      boolPtrToTF(it.WebLogon),
	}
}

func mapVPNItemList(items []sdk.AppleOsXVpnPayloadEntityV2) []profilemodels.VPNItemModel {
	if len(items) == 0 {
		return nil
	}
	result := make([]profilemodels.VPNItemModel, 0, len(items))
	for i := range items {
		result = append(result, mapVPNItem(&items[i]))
	}
	return result
}

func mapVPNItemCustomDatas(it *sdk.CustomDataV2) profilemodels.VPNItemCustomDatasModel {
	return profilemodels.VPNItemCustomDatasModel{
		Key:   stringToTF(it.Key),
		Value: stringToTF(it.Value),
	}
}

func mapVPNItemCustomDatasList(items []sdk.CustomDataV2) []profilemodels.VPNItemCustomDatasModel {
	if len(items) == 0 {
		return nil
	}
	result := make([]profilemodels.VPNItemCustomDatasModel, 0, len(items))
	for i := range items {
		result = append(result, mapVPNItemCustomDatas(&items[i]))
	}
	return result
}

func mapVPNItemVPNOnDemand(it *sdk.AppleOsXVpnOnDemandEntityV2) profilemodels.VPNItemVPNOnDemandModel {
	return profilemodels.VPNItemVPNOnDemandModel{
		Domain:         stringToTF(it.Domain),
		OnDemandAction: stringToTF(it.OnDemandAction),
	}
}

func mapVPNItemVPNOnDemandList(items []sdk.AppleOsXVpnOnDemandEntityV2) []profilemodels.VPNItemVPNOnDemandModel {
	if len(items) == 0 {
		return nil
	}
	result := make([]profilemodels.VPNItemVPNOnDemandModel, 0, len(items))
	for i := range items {
		result = append(result, mapVPNItemVPNOnDemand(&items[i]))
	}
	return result
}

func mapEasMicrosoftOutlook(it *sdk.AppleOsXEasMicrosoftOutlookPayloadEntityV2) profilemodels.EasMicrosoftOutlookModel {
	return profilemodels.EasMicrosoftOutlookModel{
		AccountName:                stringToTF(it.AccountName),
		DirectoryServer:            stringToTF(it.DirectoryServer),
		DirectoryServerPort:        stringToTF(it.DirectoryServerPort),
		DirectoryServerRequiresSSL: boolPtrToTF(it.DirectoryServerRequiresSSL),
		Domain:                     stringToTF(it.Domain),
		EmailAddress:               stringToTF(it.EmailAddress),
		ExchangeHost:               stringToTF(it.ExchangeHost),
		ExchangePort:               stringToTF(it.ExchangePort),
		SearchBase:                 stringToTF(it.SearchBase),
		UseSSL:                     boolPtrToTF(it.UseSSL),
		UserName:                   stringToTF(it.UserName),
	}
}

func mapEasMicrosoftOutlookPtr(it *sdk.AppleOsXEasMicrosoftOutlookPayloadEntityV2) *profilemodels.EasMicrosoftOutlookModel {
	if it == nil {
		return nil
	}
	m := mapEasMicrosoftOutlook(it)
	return &m
}

// vpnProxyToTF maps an absent Proxy (UEM omits it when unset) to "None", the
// value the provider sends for an unset proxy, so a read never drifts from
// what was written. See profilemodels.VPNProxyNone for the source citation.
func vpnProxyToTF(p string) types.String {
	if p == "" {
		return types.StringValue(profilemodels.VPNProxyNone)
	}
	return types.StringValue(p)
}

// knownOrNull returns v, or null when v is unknown. On create the prior
// value is the plan, where a secret the configuration leaves unset is
// unknown (it is Computed); state must never hold an unknown after apply.
func knownOrNull(v types.String) types.String {
	if v.IsUnknown() {
		return types.StringNull()
	}
	return v
}
