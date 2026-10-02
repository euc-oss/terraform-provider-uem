# Your onboarding journey with `ws1-tf`

`ws1-tf` is a command-line tool that brings your existing Workspace ONE UEM environment into
Terraform. This page walks you through that journey step by step and shows the terminal output you
see at each step. It applies to UEM 26.2 and provider v26.2.0-beta.1.

For the hands-on reference covering install instructions, every flag, and full transcripts, see
[`getting-started.md`](./getting-started.md). Each step below points to the matching section there.

The examples use fictional org groups, ids and version numbers. Names differ between steps.

## Why use `ws1-tf`

Your UEM tenant holds years of configuration created in the console. None of it is in Terraform yet.
Writing that Terraform by hand is slow, and a mistake can recreate or destroy objects that serve
live devices.

`ws1-tf` reads what already exists and generates the Terraform for it. You confirm each step. It
does not guess, and it does not touch anything it is not sure about.

---

## Step 1: Get started

Open a terminal in an empty folder you have set aside for this, and run the tool. You do not need
to know any commands yet.

```text
$ ws1-tf
You're in /Users/alex/uem-terraform. Initialize this path as your repo? [y/N]: y
```

Answer "yes" and the setup wizard starts (Step 2).

---

## Step 2: Run the setup wizard

The wizard asks for your console URL and credentials. It checks them against your real tenant
before it writes anything.

```text
Tenant name (e.g. dev, test, prod): prod
Console URL (instance_url): https://as1234.awmdm.com
API tenant code (tenant_code / aw-tenant-code): **********
Username (basic auth): svc-terraform
Password (basic auth): **********
OAuth client_id:
OAuth client_secret:
OAuth token URL (oauth2_token_url):

UEM console version: 26.2.1234.5
Proceed against UEM version 26.2.1234.5? [y/N]: y

  1. Global (Container, id 100)
  2. Sales Region (Container, id 214)
  3. West Coast Devices (Customer, id 341)
Select the org group to target (number): 1

Initialized /Users/alex/uem-terraform (tenant "prod"). Credentials written to /Users/alex/uem-terraform/prod/.env
```

You now have a scaffolded Terraform repo and saved tenant credentials. The credentials are private
and never checked into version control.

The check is a real call to your console. If it fails, nothing is written, and you can try again.

You don't pick basic authentication or OAuth2 up front. Enter either or both. The wizard keeps the
ones that work, and prefers OAuth2 if both do. A rejected password gets a retry. An unreachable
console stops the wizard, because retyping won't fix it.

---

## Step 3: Check connectivity

To confirm the tenant is reachable, for example after a password rotation or before a teammate
picks up the repo, you can run a check without repeating the wizard.

```text
$ ws1-tf verify --path /Users/alex/uem-terraform/prod
basic: true  oauth: false  version: 26.2.1234.5
```

`verify` checks the basic and OAuth credentials independently and reports both, so you can see
which ones work even if you configured only one. Its exit status follows the auth method the tenant
is configured to use (or, if none is set, the one the provider would infer): if that method fails,
`verify` exits non-zero even when the other one works, so a script can rely on it.

---

## Step 4: Come back later to the menu

When you (or a teammate) open the same folder again later, `ws1-tf` recognizes that it is already
set up. It does not ask the wizard questions again and makes no UEM API call at this point; it
shows a menu of what you can do next. The transcript below is the single-shot form of the menu, as
seen with piped or scripted input. On a real terminal the menu loops and labels the environment;
see `getting-started.md` Section 5.3.

```text
$ ws1-tf
This directory is already initialized.
Available actions:
  1. Re-run init / update credentials  (ws1-tf init --path <dir>)
  2. Show version  (ws1-tf version)
  3. Verify UEM connectivity  (ws1-tf verify --path <dir>)
  4. List onboardable types  (ws1-tf list)
  5. Onboard existing resources  (ws1-tf onboard --type <t>)
Select an action by number, or press Enter to exit: 4
TYPE                              STATUS  DETAILS
profile                           ready   Device profiles and their payload settings.
...
```

Type a number to run that action. Here, "4" runs `list`. Press Enter on a blank line to exit.

If you just cloned the repo, you have no credentials yet, because they are never committed. The tool
offers to create them.

---

## Step 5: See what you can onboard

Before you bring anything into Terraform, `ws1-tf list` shows you, type by type, what you can
onboard today.

