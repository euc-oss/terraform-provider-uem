package state

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func KeepStateString(apiVal, stateVal types.String) types.String {
	if apiVal.IsNull() || apiVal.IsUnknown() {
		return stateVal
	}
	return apiVal
}

func KeepStateBool(apiVal, stateVal types.Bool) types.Bool {
	if apiVal.IsNull() || apiVal.IsUnknown() {
		return stateVal
	}
	return apiVal
}

func KeepStateInt64(apiVal, stateVal types.Int64) types.Int64 {
	if apiVal.IsNull() || apiVal.IsUnknown() {
		return stateVal
	}
	return apiVal
}

var appConfigTypeToAPI = map[string]int{
	"STRING":       1,
	"INTEGER":      2,
	"BOOLEAN":      3,
	"CHOICE":       9,
	"MULTISELECT":  10,
	"HIDDEN":       11,
	"BUNDLE":       15,
	"BUNDLE_ARRAY": 16,
}

var appConfigTypeFromAPI = map[int]string{
	1:  "STRING",
	2:  "INTEGER",
	3:  "BOOLEAN",
	9:  "CHOICE",
	10: "MULTISELECT",
	11: "HIDDEN",
	15: "BUNDLE",
	16: "BUNDLE_ARRAY",
}

// appConfigTypeToAPIString maps TF type names to the remote SDK wire type (string).
// Named types become their numeric code as a string (e.g. BOOLEAN -> "3").
func appConfigTypeToAPIString(typeStr string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(typeStr))
	if normalized == "" {
		return "", nil
	}
	if v, ok := appConfigTypeToAPI[normalized]; ok {
		return strconv.Itoa(v), nil
	}
	if _, err := strconv.Atoi(normalized); err == nil {
		return normalized, nil
	}
	return "", fmt.Errorf("unsupported application configuration type %q", typeStr)
}

func appConfigTypeFromAPIString(apiType string) types.String {
	trimmed := strings.TrimSpace(apiType)
	if trimmed == "" {
		return types.StringNull()
	}
	if n, err := strconv.Atoi(trimmed); err == nil {
		if s, ok := appConfigTypeFromAPI[n]; ok {
			return types.StringValue(s)
		}
		return types.StringValue(trimmed)
	}
	upper := strings.ToUpper(trimmed)
	if _, ok := appConfigTypeToAPI[upper]; ok {
		return types.StringValue(upper)
	}
	return types.StringValue(trimmed)
}

func optionalStringToAPI(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

func optionalBoolToAPI(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

func boolFromAPI(v *bool) types.Bool {
	if v == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*v)
}

func int64FromAPIInt(v *int) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*v))
}

// nullIfDefaultFalseBool maps a server-returned *bool restriction/flag value
// into state for an Optional+Computed attribute with no schema Default.
// UEM echoes false for any such flag the request omitted, so when the prior
// plan/state value was null (never configured) and the server sent the
// false default, the null is kept instead of manufacturing an explicit
// false. A non-null prior, or a non-default (true) server value, is always
// taken as-is so real drift surfaces. Live-confirmed 2026-09-23 on as<internal-env>
// 26.2: "Read null-if-default when the prior is null. An explicit default is
// kept when the prior is non-null." Evidence:
// internal-design-doc
func nullIfDefaultFalseBool(apiVal *bool, prior types.Bool) types.Bool {
	v := boolFromAPI(apiVal)
	if v.IsNull() || v.IsUnknown() {
		return v
	}
	if prior.IsNull() && !v.ValueBool() {
		return types.BoolNull()
	}
	return v
}

// appDeliveryMethodFromAPI maps the server's app_delivery_method into state.
// UEM defaults omitted app_delivery_method to "ON_DEMAND" on every full-
// replace PUT; when the prior plan/state value was null (never configured)
// and the server sent that default, the null is kept instead of
// manufacturing an explicit "ON_DEMAND". A non-null prior, or a non-default
// server value, is always taken as-is so real drift surfaces. Live-confirmed
// 2026-09-23 on as<internal-env> 26.2: delivery-method removal gives an identical plan
// alone and with a sibling edit; GET shows ON_DEMAND; the replan is clean.
// Evidence:
// internal-design-doc
//
// The uppercasing itself (B16 decision table row #131, KEEP+cite): UEM
// source: AirWatch API/AW.Mam.Api/AW.Mam.Api.Model/AppDeliveryMethod.cs:32-37,
// AirWatch API/AW.Mam.Api/AW.Mam.Api.Model/Apps/Assignments/AppAssignmentDistributionV1Model.cs:69-71
// (canonical Q15): the wire enum names are already `AUTO`/`ON_DEMAND` (caps),
// and Newtonsoft's StringEnumConverter default is case-sensitive — so this
// uppercasing is redundant in practice (the server always emits the
// canonical caps already) but harmless, and defensive against a
// hypothetical lowercase echo.
func appDeliveryMethodFromAPI(apiVal string, prior types.String) types.String {
	if apiVal == "" {
		return types.StringNull()
	}
	upper := strings.ToUpper(apiVal)
	if prior.IsNull() && upper == "ON_DEMAND" {
		return types.StringNull()
	}
	return types.StringValue(upper)
}
