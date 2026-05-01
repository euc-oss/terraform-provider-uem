package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/euc-oss/terraform-sdk-uem/client"
)

const acceptHeader = "application/json;version=1"

// pickEntry matches the picklists API response shape:
// [{"Description": "...", "Key": "..."}, ...]
type pickEntry struct {
	Description string `json:"Description"`
	Key         string `json:"Key"`
}

func main() {
	instanceURL := os.Getenv("UEM_INSTANCE_URL")
	tenantCode := os.Getenv("UEM_TENANT_CODE")
	orgGroupID := os.Getenv("UEM_ORG_GROUP_ID")
	authMethod := os.Getenv("UEM_AUTH_METHOD")
	username := os.Getenv("UEM_USERNAME")
	password := os.Getenv("UEM_PASSWORD")
	clientID := os.Getenv("UEM_CLIENT_ID")
	clientSecret := os.Getenv("UEM_CLIENT_SECRET")
	oauth2TokenURL := os.Getenv("UEM_OAUTH2_TOKEN_URL")

	if instanceURL == "" || tenantCode == "" {
		fmt.Fprintln(os.Stderr, "Error: UEM_INSTANCE_URL and UEM_TENANT_CODE must be set")
		os.Exit(1)
	}
	if orgGroupID == "" {
		fmt.Fprintln(os.Stderr, "Error: UEM_ORG_GROUP_ID must be set (the picklists API is org-group scoped)")
		os.Exit(1)
	}

	if authMethod == "" {
		authMethod = "basic"
	}

	cfg := &client.Config{
		InstanceURL:    instanceURL,
		TenantCode:     tenantCode,
		AuthMethod:     authMethod,
		Username:       username,
		Password:       password,
		ClientID:       clientID,
		ClientSecret:   clientSecret,
		OAuth2TokenURL: oauth2TokenURL,
	}

	uemClient, err := client.NewClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating client: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// 1. List CAs for this OG.
	caEndpoint := fmt.Sprintf("/api/mdm/picklists/organizationgroups/%s/certificateauthorities", orgGroupID)
	cas, err := fetchPickList(ctx, uemClient, caEndpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Failed to list certificate authorities for OG %s: %v\n", orgGroupID, err)
		os.Exit(1)
	}
	if len(cas) == 0 {
		fmt.Println("No certificate authorities found for this organization group")
		return
	}
	sort.Slice(cas, func(i, j int) bool { return cas[i].Description < cas[j].Description })

	// 2. Build flat rows: one per (CA, template). CAs with no templates
	// emit a single row with empty template cells.
	type row struct {
		caName, caID, tmplName, tmplID string
	}
	var rows []row
	for _, ca := range cas {
		tmplEndpoint := fmt.Sprintf("/api/mdm/picklists/organizationgroups/%s/certificateauthorities/%s/certificatetemplates", orgGroupID, ca.Key)
		tmpls, terr := fetchPickList(ctx, uemClient, tmplEndpoint)
		if terr != nil {
			rows = append(rows, row{
				caName:   ca.Description,
				caID:     ca.Key,
				tmplName: fmt.Sprintf("(template fetch failed: %v)", terr),
			})
			continue
		}
		if len(tmpls) == 0 {
			rows = append(rows, row{caName: ca.Description, caID: ca.Key})
			continue
		}
		sort.Slice(tmpls, func(i, j int) bool { return tmpls[i].Description < tmpls[j].Description })
		for _, t := range tmpls {
			rows = append(rows, row{
				caName: ca.Description, caID: ca.Key,
				tmplName: t.Description, tmplID: t.Key,
			})
		}
	}

	fmt.Printf("Found %d certificate authorit%s in OG %s\n\n",
		len(cas), pluralY(len(cas)), orgGroupID)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CA NAME\tAUTHORITY ID\tTEMPLATE NAME\tTEMPLATE ID")
	fmt.Fprintln(w, "-------\t------------\t-------------\t-----------")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.caName, r.caID, r.tmplName, r.tmplID)
	}
	w.Flush()
}

// fetchPickList GETs an endpoint that is documented to return []pickEntry.
// Falls back to decoding [][]pickEntry and flattening if the live response
// happens to be wrapped (some swagger samples show a nested array).
func fetchPickList(ctx context.Context, c *client.Client, endpoint string) ([]pickEntry, error) {
	// Get raw bytes via DoRequest using a *json.RawMessage receiver, then
	// try the two shapes locally — keeps us tolerant of either response
	// without an extra HTTP round-trip.
	var raw json.RawMessage
	if _, err := c.DoRequest(ctx, "GET", endpoint, acceptHeader, "", nil, &raw); err != nil {
		return nil, err
	}
	var flat []pickEntry
	if err := json.Unmarshal(raw, &flat); err == nil {
		return flat, nil
	}
	var nested [][]pickEntry
	if err := json.Unmarshal(raw, &nested); err == nil {
		var out []pickEntry
		for _, group := range nested {
			out = append(out, group...)
		}
		return out, nil
	}
	return nil, fmt.Errorf("unrecognized response shape: %s", string(raw))
}

func pluralY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