```text
$ ws1-tf list
TYPE                              STATUS  DETAILS
profile                           ready   Device profiles and their payload settings.
mac_application                   ready   Internal macOS apps, downloaded with their installer,
                                          pkginfo and icon.
application_assignment            ready   Assignment rules of internal macOS apps; an app not yet
                                          onboarded comes with them.
mac_script                        ready   macOS scripts; each script body is saved as a file next to
                                          the config.
script_assignment                 ready   Assignment rules of macOS scripts; a script not yet
                                          onboarded comes with them.
mac_sensor                        ready   macOS sensors; each sensor's code is saved as a file next
                                          to the config.
sensor_assignment                 ready   Assignment rules of macOS sensors; a sensor not yet
                                          onboarded comes with them.
update_deployment                 ready   macOS update deployments.
purchased_application_assignment  ready   Assignment rules of purchased (VPP) apps; apps without
                                          rules are skipped.
smart_group                       ready   Smart groups and their criteria.

Details: ws1-tf list --type <type>

$ ws1-tf list --type mac_application
Type        mac_application
Name        macOS Applications
Status      ready
Onboards    Internal macOS apps, downloaded with their installer, pkginfo and icon.
Lists from  the uem_mac_applications data source
Import ID   <uuid>,<org_group_id>
Note        Only apps the selected org group owns, even with --include-inherited: UEM lets only the
            owning org group download an app's files. org_group_id is that org group's numeric id.

$ ws1-tf list --type widget
error: unknown type "widget" — valid types: application_assignment, mac_application, mac_script, mac_sensor, profile, purchased_application_assignment, script_assignment, sensor_assignment, smart_group, update_deployment
```

All 10 onboardable types are `ready`: `profile`, `mac_application`, `application_assignment`,
`mac_script`, `script_assignment`, `mac_sensor`, `sensor_assignment`, `update_deployment`,
`purchased_application_assignment`, and `smart_group`. `list` prints a TYPE / STATUS / DETAILS
table, and `list --type <type>` shows one type's details: what it onboards, the data source it lists
from, its Terraform import ID, and any caveat. See `getting-started.md` Section 5.4 for the full
`ws1-tf list` output.

---

## Step 6: Onboard a resource

This step turns something that already exists in your console into Terraform code and state.

```text
$ ws1-tf onboard --type profile
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number): 1
2 profiles skipped: owned by child org group "Sales" (id 12350). Add --include-inherited to onboard them.
  1. Custom Profile (AppleOsX, profile_id 12347)
  2. Restrictions Profile (AppleOsX, profile_id 12348)
  3. windows _device_profile (Windows 10, profile_id 12349)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5]: all
Onboarded 3 profiles into /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/profiles.tf. Review required before commit/apply; no apply was run.
```

You pick a type, see what exists, and choose items by number. There is no "Found N" line; the list
is the answer.

You now have Terraform HCL generated from a real `terraform import`. A real `terraform plan` then
confirms it as a no-op. That clean plan is the tool's core safety check. See `getting-started.md`
Section 5.5 for the full output.

### How references are handled

There is no `warning:` line above. `onboard` rewrites every org-group, smart-group, and
already-onboarded assignment-parent reference it can resolve, such as `org_group_id`. Each becomes
an expression that points at a generated lookup. The lookup is a `data "uem_organization_groups"` or
`data "uem_smart_groups"` block in a shared `dependencies.tf`, reused across runs. In the common
case, nothing extra is printed.

- A reference outside your credential's direct access (visible but inherited from an ancestor org
  group, not readable directly) is not a warning either. It resolves to a documented, fixed
  `locals` entry in the same `dependencies.tf` instead of a live lookup, and still plans clean.
