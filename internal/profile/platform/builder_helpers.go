package platform

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// boolPtrFromTF converts a Terraform types.Bool into *bool. Returns nil when
// the value is null/unknown so the JSON encoder omits the field.
func boolPtrFromTF(b types.Bool) *bool {
	if b.IsNull() || b.IsUnknown() {
		return nil
	}
	v := b.ValueBool()
	return &v
}

// setStringIfKnown writes the value of a Terraform types.String into dst when
// the value is known and non-null. Otherwise dst is left untouched (zero value).
func setStringIfKnown(dst *string, s types.String) {
	if s.IsNull() || s.IsUnknown() {
		return
	}
	*dst = s.ValueString()
}

// stringSliceFromTFList converts a Terraform types.List of strings to a Go
// []string. Returns nil when the list is null/unknown or empty so the JSON
// encoder omits the field.
func stringSliceFromTFList(l types.List) []string {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	elems := l.Elements()
	if len(elems) == 0 {
		return nil
	}
	out := make([]string, 0, len(elems))
	for _, e := range elems {
		if s, ok := e.(types.String); ok && !s.IsNull() && !s.IsUnknown() {
			out = append(out, s.ValueString())
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
