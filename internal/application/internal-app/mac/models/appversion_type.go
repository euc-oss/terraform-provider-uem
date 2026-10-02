package models

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// AppVersionType is the custom string type for uem_mac_application's
// app_version. UEM stores the version the provider sends at create time
// reformatted as a four-segment .NET-style version (a configured "2.6.22"
// reads back as AirwatchAppVersion "2.6.22.0", "1.01" as "1.1.0.0"), so
// AppVersionValue treats numerically equivalent versions as semantically
// equal (see AppVersionsEquivalent). That lets Read
// populate app_version from the API (so import recovers it) without producing
// a diff against the configured, unpadded value.
type AppVersionType struct {
	basetypes.StringType
}

var _ basetypes.StringTypable = AppVersionType{}

func (t AppVersionType) Equal(o attr.Type) bool {
	other, ok := o.(AppVersionType)
	if !ok {
		return false
	}
	return t.StringType.Equal(other.StringType)
}

func (t AppVersionType) String() string {
	return "AppVersionType"
}

func (t AppVersionType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return AppVersionValue{StringValue: in}, nil
}

func (t AppVersionType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	attrValue, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	stringValue, ok := attrValue.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type of %T", attrValue)
	}
	stringValuable, diags := t.ValueFromString(ctx, stringValue)
	if diags.HasError() {
		return nil, fmt.Errorf("unexpected error converting StringValue to StringValuable: %v", diags)
	}
	return stringValuable, nil
}

func (t AppVersionType) ValueType(_ context.Context) attr.Value {
	return AppVersionValue{}
}

// AppVersionValue is the value type for AppVersionType.
type AppVersionValue struct {
	basetypes.StringValue
}

var _ basetypes.StringValuableWithSemanticEquals = AppVersionValue{}

func (v AppVersionValue) Type(_ context.Context) attr.Type {
	return AppVersionType{}
}

func (v AppVersionValue) Equal(o attr.Value) bool {
	other, ok := o.(AppVersionValue)
	if !ok {
		return false
	}
	return v.StringValue.Equal(other.StringValue)
}

// StringSemanticEquals reports whether two known app versions are the same
// version (see AppVersionsEquivalent). Null and
// unknown values are never semantically equal to anything; the framework only
// consults this for differing known values, so identical values never reach
// here as a "change".
func (v AppVersionValue) StringSemanticEquals(_ context.Context, newValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	newValue, ok := newValuable.(AppVersionValue)
	if !ok {
		diags.AddError(
			"Semantic Equality Check Error",
			fmt.Sprintf("An unexpected value type was received while performing semantic equality checks. "+
				"Expected Value Type: %T, Got Value Type: %T. Please report this to the provider developers.", v, newValuable),
		)
		return false, diags
	}

	if v.IsNull() || v.IsUnknown() || newValue.IsNull() || newValue.IsUnknown() {
		return v.Equal(newValue), diags
	}

	return AppVersionsEquivalent(v.ValueString(), newValue.ValueString()), diags
}

// AppVersionsEquivalent reports whether two known app version strings name
// the same version. Purely numeric dotted versions compare as integer
// segments with trailing zero segments ignored — the way UEM pads and
// reformats them: "1.0.0" ≡ "1.0.0.0", "1.01" ≡ "1.1.0.0". Anything else
// compares exactly. It is the single comparison shared by AppVersionValue's
// semantic equality (Read / Create) and app_version's RequiresReplaceIf plan
// modifier (plan time, where semantic equality does not run).
//
// UEM source (B16 decision table row #106; CORRECTS a prior, wrong citation
// to .NET System.Version): AirWatch API/AW.Mam.Api/AW.Mam.Api/Controllers/macOS/Apps/V1/MacOsAppsV1Controller.cs:114-120,
// BusinessImpl/src/BusinessImplSln/WanderingWiFi.AirWatch.BusinessImpl/Device/MacOs/SFD/AppleMacOsApplicationBusiness.cs:252-260,
// Framework/Core/WanderingWiFi.AirWatch.Framework.Entity/Version.cs:282-286
// (canonical Q12): the parsed/stored type is
// WanderingWiFi.AirWatch.Framework.Entity.Version, not System.Version,
// parsed with AppsConstants.FourthDecimalPlace (parse depth 4). Its
// ToString() (echoed back as AirwatchAppVersion) yields 3 segments
// ("Major.Minor.Build") when Revision is null, or 4
// ("Major.Minor.Build.Revision") when it is set — the business layer's
// AppleMacOsApplicationBusiness always constructs the entity with an
// explicit (defaulted) Revision, which is consistent with the
// live-confirmed 4-segment padding this equivalence check tolerates
// ("1.0.0" reads back as "1.0.0.0"). This equivalence logic's own semantics
// (trailing-zero-segment-insensitive integer comparison) are unaffected by
// the corrected citation and remain live-confirmed 2026-09-23 on as<internal-env>
// 26.2. Evidence: internal-design-doc
func AppVersionsEquivalent(a, b string) bool {
	na, aNumeric := numericAppVersion(a)
	nb, bNumeric := numericAppVersion(b)
	if !aNumeric || !bNumeric {
		return a == b
	}
	if len(na) != len(nb) {
		return false
	}
	for i := range na {
		if na[i] != nb[i] {
			return false
		}
	}
	return true
}

// numericAppVersion parses a purely numeric dotted version into unsigned
// integer segments (dropping leading zeros) with trailing zero segments
// removed, keeping at least one ("2.06.22.0" -> [2 6 22]). It reports false
// for any version with a non-numeric or empty segment, or a segment overflowing
// uint64; such versions only ever compare exactly.
func numericAppVersion(s string) (segments []uint64, ok bool) {
	parts := strings.Split(s, ".")
	nums := make([]uint64, len(parts))
	for i, seg := range parts {
		if seg == "" || strings.Trim(seg, "0123456789") != "" {
			return nil, false
		}
		n, err := strconv.ParseUint(seg, 10, 64)
		if err != nil {
			return nil, false
		}
		nums[i] = n
	}
	for len(nums) > 1 && nums[len(nums)-1] == 0 {
		nums = nums[:len(nums)-1]
	}
	return nums, true
}

// NewAppVersionValue returns a known AppVersionValue.
func NewAppVersionValue(s string) AppVersionValue {
	return AppVersionValue{StringValue: basetypes.NewStringValue(s)}
}

// NewAppVersionNull returns a null AppVersionValue.
func NewAppVersionNull() AppVersionValue {
	return AppVersionValue{StringValue: basetypes.NewStringNull()}
}