- The same applies to a smart group referenced only by a UUID that UEM's smart group search does
  not return (typically an org group's own smart group); its `locals` comment says so rather than
  reporting an access problem.
- A smart-group reference owned by the org group you are onboarding is imported as its own managed
  `uem_smart_group` resource (deduplicated against one already in state, whether from this run, an
  earlier run, or from onboarding it directly with `--type smart_group`), and the reference points at
  that resource.
- A warning appears only in the one case `onboard` cannot resolve on its own: an assignment whose
  parent was skipped by owned-only filtering.

See the "Dependencies" section of `getting-started.md` (just above the per-type walkthrough in
Section 5.5) for a plain-language explanation of lookups, resources, and `locals`, and its "Two
warnings you will see in real use" section for the generated HCL and the one remaining warning.

All 10 onboardable types (Step 5) onboard end to end. See
[`getting-started.md` Section 5.5](./getting-started.md#55-onboard-journey-step-6) for the full
transcript of all 10 types and the generated `profiles.tf`.

The generated configuration and Terraform state live in the hidden `.ws1tf-onboard-work/` folder
inside the tenant directory (as in the path printed above).

### Owned-only by default

By default, `onboard` imports only items the selected org group owns. An item that is only
inherited from a parent is skipped. The skip is never silent. One line per owning org group gives
the count and names the owner as precisely as UEM allows (`parent` or `child` plus the org group's name and id, `parent org group id <n>` when only
the id is known, or the bare uuid when the owner is not in the selected org group's own tree).
Importing such an item here and again under the parent's own Terraform configuration would leave
two states managing the same object.

- `--include-inherited` opts in to importing inherited items.
- `--org-group <id|uuid>` replaces only the numbered org-group prompt with a non-interactive value;
  the version confirmation and item selection prompts still run.

Both flags are demonstrated in the "Owned-only by default" section of `getting-started.md`,
including a case with two different other owners skipped in the same run.

---

## Step 7: Use the same commands in scripts and automation

Each menu entry runs the same action as its command-line form. A script or an AI agent gets the
same result you do.

```text
# A human, driving the interactive menu:
$ ws1-tf
This directory is already initialized.
Available actions:
  4. List onboardable types  (ws1-tf list)

# The exact same action, driven by flag — no menu, no prompts:
$ ws1-tf list
TYPE                              STATUS  DETAILS
profile                           ready   Device profiles and their payload settings.
...
```

No step requires a person at a keyboard. A script, or an agent such as Claude Code, can run the
whole flow non-interactively with the commands the menu shows.

---

## What else you get today

- The `init` setup wizard, which verifies credentials before writing anything (both credential sets
  checked, OAuth2 preferred when both verify).
- `verify`, for on-demand connectivity checks.
- The menu for an already-initialized folder, with numbered actions you can run directly.
- `list`, a type-by-type view of what you can bring into Terraform.
- `onboard`, which turns live UEM resources into Terraform code and state and finishes with a clean
  `terraform plan` after import. All 10 types work: `profile`, `mac_application`,
  `application_assignment`, `mac_script`, `script_assignment`, `mac_sensor`, `sensor_assignment`,
  `update_deployment`, `purchased_application_assignment` (which skips VPP apps without assignment
  rules), and `smart_group`. It includes owned-only import filtering by default with the
  `--include-inherited` and `--org-group` options, and macOS-only filtering for the sensor and
  script types.
- Pickers that page and search on a real terminal (with `all`, numbers, and ranges such as `3,7-9`),
  plus `--name` to filter the list before the picker.
- Clean skips instead of failures: a mac application owned by a parent org group, and a profile UEM
  cannot serialize over its API (`400 Invalid Payload Key`), are each skipped with a note and a
  count. So are a profile holding a payload `uem_profile` does not model yet (Associated Domains,
  Exchange ActiveSync native mail client, Mail, Setup Assistant, Smart Card, and SSO Extensions), an
  object UEM answers with HTTP 500, and a profile whose payload check could not read it (because it
  cannot be shown to be safe). Any other failure, including HTTP 502, 503, and 504, stops the run and
  rolls the imports back.
- Write-only profile secrets stay out of the generated files. The passwords and certificates UEM
  never returns are wired to a `secrets.json`/`secrets.tf` pair instead. Only `secrets.json` holds
  values: it lives under `uem-artifacts/<tenant>/secrets/`, has mode 0600, and is covered by the
  `/uem-artifacts/` rule in the repo-root `.gitignore`, which `init` writes and `onboard` adds if it
  is missing; if you manage that file yourself, make sure it contains `/uem-artifacts/`.
  `secrets.tf` has mode 0644 in `.ws1tf-onboard-work`, is committed like the other generated `.tf`
  files, and holds no secret values. A blank key renders as `null` and is never sent (no plan-time
  error), and the end-of-run summary lists the keys that are still blank so you can fill them before
  applying to a new environment. Only attributes with those exact names are treated as secrets.
  Everything else UEM returns, including script and sensor bodies, is written as UEM returned it, so
  review the generated files before you commit them.
- `refresh-local`, which detects drift between UEM and the generated `.tf` files and fixes it in
  place. Only an attribute written as a plain literal is ever rewritten. A change to an attribute
  whose current `.tf` value is an expression (for example, a lookup reference) is shown but never
  applied silently. A resource that also has a change `refresh-local` will not apply is left alone
  entirely rather than partly updated, and every file it rewrites keeps a `.ws1tf-bak` copy of the
  original.
- A per-run action log (JSON Lines) of every `terraform` command, UEM API call, and file written.
- The provider acquire and install flow (see `getting-started.md` Section 4), the self-installing
  CLI release bundle, and full org-group list pagination, so a large tenant's org-group list is never
  silently truncated.

---

## Coming later

The following are planned and **not available yet**:

- **Generated documentation.** A per-tenant document set (a README, an onboarding runbook, a GitOps
  guide, and notes per resource type), generated from the data `ws1-tf` already collects (console
  URL, UEM version, org-group scope, and what has been onboarded). A `docs --check` command would
  report when the docs no longer match the onboarded state, for example after someone changes the
  environment without re-running `ws1-tf`. Illustrative output:

  ```text
  $ ws1-tf docs --check
  Checking generated docs against current onboarded state...
    README.md            up to date
    RUNBOOK.md           up to date
    GITOPS-GUIDE.md       up to date
    profiles.md          up to date
  All docs current. ✅
  ```

- **Package manager distribution**, in addition to the release ZIP.
- **Claude Code skills** for `ws1-tf`.
