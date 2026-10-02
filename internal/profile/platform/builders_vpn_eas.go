package platform

import (
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
)

// Builders for the macOS VPN and Microsoft Outlook (EAS) payloads (F13),
// generated field for field from the SDK structs.

func buildVPNItem(it *profilemodels.VPNItemModel) sdk.AppleOsXVpnPayloadEntityV2 {
	var out sdk.AppleOsXVpnPayloadEntityV2
	setStringIfKnown(&out.Account, it.Account)
	out.AppMapping = boolPtrFromTF(it.AppMapping)
	out.ApplicationBundleID = stringSliceFromTFList(it.ApplicationBundleID)
	out.AssociatedDomains = stringSliceFromTFList(it.AssociatedDomains)
	out.CalendarDomains = stringSliceFromTFList(it.CalendarDomains)
	out.ConnectAutomatically = boolPtrFromTF(it.ConnectAutomatically)
	setStringIfKnown(&out.ConnectionName, it.ConnectionName)
	setStringIfKnown(&out.ConnectionType, it.ConnectionType)
	out.ContactsDomains = stringSliceFromTFList(it.ContactsDomains)
	out.CustomDatas = buildVPNItemCustomDatasList(it.CustomDatas)
	out.EnableSafariDomains = boolPtrFromTF(it.EnableSafariDomains)
	out.EnableVPNOnDemand = boolPtrFromTF(it.EnableVPNOnDemand)
	if !it.EncryptionLevel.IsNull() && !it.EncryptionLevel.IsUnknown() {
		out.EncryptionLevel = sdk.IntPtr(int(it.EncryptionLevel.ValueInt64()))
	}
	out.ExcludeLocalNetworks = boolPtrFromTF(it.ExcludeLocalNetworks)
	out.ExcludedDomains = stringSliceFromTFList(it.ExcludedDomains)
	setStringIfKnown(&out.GroupName, it.GroupName)
	setStringIfKnown(&out.IdentityCertificate, it.IdentityCertificate)
	out.IncludeAllNetworks = boolPtrFromTF(it.IncludeAllNetworks)
	out.IncludeUserPIN = boolPtrFromTF(it.IncludeUserPIN)
	if !it.MachineAuthentication.IsNull() && !it.MachineAuthentication.IsUnknown() {
		out.MachineAuthentication = sdk.IntPtr(int(it.MachineAuthentication.ValueInt64()))
	}
	out.MailDomains = stringSliceFromTFList(it.MailDomains)
	setStringIfKnown(&out.MdmAssignedID, it.MdmAssignedID)
	setStringIfKnown(&out.MdmDeviceSerialNumber, it.MdmDeviceSerialNumber)
	setStringIfKnown(&out.MdmDeviceUniqueID, it.MdmDeviceUniqueID)
	setStringIfKnown(&out.MdmDeviceWifiMacAddress, it.MdmDeviceWifiMACAddress)
	setStringIfKnown(&out.Password, it.Password)
	out.PerAppVpn = boolPtrFromTF(it.PerAppVPN)
	if !it.Port.IsNull() && !it.Port.IsUnknown() {
		out.Port = sdk.IntPtr(int(it.Port.ValueInt64()))
	}
	out.PromptForPassword = boolPtrFromTF(it.PromptForPassword)
	setStringIfKnown(&out.ProviderDesignatedRequirement, it.ProviderDesignatedRequirement)
	setStringIfKnown(&out.ProviderType, it.ProviderType)
	setStringIfKnown(&out.Proxy, it.Proxy)
	if out.Proxy == "" {
		// UEM requires Proxy on every write; unset means no proxy
		// (see profilemodels.VPNProxyNone for the source citation).
		out.Proxy = profilemodels.VPNProxyNone
	}
	setStringIfKnown(&out.ProxyServer, it.ProxyServer)
	setStringIfKnown(&out.ProxyServerAutoConfigURL, it.ProxyServerAutoConfigURL)
	out.SafariDomains = stringSliceFromTFList(it.SafariDomains)
	out.SendAllTraffic = boolPtrFromTF(it.SendAllTraffic)
	setStringIfKnown(&out.Server, it.Server)
	setStringIfKnown(&out.SharedSecret, it.SharedSecret)
	out.UseHybridAuthentication = boolPtrFromTF(it.UseHybridAuthentication)
	setStringIfKnown(&out.UserAuthentication, it.UserAuthentication)
	setStringIfKnown(&out.UserName, it.UserName)
	out.VpnOnDemand = buildVPNItemVPNOnDemandList(it.VPNOnDemand)
	setStringIfKnown(&out.VpnPassword, it.VPNPassword)
	out.WebLogon = boolPtrFromTF(it.WebLogon)
	return out
}

