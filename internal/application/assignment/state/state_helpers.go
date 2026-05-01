package state

import (
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
