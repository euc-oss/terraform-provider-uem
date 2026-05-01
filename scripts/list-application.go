package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/euc-oss/terraform-sdk-uem/client"
)

func main() {
	// platform filter (Android, Apple iOS, AppleOsX, Windows 10, etc.)
	platformFilter := flag.String("platform", "AppleOsX", "Filter by platform (Android, Apple iOS, AppleOsX, Windows 10, etc.) omit for no platform filter")

	// app type filter (INTERNAL, PUBLIC, etc.)
	appTypeFilter := flag.String("app-type", "INTERNAL", "Filter by application type (INTERNAL, PUBLIC, etc.) omit for no platform filter")

	// application search page size
	pageSize := flag.Int("page-size", 100, "Number of results to fetch per page (max 500)")
	flag.Parse()

	// Get configuration from environment variables
	instanceURL := os.Getenv("UEM_INSTANCE_URL")
	tenantCode := os.Getenv("UEM_TENANT_CODE")
	authMethod := os.Getenv("UEM_AUTH_METHOD")
	username := os.Getenv("UEM_USERNAME")
	password := os.Getenv("UEM_PASSWORD")
	clientID := os.Getenv("UEM_CLIENT_ID")
	clientSecret := os.Getenv("UEM_CLIENT_SECRET")
	oauth2TokenURL := os.Getenv("UEM_OAUTH2_TOKEN_URL")

	// Validate required configuration
	if instanceURL == "" || tenantCode == "" {
		fmt.Fprintln(os.Stderr, "Error: UEM_INSTANCE_URL and UEM_TENANT_CODE must be set")
		os.Exit(1)
	}

	// Default to basic auth if not specified
	if authMethod == "" {
		authMethod = "basic"
	}

	// Create client configuration
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

	// Create Workspace ONE client
	wsoneClient, err := client.NewClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating client: %v\n", err)
		os.Exit(1)
	}

	// Create apps service
	appsService := sdk.NewAppsV2Service(wsoneClient)

	ctx := context.Background()

	// Collect all applications
	type ApplicationInfo struct {
		ID                    int
		UUID                  string
		Name                  string
		Platform              string
		Description           string
		Version               string
		ApplicationType       string
		BundleID              string
		OrganizationGroupUUID string
	}

	var allApplications []ApplicationInfo

	applicationSearchOption := sdk.AppsV2SearchOptions{
		PageSize: pageSize,
	}
	page := 0
	applicationSearchOption.Page = &page

	// If platform filter specified, search only that platform
	if *platformFilter != "" {
		applicationSearchOption.Platform = platformFilter
	}

	if *appTypeFilter != "" {
		applicationSearchOption.ApplicationType = appTypeFilter
	}

	for {
		_, result, err := appsService.Search(ctx, &applicationSearchOption)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Failed to search %s applications: %v\n", *platformFilter, err)
			os.Exit(1)
		}

		if len(result.Applications) == 0 {
			break
		}

		for _, apps := range result.Applications {
			appID := 0
			if apps.ID != nil {
				appID = *apps.ID
			}

			applicationInfo := ApplicationInfo{
				ID:                    appID,
				UUID:                  apps.UUID,
				Name:                  apps.ApplicationName,
				Platform:              apps.Platform,
				Version:               apps.VersionIdentifier,
				ApplicationType:       apps.ApplicationType,
				BundleID:              apps.BundleID,
				OrganizationGroupUUID: apps.OrganizationGroupUUID,
				Description:           apps.Description,
			}
			allApplications = append(allApplications, applicationInfo)
		}

		page++
	}

	// Sort applications by platform, then by name
	sort.Slice(allApplications, func(i, j int) bool {
		if allApplications[i].Platform != allApplications[j].Platform {
			return allApplications[i].Platform < allApplications[j].Platform
		}
		return allApplications[i].Name < allApplications[j].Name
	})

	// Display results
	if len(allApplications) == 0 {
		fmt.Println("No applications found")
		return
	}

	fmt.Printf("Found %d application(s)\n\n", len(allApplications))

	// Create tabwriter for aligned output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tUUID\tNAME\tPLATFORM\tVERSION\tAPP_TYPE\tBUNDLE_ID\tORG_GROUP_UUID\tDESCRIPTION")
	fmt.Fprintln(w, "----\t----\t----\t--------\t-------\t--------\t---------\t--------------\t-----------")

	for _, application := range allApplications {

		// Truncate description if too long
		desc := application.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}

		// Replace newlines in description
		desc = strings.ReplaceAll(desc, "\n", " ")
		desc = strings.ReplaceAll(desc, "\r", " ")

		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			application.ID,
			application.UUID,
			application.Name,
			application.Platform,
			application.Version,
			application.ApplicationType,
			application.BundleID,
			application.OrganizationGroupUUID,
			desc)
	}

	w.Flush()
}
