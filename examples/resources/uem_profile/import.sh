#!/bin/bash
# Import an existing Workspace ONE profile into Terraform state

# Import by profile ID and platform
# Usage: terraform import uem_profile.example "<profile_id>:<platform>"

terraform import uem_profile.android_basic "12345:Android"

