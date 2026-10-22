package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rsschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// repoRootFromThisFile resolves the repository root relative to this test
// file (internal/provider/docs_data_sources_test.go), so the test works
// regardless of the working directory `go test` is invoked from.
func repoRootFromThisFile(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to determine this test file's path via runtime.Caller")
	}
	// this file lives at <repoRoot>/internal/provider/docs_data_sources_test.go
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

// collectAttributeNames walks a data source schema's attributes recursively
// (into ListNestedAttribute / SetNestedAttribute / MapNestedAttribute /
// SingleNestedAttribute nested objects) and records every attribute name
// encountered at any depth into out.
func collectAttributeNames(attrs map[string]dsschema.Attribute, out map[string]bool) {
	for name, attr := range attrs {
		out[name] = true
		switch a := attr.(type) {
		case dsschema.ListNestedAttribute:
			collectAttributeNames(a.NestedObject.Attributes, out)
		case dsschema.SetNestedAttribute:
			collectAttributeNames(a.NestedObject.Attributes, out)
		case dsschema.MapNestedAttribute:
			collectAttributeNames(a.NestedObject.Attributes, out)
		case dsschema.SingleNestedAttribute:
			collectAttributeNames(a.Attributes, out)
		}
	}
}

// TestDataSourceDocsExistAndCoverSchema is a guard test for internal-task: every
// data source registered in the provider's DataSources() list must have a
// docs/data-sources/<name>.md page and an examples/data-sources/<TypeName>/
// data-source.tf example, and that doc page must mention every attribute the
// data source's real Schema() produces (recursively, including attributes
// nested inside Attributes List/Set/Map/Single objects) as a Markdown code
// span (backtick-quoted, e.g. `name`) -- the same convention tfplugindocs
// itself uses to render attribute names.
//
// This is deliberately driven off the REAL, REGISTERED provider
// (New("test")().DataSources(ctx)) and the REAL Schema() each data source
// returns, not a hand-maintained list of data source names/attributes, so it
// fails the moment a data source's schema changes without its doc page being
// updated to match, or a doc page/example is deleted.
func TestDataSourceDocsExistAndCoverSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	repoRoot := repoRootFromThisFile(t)

	p := New("test")()
	ctors := p.DataSources(ctx)
	if len(ctors) == 0 {
		t.Fatal("provider.DataSources() returned zero data sources; nothing to check")
	}

	for _, ctor := range ctors {
		ds := ctor()

		var metaResp datasource.MetadataResponse
		ds.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "uem"}, &metaResp)
		typeName := metaResp.TypeName
		if typeName == "" {
			t.Fatalf("a data source constructor returned an empty TypeName from Metadata()")
		}

		t.Run(typeName, func(t *testing.T) {
			t.Parallel()

			var schemaResp datasource.SchemaResponse
			ds.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
			if schemaResp.Diagnostics.HasError() {
				t.Fatalf("data source %s: Schema() produced diagnostics errors: %s", typeName, schemaResp.Diagnostics)
			}

			// --- docs/data-sources/<name>.md must exist and mention every
			// attribute name (recursively) as a `name` code span. ---
			docName := strings.TrimPrefix(typeName, "uem_")
			docPath := filepath.Join(repoRoot, "docs", "data-sources", docName+".md")
			docBytes, err := os.ReadFile(docPath)
			if err != nil {
				t.Fatalf("data source %s: expected docs page %s to exist, but it does not (or could not be read): %s",
					typeName, docPath, err)
			}
			docContent := string(docBytes)

			attrNames := map[string]bool{}
			collectAttributeNames(schemaResp.Schema.Attributes, attrNames)
			if len(attrNames) == 0 {
				t.Fatalf("data source %s: Schema() returned zero attributes", typeName)
			}

			for _, name := range missingSchemaAttributes(docContent, attrNames) {
				t.Errorf("data source %s: docs page %s is missing attribute %q (expected a \"- `%s`\" item after the ## Schema heading)",
					typeName, docPath, name, name)
			}

			// --- examples/data-sources/<TypeName>/data-source.tf must exist
			// and reference this data source by its real TypeName. ---
			examplePath := filepath.Join(repoRoot, "examples", "data-sources", typeName, "data-source.tf")
			exampleBytes, err := os.ReadFile(examplePath)
			if err != nil {
				t.Fatalf("data source %s: expected example file %s to exist, but it does not (or could not be read): %s",
					typeName, examplePath, err)
			}
			exampleContent := string(exampleBytes)

			dataRef := fmt.Sprintf("data %q", typeName)
			if !strings.Contains(exampleContent, dataRef) {
				t.Errorf("data source %s: example file %s does not contain a %s block",
					typeName, examplePath, dataRef)
			}

			// --- the doc page's "## Example Usage" fenced code block must
			// byte-match examples/data-sources/<TypeName>/data-source.tf
			// (after normalizing CRLF to LF and trimming a trailing newline
			// on both sides); see the resource-side check in
			// TestResourceDocsExistAndCoverSchema for why. ---
			block, hasBlock := exampleUsageCodeBlock(docContent)
			if !hasBlock {
				t.Errorf("data source %s: docs page %s has no \"## Example Usage\" fenced code block",
					typeName, docPath)
			} else if got, want := normalizeExampleContent(block), normalizeExampleContent(exampleContent); got != want {
				t.Errorf("data source %s: docs page %s \"## Example Usage\" block does not match example file %s (compared after normalizing CRLF to LF and trimming a trailing newline)\n--- doc block ---\n%s\n--- example file ---\n%s",
					typeName, docPath, examplePath, got, want)
			}
		})
	}
}

