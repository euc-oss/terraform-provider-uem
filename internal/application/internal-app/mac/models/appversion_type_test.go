package models

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestAppVersionValue_StringSemanticEquals(t *testing.T) {
	unknown := AppVersionValue{StringValue: basetypes.NewStringUnknown()}

	testCases := []struct {
		name  string
		prior AppVersionValue
		new   AppVersionValue
		want  bool
	}{
		{name: "identical", prior: NewAppVersionValue("2.6.22"), new: NewAppVersionValue("2.6.22"), want: true},
		{name: "api padded one .0", prior: NewAppVersionValue("2.6.22"), new: NewAppVersionValue("2.6.22.0"), want: true},
		{name: "api padded two .0", prior: NewAppVersionValue("2.6.22"), new: NewAppVersionValue("2.6.22.0.0"), want: true},
		{name: "padded both sides", prior: NewAppVersionValue("2.6.22.0"), new: NewAppVersionValue("2.6.22.0.0"), want: true},
		{name: "reverse direction", prior: NewAppVersionValue("1.0.0.0"), new: NewAppVersionValue("1.0"), want: true},
		{name: "single zero vs padded", prior: NewAppVersionValue("0"), new: NewAppVersionValue("0.0.0.0"), want: true},
		{name: "different patch", prior: NewAppVersionValue("2.6"), new: NewAppVersionValue("2.6.1"), want: false},
		{name: "different version padded", prior: NewAppVersionValue("2.6.22"), new: NewAppVersionValue("2.6.23.0"), want: false},
		{name: "interior zero is significant", prior: NewAppVersionValue("2.0.1"), new: NewAppVersionValue("2.1"), want: false},
		{name: "trailing 00 is a zero segment", prior: NewAppVersionValue("2.6"), new: NewAppVersionValue("2.6.00"), want: true},
		{name: "leading zero in segment", prior: NewAppVersionValue("1.01"), new: NewAppVersionValue("1.1.0.0"), want: true},
		{name: "leading zero first segment", prior: NewAppVersionValue("01.1"), new: NewAppVersionValue("1.1"), want: true},
		{name: "1.1 vs 1.2", prior: NewAppVersionValue("1.1"), new: NewAppVersionValue("1.2"), want: false},
		{name: "1.10 vs 1.1", prior: NewAppVersionValue("1.10"), new: NewAppVersionValue("1.1"), want: false},
		{name: "overflow segment compares exactly", prior: NewAppVersionValue("99999999999999999999"), new: NewAppVersionValue("99999999999999999999.0"), want: false},
		{name: "overflow segment identical", prior: NewAppVersionValue("99999999999999999999.1"), new: NewAppVersionValue("99999999999999999999.1"), want: true},
		{name: "numeric never equals non-numeric lookalike", prior: NewAppVersionValue("#1"), new: NewAppVersionValue("1"), want: false},
		{name: "non-numeric exact only", prior: NewAppVersionValue("2.6-beta"), new: NewAppVersionValue("2.6-beta.0"), want: false},
		{name: "non-numeric identical", prior: NewAppVersionValue("v2.6.0"), new: NewAppVersionValue("v2.6.0"), want: true},
		{name: "non-numeric not normalized", prior: NewAppVersionValue("v2.6"), new: NewAppVersionValue("v2.6.0"), want: false},
		{name: "empty segment exact only", prior: NewAppVersionValue("2..6"), new: NewAppVersionValue("2..6.0"), want: false},
		{name: "null vs known", prior: NewAppVersionNull(), new: NewAppVersionValue("2.6.22.0"), want: false},
		{name: "known vs null", prior: NewAppVersionValue("2.6.22"), new: NewAppVersionNull(), want: false},
		{name: "unknown vs known", prior: unknown, new: NewAppVersionValue("2.6.22"), want: false},
		{name: "null vs null", prior: NewAppVersionNull(), new: NewAppVersionNull(), want: true},
		{name: "unknown vs unknown", prior: unknown, new: unknown, want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, diags := tc.prior.StringSemanticEquals(context.Background(), tc.new)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if got != tc.want {
				t.Fatalf("StringSemanticEquals(%s, %s) = %v, want %v", tc.prior, tc.new, got, tc.want)
			}
		})
	}
}

func TestAppVersionValue_StringSemanticEquals_WrongTypeErrors(t *testing.T) {
	_, diags := NewAppVersionValue("1.0").StringSemanticEquals(context.Background(), basetypes.NewStringValue("1.0"))
	if !diags.HasError() {
		t.Fatal("expected an error diagnostic for a non-AppVersionValue argument")
	}
}

func TestAppVersionType_RoundTrip(t *testing.T) {
	ctx := context.Background()
	typ := AppVersionType{}

	v, err := typ.ValueFromTerraform(ctx, tftypes.NewValue(tftypes.String, "2.6.22"))
	if err != nil {
		t.Fatalf("ValueFromTerraform: %v", err)
	}
	av, ok := v.(AppVersionValue)
	if !ok {
		t.Fatalf("expected AppVersionValue, got %T", v)
	}
	if av.ValueString() != "2.6.22" {
		t.Fatalf("expected 2.6.22, got %q", av.ValueString())
	}
	if !av.Type(ctx).Equal(typ) {
		t.Fatal("value type does not round-trip to AppVersionType")
	}
	if typ.Equal(basetypes.StringType{}) {
		t.Fatal("AppVersionType must not equal plain StringType")
	}
	if _, ok := typ.ValueType(ctx).(AppVersionValue); !ok {
		t.Fatalf("ValueType returned %T", typ.ValueType(ctx))
	}
	if av.Equal(NewAppVersionValue("2.6.22.0")) {
		t.Fatal("Equal must be exact; normalization belongs only to semantic equality")
	}
}