func buildVPNItemList(items []profilemodels.VPNItemModel) []sdk.AppleOsXVpnPayloadEntityV2 {
	if len(items) == 0 {
		return nil
	}
	result := make([]sdk.AppleOsXVpnPayloadEntityV2, 0, len(items))
	for i := range items {
		result = append(result, buildVPNItem(&items[i]))
	}
	return result
}
func buildVPNItemCustomDatas(it *profilemodels.VPNItemCustomDatasModel) sdk.CustomDataV2 {
	var out sdk.CustomDataV2
	setStringIfKnown(&out.Key, it.Key)
	setStringIfKnown(&out.Value, it.Value)
	return out
}

func buildVPNItemCustomDatasList(items []profilemodels.VPNItemCustomDatasModel) []sdk.CustomDataV2 {
	if len(items) == 0 {
		return nil
	}
	result := make([]sdk.CustomDataV2, 0, len(items))
	for i := range items {
		result = append(result, buildVPNItemCustomDatas(&items[i]))
	}
	return result
}
func buildVPNItemVPNOnDemand(it *profilemodels.VPNItemVPNOnDemandModel) sdk.AppleOsXVpnOnDemandEntityV2 {
	var out sdk.AppleOsXVpnOnDemandEntityV2
	setStringIfKnown(&out.Domain, it.Domain)
	setStringIfKnown(&out.OnDemandAction, it.OnDemandAction)
	return out
}

func buildVPNItemVPNOnDemandList(items []profilemodels.VPNItemVPNOnDemandModel) []sdk.AppleOsXVpnOnDemandEntityV2 {
	if len(items) == 0 {
		return nil
	}
	result := make([]sdk.AppleOsXVpnOnDemandEntityV2, 0, len(items))
	for i := range items {
		result = append(result, buildVPNItemVPNOnDemand(&items[i]))
	}
	return result
}
func buildEasMicrosoftOutlook(it *profilemodels.EasMicrosoftOutlookModel) sdk.AppleOsXEasMicrosoftOutlookPayloadEntityV2 {
	var out sdk.AppleOsXEasMicrosoftOutlookPayloadEntityV2
	setStringIfKnown(&out.AccountName, it.AccountName)
	setStringIfKnown(&out.DirectoryServer, it.DirectoryServer)
	setStringIfKnown(&out.DirectoryServerPort, it.DirectoryServerPort)
	out.DirectoryServerRequiresSSL = boolPtrFromTF(it.DirectoryServerRequiresSSL)
	setStringIfKnown(&out.Domain, it.Domain)
	setStringIfKnown(&out.EmailAddress, it.EmailAddress)
	setStringIfKnown(&out.ExchangeHost, it.ExchangeHost)
	setStringIfKnown(&out.ExchangePort, it.ExchangePort)
	setStringIfKnown(&out.Password, it.Password)
	setStringIfKnown(&out.SearchBase, it.SearchBase)
	out.UseSSL = boolPtrFromTF(it.UseSSL)
	setStringIfKnown(&out.UserName, it.UserName)
	return out
}
