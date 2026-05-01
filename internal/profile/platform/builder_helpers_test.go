package platform

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestBoolPtrFromTF(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   types.Bool
		want *bool
	}{
		{"null returns nil", types.BoolNull(), nil},
		{"unknown returns nil", types.BoolUnknown(), nil},
		{"true returns &true", types.BoolValue(true), boolPtr(true)},
		{"false returns &false", types.BoolValue(false), boolPtr(false)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := boolPtrFromTF(tc.in)
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("got %v, want nil", *got)
			case tc.want != nil && got == nil:
				t.Fatalf("got nil, want %v", *tc.want)
			case tc.want != nil && got != nil && *tc.want != *got:
				t.Fatalf("got %v, want %v", *got, *tc.want)
			}
		})
	}
}

func TestSetStringIfKnown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		initial string
		in      types.String
		want    string
	}{
		{"null leaves dst untouched", "preset", types.StringNull(), "preset"},
		{"unknown leaves dst untouched", "preset", types.StringUnknown(), "preset"},
		{"known overwrites dst", "preset", types.StringValue("new"), "new"},
		{"empty known overwrites dst", "preset", types.StringValue(""), ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dst := tc.initial
			setStringIfKnown(&dst, tc.in)
			if dst != tc.want {
				t.Fatalf("dst = %q, want %q", dst, tc.want)
			}
		})
	}
}

func TestStringSliceFromTFList(t *testing.T) {
	t.Parallel()

	t.Run("null returns nil", func(t *testing.T) {
		t.Parallel()
		if got := stringSliceFromTFList(types.ListNull(types.StringType)); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("unknown returns nil", func(t *testing.T) {
		t.Parallel()
		if got := stringSliceFromTFList(types.ListUnknown(types.StringType)); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("empty returns nil", func(t *testing.T) {
		t.Parallel()
		l, diags := types.ListValue(types.StringType, []attr.Value{})
		if diags.HasError() {
			t.Fatalf("ListValue diags: %v", diags)
		}
		if got := stringSliceFromTFList(l); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("populated list returns slice", func(t *testing.T) {
		t.Parallel()
		l := mustStringList(t, "alpha", "beta", "gamma")
		got := stringSliceFromTFList(l)
		want := []string{"alpha", "beta", "gamma"}
		if !equalStringSlices(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("list of all-null elements returns nil", func(t *testing.T) {
		t.Parallel()
		l, diags := types.ListValue(types.StringType, []attr.Value{
			types.StringNull(), types.StringNull(),
		})
		if diags.HasError() {
			t.Fatalf("ListValue diags: %v", diags)
		}
		if got := stringSliceFromTFList(l); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})
}

func boolPtr(b bool) *bool { return &b }

func mustStringList(t *testing.T, vals ...string) types.List {
	t.Helper()
	elems := make([]attr.Value, 0, len(vals))
	for _, v := range vals {
		elems = append(elems, types.StringValue(v))
	}
	l, diags := types.ListValue(types.StringType, elems)
	if diags.HasError() {
		t.Fatalf("ListValue diags: %v", diags)
	}
	return l
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
