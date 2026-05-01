package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/euc-oss/terraform-sdk-uem/client"
	"github.com/euc-oss/terraform-sdk-uem/resources"
)

func main() {
	// Command line flags
	platformFilter := flag.String("platform", "", "Filter by platform (Android, Apple iOS, AppleOsX, Windows 10, etc.)")
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

	// Create profile service
	profileService := resources.NewProfileService(wsoneClient)

	ctx := context.Background()

	// Collect all profiles
	type ProfileInfo struct {
		ID          int
		Name        string
		Platform    string
		Description string
		IsActive    bool
	}

	var allProfiles []ProfileInfo

	// If platform filter specified, search only that platform
	if *platformFilter != "" {
		result, err := profileService.Search(ctx, &resources.SearchOptions{
			Platform: *platformFilter,
			PageSize: 500,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Failed to search %s profiles: %v\n", *platformFilter, err)
			os.Exit(1)
		}

		for _, profile := range result.Profiles {
			profileInfo := ProfileInfo{
				ID:       profile.GetProfileID(),
				Name:     profile.GetName(),
				Platform: *platformFilter,
			}
			allProfiles = append(allProfiles, profileInfo)
		}
	} else {
		// Search ALL profiles without platform filter
		result, err := profileService.Search(ctx, &resources.SearchOptions{
			PageSize: 500, // Get up to 500 profiles
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Failed to search profiles: %v\n", err)
			os.Exit(1)
		}

		for _, profile := range result.Profiles {
			// Use the new GetPlatform() method added in SDK bug fix
			platform := profile.GetPlatform()
			if platform == "" {
				platform = "Unknown"
			}

			profileInfo := ProfileInfo{
				ID:       profile.GetProfileID(),
				Name:     profile.GetName(),
				Platform: platform,
			}
			allProfiles = append(allProfiles, profileInfo)
		}
	}

	// Sort profiles by platform, then by name
	sort.Slice(allProfiles, func(i, j int) bool {
		if allProfiles[i].Platform != allProfiles[j].Platform {
			return allProfiles[i].Platform < allProfiles[j].Platform
		}
		return allProfiles[i].Name < allProfiles[j].Name
	})

	// Display results
	if len(allProfiles) == 0 {
		fmt.Println("No profiles found")
		return
	}

	fmt.Printf("Found %d profile(s)\n\n", len(allProfiles))

	// Create tabwriter for aligned output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tPLATFORM\tACTIVE\tDESCRIPTION")
	fmt.Fprintln(w, "----\t----\t--------\t------\t-----------")

	for _, profile := range allProfiles {
		active := "Yes"
		if !profile.IsActive {
			active = "No"
		}

		// Truncate description if too long
		desc := profile.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}

		// Replace newlines in description
		desc = strings.ReplaceAll(desc, "\n", " ")
		desc = strings.ReplaceAll(desc, "\r", " ")

		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
			profile.ID,
			profile.Name,
			profile.Platform,
			active,
			desc)
	}

	w.Flush()
}