// missingSchemaAttributes returns, sorted, the attribute names that have no
// "- `name`" list item after the page's "## Schema" heading. Mentions in the
// intro prose or examples above the schema do not count, so deleting a schema
// bullet fails even when the prose still names the attribute.
func missingSchemaAttributes(page string, names map[string]bool) []string {
	// Normalize CRLF to LF before searching: a couple of doc pages
	// (purchased_application_assignment.md, update_deployment.md) are CRLF
	// on disk, and the "\n## Schema\n" search below would otherwise never
	// match "\r\n## Schema\r\n", making every attribute look missing. This
	// only affects the in-memory string used for the search below; it does
	// not touch the file on disk.
	page = strings.ReplaceAll(page, "\r\n", "\n")

	schema := ""
	if i := strings.Index(page, "\n## Schema\n"); i >= 0 {
		schema = page[i:]
	}
	listed := map[string]bool{}
	for _, line := range strings.Split(schema, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- `") {
			continue
		}
		rest := line[len("- `"):]
		if j := strings.Index(rest, "`"); j > 0 {
			listed[rest[:j]] = true
		}
	}
	var missing []string
	for name := range names {
		if !listed[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

// collectResourceAttributeNames walks a RESOURCE schema's attributes and
// blocks recursively (into ListNestedAttribute / SetNestedAttribute /
// MapNestedAttribute / SingleNestedAttribute nested objects, and into
// ListNestedBlock / SetNestedBlock / SingleNestedBlock nested objects) and
// records every attribute/block name encountered at any depth into out. This
// is the resource/schema counterpart to collectAttributeNames (which walks
// datasource/schema); resource schemas additionally support Blocks, which
// datasource schemas do not have.
func collectResourceAttributeNames(attrs map[string]rsschema.Attribute, blocks map[string]rsschema.Block, out map[string]bool) {
	for name, attr := range attrs {
		out[name] = true
		switch a := attr.(type) {
		case rsschema.ListNestedAttribute:
			collectResourceAttributeNames(a.NestedObject.Attributes, nil, out)
		case rsschema.SetNestedAttribute:
			collectResourceAttributeNames(a.NestedObject.Attributes, nil, out)
		case rsschema.MapNestedAttribute:
			collectResourceAttributeNames(a.NestedObject.Attributes, nil, out)
		case rsschema.SingleNestedAttribute:
			collectResourceAttributeNames(a.Attributes, nil, out)
		}
	}
	for name, block := range blocks {
		out[name] = true
		switch b := block.(type) {
		case rsschema.ListNestedBlock:
			collectResourceAttributeNames(b.NestedObject.Attributes, b.NestedObject.Blocks, out)
		case rsschema.SetNestedBlock:
			collectResourceAttributeNames(b.NestedObject.Attributes, b.NestedObject.Blocks, out)
		case rsschema.SingleNestedBlock:
			collectResourceAttributeNames(b.Attributes, b.Blocks, out)
		}
	}
}

// importIDShapes maps each resource TypeName that implements
// resource.ResourceWithImportState to a regexp describing the shape of the
// import ID its ImportState method accepts. This is a cheap, hand-maintained
// check (not driven off the real ImportState code), so every entry names the
// ImportState implementation it was read from; a registered importable
// resource with no entry here fails TestResourceDocsExistAndCoverSchema, so
// a new importable resource cannot ship without someone adding (and
// justifying) a shape row.
var importIDShapes = map[string]*regexp.Regexp{
	// internal/profile/resource_crud.go ImportState: splits on ":" and
	// requires exactly "<profile_id>:<platform>".
	"uem_profile": regexp.MustCompile(`^\d+:[A-Za-z0-9_ ]+$`),
	// internal/application/assignment/resource_crud.go ImportState: a bare
	// (non-empty, trimmed) application UUID.
	"uem_application_assignment": regexp.MustCompile(`^[0-9a-fA-F-]{36}$`),
	// internal/application/purchased-app/assignment/resource_crud.go
	// ImportState: a bare (normalized, trimmed/lowercased) application UUID.
	"uem_purchased_application_assignment": regexp.MustCompile(`^[0-9a-fA-F-]{36}$`),
	// internal/application/internal-app/mac/resource_crud.go ImportState:
	// splits on "," and accepts either a bare UUID (validated against a
	// strict 8-4-4-4-12 hex pattern) or "<uuid>,<org_group_id>".
	"uem_mac_application": regexp.MustCompile(`^[0-9a-fA-F-]{36}(,\d+)?$`),
	// internal/scripts/mac/crud.go ImportState: a bare (non-empty, trimmed)
	// script UUID.
	"uem_mac_script": regexp.MustCompile(`^[0-9a-fA-F-]{36}$`),
	// internal/scripts/assignment/resource_crud.go ImportState: a bare
	// (non-empty, trimmed) script UUID.
	"uem_script_assignment": regexp.MustCompile(`^[0-9a-fA-F-]{36}$`),
	// internal/sensors/mac/crud.go ImportState: a bare (non-empty, trimmed)
	// device sensor UUID.
	"uem_mac_sensor": regexp.MustCompile(`^[0-9a-fA-F-]{36}$`),
	// internal/sensors/assignment/resource_crud.go ImportState: a bare
	// (non-empty, trimmed) device sensor UUID.
	"uem_sensor_assignment": regexp.MustCompile(`^[0-9a-fA-F-]{36}$`),
	// internal/updates/deployment/crud.go ImportState: a bare (non-empty,
	// trimmed) deployment UUID.
	"uem_update_deployment": regexp.MustCompile(`^[0-9a-fA-F-]{36}$`),
	// internal/smartgroup/resource_crud.go ImportState: a bare numeric smart
	// group id, or a smart group UUID (strict 8-4-4-4-12 hex, resolved
	// case-insensitively through the shared search walk).
	"uem_smart_group": regexp.MustCompile(`^(\d+|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`),
}

// exampleUsagePattern matches a doc page's "## Example Usage" heading
// followed by a single fenced code block (the fence language varies -- most
// pages use ```terraform, a couple of hand-written pages use ```hcl -- so the
// language token is captured loosely as \w*). Applied to a page already
// normalized to LF (see exampleUsageCodeBlock).
var exampleUsagePattern = regexp.MustCompile("(?s)\n## Example Usage\n\n```\\w*\n(.*?)\n```\n")

// exampleUsageCodeBlock returns the content of a doc page's "## Example
// Usage" fenced code block, or false if the page has no such heading/block.
// CRLF is normalized to LF first, matching missingSchemaAttributes and
// importSection, so this works for both LF and CRLF pages.
func exampleUsageCodeBlock(page string) (string, bool) {
	page = strings.ReplaceAll(page, "\r\n", "\n")
	m := exampleUsagePattern.FindStringSubmatch(page)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// normalizeExampleContent normalizes CRLF to LF and trims a single trailing
// newline, so a doc's fenced Example Usage block and its corresponding
// examples/.../*.tf file can be compared byte-for-byte regardless of the
// doc page's line endings or a trailing newline at end of file.
func normalizeExampleContent(s string) string {
	return strings.TrimRight(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

// importSection returns the body of a doc page's "## Import" section (from
// just after the heading to the next "## " heading or EOF), or "" if there is
// no such heading. CRLF is normalized to LF first, matching
// missingSchemaAttributes, so this works for both LF and CRLF pages.
func importSection(page string) (string, bool) {
	page = strings.ReplaceAll(page, "\r\n", "\n")
	i := strings.Index(page, "\n## Import\n")
	if i < 0 {
		return "", false
	}
	section := page[i+len("\n## Import\n"):]
	if j := strings.Index(section, "\n## "); j >= 0 {
		section = section[:j]
	}
	return section, true
}

// importIDsInSection returns every id named on a "terraform import
// <type>.<name> <id>" line (quoted or unquoted) inside section. Lines that
// are shell comments (after trimming leading whitespace they start with "#",
// which covers the "# Usage: terraform import <type>.example "<format>""
// placeholder-format comment every import.sh carries) are skipped, so only
// the real, runnable example line(s) are checked against the shape regexp.
func importIDsInSection(section, typeName string) []string {
	pattern := regexp.MustCompile(`terraform import ` + regexp.QuoteMeta(typeName) + `\.[A-Za-z0-9_-]+\s+"?([^"\s]+)"?`)
	var ids []string
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if m := pattern.FindStringSubmatch(line); m != nil {
			ids = append(ids, m[1])
		}
	}
	return ids
}

// TestResourceDocsExistAndCoverSchema is a guard test for internal-task: every
// resource registered in the provider's Resources() list must have a
// docs/resources/<name>.md page and an examples/resources/<TypeName>/
// resource.tf example that references it, and that doc page must mention
// every attribute the resource's real Schema() produces (recursively,
// including attributes nested inside Attributes List/Set/Map/Single objects
// and inside Blocks) as a Markdown code span (backtick-quoted, e.g. `name`)
// after the page's "## Schema" heading -- the same convention
// TestDataSourceDocsExistAndCoverSchema enforces for data sources.
//
// This is deliberately driven off the REAL, REGISTERED provider
// (New("test")().Resources(ctx)) and the REAL Schema() each resource
// returns, not a hand-maintained list of resource names/attributes, so it
// fails the moment a resource's schema changes without its doc page being
// updated to match, or a doc page/example is deleted.
func TestResourceDocsExistAndCoverSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	repoRoot := repoRootFromThisFile(t)

	p := New("test")()
	ctors := p.Resources(ctx)
	if len(ctors) == 0 {
		t.Fatal("provider.Resources() returned zero resources; nothing to check")
	}

	for _, ctor := range ctors {
		res := ctor()

		var metaResp resource.MetadataResponse
		res.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "uem"}, &metaResp)
		typeName := metaResp.TypeName
		if typeName == "" {
			t.Fatalf("a resource constructor returned an empty TypeName from Metadata()")
		}

		t.Run(typeName, func(t *testing.T) {
			t.Parallel()

			var schemaResp resource.SchemaResponse
			res.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
			if schemaResp.Diagnostics.HasError() {
				t.Fatalf("resource %s: Schema() produced diagnostics errors: %s", typeName, schemaResp.Diagnostics)
			}

			// --- docs/resources/<name>.md must exist and mention every
			// attribute name (recursively) as a `name` code span after
			// "## Schema". ---
			docName := strings.TrimPrefix(typeName, "uem_")
			docPath := filepath.Join(repoRoot, "docs", "resources", docName+".md")
			docBytes, err := os.ReadFile(docPath)
			if err != nil {
				t.Fatalf("resource %s: expected docs page %s to exist, but it does not (or could not be read): %s",
					typeName, docPath, err)
			}
			docContent := string(docBytes)

			attrNames := map[string]bool{}
			collectResourceAttributeNames(schemaResp.Schema.Attributes, schemaResp.Schema.Blocks, attrNames)
			if len(attrNames) == 0 {
				t.Fatalf("resource %s: Schema() returned zero attributes", typeName)
			}

			for _, name := range missingSchemaAttributes(docContent, attrNames) {
				t.Errorf("resource %s: docs page %s is missing attribute %q (expected a \"- `%s`\" item after the ## Schema heading)",
					typeName, docPath, name, name)
			}

			// --- a resource implementing resource.ResourceWithImportState
			// must document import: a "## Import" section whose example
			// "terraform import <type>.<name> <id>" line(s) match this
			// resource's entry in importIDShapes (the cheap ImportState
			// shape check). A resource with no importIDShapes entry fails,
			// so a new importable resource cannot ship undocumented. ---
			if _, ok := res.(resource.ResourceWithImportState); ok {
				section, hasImport := importSection(docContent)
				if !hasImport {
					t.Errorf("resource %s: implements resource.ResourceWithImportState but docs page %s has no \"## Import\" section",
						typeName, docPath)
				} else {
					shape, hasShape := importIDShapes[typeName]
					if !hasShape {
						t.Errorf("resource %s: implements resource.ResourceWithImportState but has no entry in importIDShapes; add one describing what ImportState accepts",
							typeName)
					} else {
						ids := importIDsInSection(section, typeName)
						if len(ids) == 0 {
							t.Errorf("resource %s: docs page %s \"## Import\" section has no \"terraform import %s.<name> \\\"<id>\\\"\" example line",
								typeName, docPath, typeName)
						}
						for _, id := range ids {
							if !shape.MatchString(id) {
								t.Errorf("resource %s: docs page %s Import section example id %q does not match the expected shape %s (see ImportState)",
									typeName, docPath, id, shape.String())
							}
						}
					}
				}
			}

			// --- examples/resources/<TypeName>/resource.tf must exist and
			// reference this resource by its real TypeName. ---
			examplePath := filepath.Join(repoRoot, "examples", "resources", typeName, "resource.tf")
			exampleBytes, err := os.ReadFile(examplePath)
			if err != nil {
				t.Fatalf("resource %s: expected example file %s to exist, but it does not (or could not be read): %s",
					typeName, examplePath, err)
			}
			exampleContent := string(exampleBytes)

			resourceRef := fmt.Sprintf("resource %q", typeName)
			if !strings.Contains(exampleContent, resourceRef) {
				t.Errorf("resource %s: example file %s does not contain a %s block",
					typeName, examplePath, resourceRef)
			}

			// --- the doc page's "## Example Usage" fenced code block must
			// byte-match examples/resources/<TypeName>/resource.tf (after
			// normalizing CRLF to LF and trimming a trailing newline on both
			// sides), so a doc snippet and its example file cannot silently
			// drift apart -- e.g. a `terraform fmt` pass on the example file
			// that isn't mirrored into the doc. ---
			block, hasBlock := exampleUsageCodeBlock(docContent)
			if !hasBlock {
				t.Errorf("resource %s: docs page %s has no \"## Example Usage\" fenced code block",
					typeName, docPath)
			} else if got, want := normalizeExampleContent(block), normalizeExampleContent(exampleContent); got != want {
				t.Errorf("resource %s: docs page %s \"## Example Usage\" block does not match example file %s (compared after normalizing CRLF to LF and trimming a trailing newline)\n--- doc block ---\n%s\n--- example file ---\n%s",
					typeName, docPath, examplePath, got, want)
			}
		})
	}
}

func TestMissingSchemaAttributes(t *testing.T) {
	t.Parallel()
	names := map[string]bool{"assignment_count": true, "uuid": true}
	cases := []struct {
		name string
		page string
		want []string
	}{
		{"both listed", "intro\n\n## Schema\n\n- `assignment_count` (Number) x\n- `uuid` (String) y\n", nil},
		{"prose-only mention does not count", "Note: `assignment_count` costs a GET.\n\n## Schema\n\n- `uuid` (String) y\n", []string{"assignment_count"}},
		{"no schema heading", "- `assignment_count` (Number) x\n- `uuid` (String) y\n", []string{"assignment_count", "uuid"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := missingSchemaAttributes(tc.page, names)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("missingSchemaAttributes = %v, want %v", got, tc.want)
			}
		})
	}
}
