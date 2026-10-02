package macapplication

import (
	"encoding/base64"
	"strings"
	"testing"
)

const basePlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>name</key>
	<string>Example App</string>
	<key>version</key>
	<string>2.6.22</string>
	<key>installer_item_size</key>
	<integer>17</integer>
	<key>uninstallable</key>
	<true/>
	<key>ratio</key>
	<real>1.5</real>
	<key>icon</key>
	<data>
	aGVsbG8g
	d29ybGQ=
	</data>
	<key>created</key>
	<date>2026-09-23T00:00:00Z</date>
	<key>receipts</key>
	<array>
		<dict>
			<key>bundle_id</key>
			<string>com.example.a</string>
			<key>installed_size</key>
			<integer>1</integer>
		</dict>
		<string>second</string>
	</array>
</dict>
</plist>
`

// keyOrderVariant is basePlist with every <dict> (outer and nested) in a
// different key order, a different declaration and no DOCTYPE, compacted
// whitespace and a comment.
const keyOrderVariant = `<?xml version='1.0'?><plist version="1.0"><dict>` +
	`<key>receipts</key><array><dict><key>installed_size</key><integer>1</integer>` +
	`<key>bundle_id</key><string>com.example.a</string></dict><string>second</string></array>` +
	`<!-- reordered --><key>uninstallable</key><true/>` +
	`<key>version</key><string>2.6.22</string><key>icon</key><data>aGVsbG8gd29ybGQ=</data>` +
	`<key>created</key><date> 2026-09-23T00:00:00Z </date>` +
	`<key>ratio</key><real>1.5</real><key>name</key><string>Example App</string>` +
	`<key>installer_item_size</key><integer> 17 </integer></dict></plist>`

func mustCanonicalSHA(t *testing.T, s string) string {
	t.Helper()
	sum, err := canonicalPlistSHA256([]byte(s))
	if err != nil {
		t.Fatalf("canonicalPlistSHA256: %v", err)
	}
	return sum
}

func TestCanonicalPlist_EquivalentForms(t *testing.T) {
	base := mustCanonicalSHA(t, basePlist)

	testCases := []struct {
		name    string
		variant string
	}{
		{name: "identical", variant: basePlist},
		{name: "whitespace reformatted", variant: strings.NewReplacer("\n", "\r\n    ", "\t", "  ").Replace(basePlist)},
		{name: "no xml declaration", variant: strings.SplitN(basePlist, "\n", 2)[1]},
		{name: "no doctype", variant: strings.Replace(basePlist, `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`, "", 1)},
		{name: "key order, declaration, compaction", variant: keyOrderVariant},
		{name: "CDATA vs entity-encoded string", variant: strings.Replace(basePlist, "<string>Example App</string>", "<string><![CDATA[Example App]]></string>", 1)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mustCanonicalSHA(t, tc.variant); got != base {
				t.Fatalf("canonical sha differs from base:\n got %s\nwant %s", got, base)
			}
		})
	}
	t.Run("CDATA vs entity-encoded special characters", func(t *testing.T) {
		a := mustCanonicalSHA(t, strings.Replace(basePlist, "<string>Example App</string>", "<string>a &lt;b&gt; &amp; c</string>", 1))
		b := mustCanonicalSHA(t, strings.Replace(basePlist, "<string>Example App</string>", "<string><![CDATA[a <b> & c]]></string>", 1))
		if a != b {
			t.Fatal("CDATA and entity-encoded forms of the same text must canonicalize identically")
		}
	})
	t.Run("self-closing string equals empty string", func(t *testing.T) {
		a := mustCanonicalSHA(t, strings.Replace(basePlist, "<string>second</string>", "<string></string>", 1))
		b := mustCanonicalSHA(t, strings.Replace(basePlist, "<string>second</string>", "<string/>", 1))
		if a != b {
			t.Fatal("<string></string> and <string/> must canonicalize identically")
		}
	})
}

func TestCanonicalPlist_ValueChangesDiffer(t *testing.T) {
	base := mustCanonicalSHA(t, basePlist)

	testCases := []struct {
		name string
		old  string
		new  string
	}{
		{name: "string value", old: "<string>2.6.22</string>", new: "<string>2.6.23</string>"},
		{name: "integer value", old: "<integer>17</integer>", new: "<integer>18</integer>"},
		{name: "bool value", old: "<true/>", new: "<false/>"},
		{name: "real value", old: "<real>1.5</real>", new: "<real>1.25</real>"},
		{name: "data value", old: "d29ybGQ=", new: "d29ybGQh"},
		{name: "date value", old: "2026-09-23T00:00:00Z", new: "2026-09-24T00:00:00Z"},
		{name: "array element added", old: "<string>second</string>", new: "<string>second</string><string>third</string>"},
		{name: "extra key", old: "<key>name</key>", new: "<key>extra</key><string>x</string><key>name</key>"},
		{name: "key renamed", old: "<key>ratio</key>", new: "<key>ratio2</key>"},
		{name: "string inner whitespace", old: "<string>Example App</string>", new: "<string>Example  App</string>"},
		{name: "string leading/trailing whitespace", old: "<string>Example App</string>", new: "<string>  Example App  </string>"},
		{name: "string newline padding", old: "<string>2.6.22</string>", new: "<string>\n\t2.6.22\n</string>"},
		{name: "key whitespace", old: "<key>ratio</key>", new: "<key>ratio </key>"},
		{name: "value type changed", old: "<integer>17</integer>", new: "<string>17</string>"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(basePlist, tc.old) {
				t.Fatalf("fixture does not contain %q", tc.old)
			}
			if got := mustCanonicalSHA(t, strings.Replace(basePlist, tc.old, tc.new, 1)); got == base {
				t.Fatalf("expected a different canonical sha after %s change", tc.name)
			}
		})
	}

	t.Run("array element order swapped", func(t *testing.T) {
		swapped := strings.Replace(basePlist,
			"<array>\n\t\t<dict>\n\t\t\t<key>bundle_id</key>\n\t\t\t<string>com.example.a</string>\n\t\t\t<key>installed_size</key>\n\t\t\t<integer>1</integer>\n\t\t</dict>\n\t\t<string>second</string>\n\t</array>",
			"<array><string>second</string><dict><key>bundle_id</key><string>com.example.a</string><key>installed_size</key><integer>1</integer></dict></array>", 1)
		if swapped == basePlist {
			t.Fatal("fixture replacement did not apply")
		}
		if mustCanonicalSHA(t, swapped) == base {
			t.Fatal("array order must be significant")
		}
	})
}

func TestCanonicalPlist_RejectsNonXML(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{name: "binary plist", input: "bplist00\xd1\x01\x02Q"},
		{name: "base64 of binary plist", input: base64.StdEncoding.EncodeToString([]byte("bplist00\xd1\x01\x02Q"))},
		{name: "plain text", input: "not a plist at all"},
		{name: "empty", input: ""},
		{name: "declaration only", input: `<?xml version="1.0"?>`},
		{name: "malformed xml", input: "<plist><dict><key>a</key></plist>"},
		{name: "two roots", input: "<plist/><plist/>"},
		{name: "unpaired dict key", input: "<plist><dict><key>a</key></dict></plist>"},
		{name: "dict value without key", input: "<plist><dict><string>a</string><string>b</string></dict></plist>"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := canonicalPlistSHA256([]byte(tc.input)); err == nil {
				t.Fatalf("expected an error canonicalizing %q", tc.input)
			}
		})
	}
}
