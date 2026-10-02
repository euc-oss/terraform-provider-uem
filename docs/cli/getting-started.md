# `ws1-tf` — Getting Started

**What this is:** A hands-on guide to installing `ws1-tf` and using it to bring an existing
Workspace ONE UEM environment into Terraform — every command below, and what it prints.

**Who it's for:** Anyone installing or running `ws1-tf` for the first time.

**Version covered:** This guide targets **UEM 26.2** and **provider v26.2.0-beta.1** of the
Omnissa Workspace ONE UEM Terraform provider (`omnissa/uem`). The transcripts print the provider
version as `26.2.0`.

**How to use this document:**

- Companion to [`onboarding-journey.md`](./onboarding-journey.md), which tells the same
  journey step by step. This document is the hands-on how-to; use the Table of
  Contents below to jump to the section you need.
- New to `ws1-tf`? Start at [Section 3](#3-install-the-cli) to install it, then
  [Section 5](#5-usage) to use it.
- Curious how `ws1-tf` picks and installs its Terraform provider? That's
  [Section 4](#4-how-ws1-tf-finds-and-installs-the-omnissauem-provider) — background you can skip
  on a first read.

**A few things worth knowing before you dive in:**

- Provider behavior described here (for example `uem_mac_application` fields such as `icon_file_path`, the
  `org_group_id` migration, and `dmg_file_path`/`plist_file_path` replacement) is that of the 26.2.0 provider
  release.
- The transcripts in Sections 3 and 5 are **captured** from actual runs of the `ws1-tf` binary
  and its shipped `install.sh` (with the placeholder substitutions below), not hand-written. A few
  blocks are composites or illustrations rather than one verbatim capture, and each says so where it
  appears: the command forms and sample output in Sections 5.7 and 5.9, the record table and replay
  recipes in Section 5.8, and the folder listings, which are trimmed to the files each section is about.
  Section 4 describes behavior rather than showing fresh captures.
- Filesystem paths are normalized to `/Users/alex/uem-terraform` (or `/Users/alex` for the install
  walkthrough) for readability. Every credential prompt is shown with a **placeholder** value or
  blank, never a real one; every org-group and object id/uuid in Section 5 is a consistent
  stand-in for the real value the tenant returned. Nothing else (prompts, values, error text)
  is altered from what the binary/script actually printed.

## Table of Contents

- [1. Overview](#1-overview)
- [2. Prerequisites](#2-prerequisites)
- [3. Install the CLI](#3-install-the-cli)
- [4. How `ws1-tf` finds and installs the `omnissa/uem` provider](#4-how-ws1-tf-finds-and-installs-the-omnissauem-provider)
  - [4.1 Picking the provider version: the compatibility table](#41-picking-the-provider-version-the-compatibility-table)
  - [4.2 Nothing installed at all](#42-if-nothing-is-installed-at-all)
  - [4.3 An installed provider doesn't match the tenant](#43-if-an-installed-provider-doesnt-match-the-tenant)
  - [4.4 Acquiring the provider ZIP (mirror mode)](#44-acquiring-the-provider-zip-mirror-mode)
  - [4.5 Installing the mirror](#45-installing-the-mirror)
  - [4.6 Console URL vs API host: the `cn`→`as` fallback](#46-console-url-vs-api-host-the-cnas-fallback)
- [5. Usage](#5-usage)
  - [5.1 First run / init wizard](#51-first-run--init-wizard-journey-steps-12)
  - [5.2 Verify](#52-verify-journey-step-3)
  - [5.3 Interactive menu](#53-interactive-menu-journey-step-4)
  - [5.4 List / list --type](#54-list--list---type-journey-step-5)
  - [5.5 Onboard](#55-onboard-journey-step-6)
  - [5.6 Final safety check: a clean `terraform plan`](#56-final-safety-check-a-clean-terraform-plan)
  - [5.7 Menu ↔ flag parity](#57-menu--flag-parity-journey-step-7)
  - [5.8 Reading the action log](#58-reading-the-action-log)
  - [5.9 `refresh-local`: fixing drift in place](#59-refresh-local-fixing-drift-in-place)

## 1. Overview

You already run Workspace ONE UEM in production — profiles, applications, scripts, sensors, years
of console-driven configuration. `ws1-tf` is the guided command-line tool that gets that environment
into Terraform: it verifies your tenant credentials against the real console before writing anything,
scaffolds a ready-to-use repo, and (for resource types the provider can enumerate) generates and
imports the matching Terraform code — checked, on the spot, for a clean `terraform plan`.

## 2. Prerequisites

- **Terraform 1.x** on `PATH` — required for `ws1-tf onboard`; not needed for `init`/`verify`/`list`
  (verified with Terraform v1.16.3). HashiCorp removed Terraform from Homebrew *core*, so
  `brew install terraform` no longer resolves — install from the HashiCorp tap instead:

  ```bash
  brew tap hashicorp/tap
  brew install hashicorp/tap/terraform
  terraform -version
  ```

  (Alternatively use `tfenv`, or download the binary from `releases.hashicorp.com/terraform`.)
  `ws1-tf` execs the literal `terraform` binary, so OpenTofu (`tofu`) does **not** satisfy this
  prerequisite as-is.
- **A Workspace ONE UEM tenant** — and credentials for it. (The examples in this guide were captured against a
  lab tenant, or a local fake server for a demo/dry-run.)

## 3. Install the CLI

`ws1-tf` is distributed as `ws1-tf-26.2.0.zip` (binaries only). Unzip it and run the
bundled installer:

```text
$ unzip ws1-tf-26.2.0.zip && cd ws1-tf-26.2.0
$ ls
install.sh                     README.md                      ws1-tf_26.2.0_darwin_universal
ws1-tf_26.2.0_linux_amd64      ws1-tf_26.2.0_linux_arm64
$ bash install.sh
Installed ws1-tf 26.2.0 -> /Users/alex/.local/bin/ws1-tf
Appended PATH export to /Users/alex/.zshrc

restart your shell or 'source /Users/alex/.zshrc' to pick up ws1-tf
or, to use it right now in THIS shell, run:
  export PATH="/Users/alex/.local/bin:$PATH"
$ export PATH="/Users/alex/.local/bin:$PATH"
$ ws1-tf version
ws1-tf 26.2.0 (commit a1b2c3d4e, built 2026-09-25)
$ ws1-tf --version
ws1-tf 26.2.0 (commit a1b2c3d4e, built 2026-09-25)
```

(Real, captured output above, run against `install.sh` exactly as shipped. Note: `/usr/local/bin`
wasn't writable in the environment this was captured in, so `install.sh` fell back to
`~/.local/bin` — that's why the transcript takes the fallback path below rather than the
"already on `PATH`" short-circuit.)

The version string carries a build date — `ws1-tf version`/`--version` print
`ws1-tf <ver> (commit <sha>, built <date>)`, and append `-dirty` to the commit when the build had
uncommitted changes. A build that does not set a build date omits the `, built <date>` clause
entirely rather than showing a blank date.

What `install.sh` actually does:

1. **Picks the right binary next to itself** for your OS/arch (`uname -s`/`uname -m`) —
   `ws1-tf_26.2.0_darwin_universal` on macOS (one binary, both Apple silicon and Intel), or
   `ws1-tf_26.2.0_linux_{amd64,arm64}` on Linux. It must be run from inside the extracted
   `ws1-tf-26.2.0/` folder — it looks for its sibling binaries in its own directory, not on `PATH`.
2. **Picks an install directory, no `sudo` ever:** `/usr/local/bin` if that's writable without
   elevation, else `~/.local/bin` (created if needed). On most locked-down Macs and most Linux
   boxes, that means `~/.local/bin`.
3. Copies the binary to a temporary file in the install directory, makes it executable, on macOS clears
   the quarantine attribute (`xattr -dr com.apple.quarantine`) so Gatekeeper doesn't block it, and
   renames it into place as `ws1-tf` — so a re-install replaces the file instead of rewriting it in
   place, and an interrupted install never leaves a half-written binary.
4. **If the install dir is already on `PATH`**, it says so and stops — nothing else to do (unless a
   different `ws1-tf` earlier on `PATH` still wins, in which case it names that file and tells you to
   remove it or put the install directory ahead of it).
5. **If not**, it idempotently appends an `export PATH=...` line to your shell rc — `~/.zshrc` for
   zsh, both `~/.bashrc` and `~/.profile` for bash, `~/.profile` otherwise — skipping the append (and
   saying so) if that exact line is already there from a prior run.
6. Finally it prints the paste-ready line for your *current* shell (shown above) so you don't have
   to open a new terminal to use `ws1-tf` right away.

## 4. How `ws1-tf` finds and installs the `omnissa/uem` provider

This section describes how `ws1-tf onboard` makes sure a usable `omnissa/uem` provider is available
to Terraform. Flags: `--consumption-mode dev|mirror|registry` (default: auto-detected; any other
value is an error).

### 4.1 Picking the provider version: the compatibility table

`ws1-tf init` reads the tenant's live UEM console version (`GET /api/system/info`) and looks it up
in a compatibility table that maps a UEM `MAJOR.MINOR` version to a provider version. Today the
table has one entry:

| UEM version | Provider version |
| --- | --- |
| `26.2` | `26.2.0` |

The raw version is normalized to a `MAJOR.MINOR` key first — it accepts a dotted string (`"26.2.0.0"` → `"26.2"`; leading zeros are dropped, so `"26.02.0.0"` gives
the same key) or a bare `YYMM` slug (`"2602"` → `"26.2"`). An unrecognized key falls back to a default version
(`26.2.0` today) and prints a warning; init still scaffolds successfully either way — the version
baked into the new tenant's `versions.tf` is *always* one of those two (table hit, or fallback).
A tenant that reports `26.2.1000.0` normalizes to `"26.2"` — a table hit,
so Section 5.1's capture shows no fallback warning. Against a tenant on a UEM version the table doesn't list, `init` prints it just
before the `Initialized …` line:

```text
warning: UEM version "27.0.1000.0" is not in the compatibility table; scaffolding the default provider 26.2.0 (verify before apply)
```

**`init` and `onboard` handle a version outside the table alike when it parses, and differently when it
does not.** `init` gives the warning above, and the default provider version, both for a version that
parses but is not in the table and for one that cannot be parsed at all. `onboard` re-reads the live
version. For a version that parses but is not in the table there is no target provider version, so
`onboard` keeps the version already pinned in `versions.tf`, asks nothing, and prints this warning so the
skipped check is not silent:

```text
warning: UEM version "27.0.1000.0" is not in the compatibility table; the provider version check was skipped and the pinned provider 26.2.0 is used (verify before apply)
```

No mismatch prompt can arise for such a version (a tenant on UEM 27.0 gets a warning at `init` and another at
`onboard`, then runs with the pinned `26.2.0`). A version `onboard` cannot parse stops it
with `unparseable UEM version (N bytes: <reason>)`, where the reason is one of a short fixed set such as
`major or minor is not a number` (an empty version gives `empty UEM version`); that error never contains the
version the server sent. The two warnings
above do quote the version string UEM reported, so they are the exception to the rule that a server-supplied
value stays out of messages (Section 5.1, "URL rules").

`init` itself never acquires or installs anything — it only writes that version string into
`versions.tf`. Acquisition/installation (below) happens later, at `ws1-tf onboard` time, when the
CLI **re-checks** the tenant's live version against the version already pinned in `versions.tf`
and, on a real mismatch, offers to fix it.

### 4.2 If nothing is installed at all

Independent of Section 4.3's mismatch check, and evaluated before it, `onboard` asks a second,
different question on every run: is `omnissa/uem` available to Terraform **at all** — not just at the right
version? `ws1-tf` resolves a *consumption mode* — `dev` (a `dev_overrides` config), `mirror` (a
local filesystem mirror) or `registry` — and, before the provider is
published to the public Terraform Registry, that Registry has nothing under `omnissa/uem`, so a plain
`terraform init` in `registry` mode is guaranteed to fail. `onboard` catches exactly this case with its
own consent prompt ("UEM console version" line is the version re-check from
Section 4.1, shown for context):

```text
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 1 item
  1. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 1
Provider omnissa/uem is not installed locally (no dev_overrides, no filesystem_mirror) and the public registry has nothing published pre-M2. Download and install the pre-release provider mirror now? [y/N]: y
  1. Compliance Test Profile (Apple iOS, profile_id 12350)
  2. Windows_Test_Profile (Windows 10, profile_id 12351)
```

(The prompt's own wording, "pre-M2," is the CLI's literal printed text — shown verbatim above. It
means "before the provider is published to the public Terraform Registry," same as elsewhere in
this section.)

How the mode is detected, in this order: `--consumption-mode`, when given, wins. Otherwise a `dev_overrides`
block for `omnissa/uem` counts first, because Terraform applies a `dev_overrides` entry over every other
installation method: it counts if it is in the Terraform CLI config file (the file `TF_CLI_CONFIG_FILE` names,
made absolute against the directory `ws1-tf` runs in when it is relative, or `~/.terraformrc` when that
variable is not set, or `%APPDATA%\terraform.rc` on Windows) as an entry of a `dev_overrides` block inside a
`provider_installation` block (Terraform ignores one anywhere else) with the key
`"omnissa/uem"` or `"<host>/omnissa/uem"`, so an override of another provider does not count. A block that is
commented out (`#`, `//` or `/* */`), or whose words only appear inside a
string or a heredoc, is ignored. A file that cannot be read with
confidence (an unterminated comment, string, heredoc or block) counts as no override. Failing that, an installed mirror
counts; otherwise the mode is `registry`. (A machine with both a `dev_overrides` block and an installed mirror
is therefore `dev`, the same answer Terraform gives; Section 4.3 says what `ws1-tf` prints then.)

The mirror check looks where Terraform looks. It reads the same CLI config file. When that file has a
`provider_installation` block, the mirror directories are the `path` of each `filesystem_mirror` in it that
serves `omnissa/uem` (an `include` list without a pattern that matches it, or an `exclude` pattern that matches
it, rules the entry out). A `path` that is not a plain literal string is not followed, and neither is an
`include` or `exclude` list that holds anything other than quoted strings. An explicit block turns
Terraform's implied plugins directory off, so a provider sitting only in `~/.terraform.d/plugins` does not count
when the block does not name that directory. With no `provider_installation` block, or no config file, the
directory is the implied one: `~/.terraform.d/plugins`, or `%APPDATA%\terraform.d\plugins` on Windows. A config
file that cannot be read with confidence (an unreadable file, an unterminated block, a `.json` config) counts as
nothing installed; `ws1-tf` does not guess. In a mirror directory the provider counts only when
`registry.terraform.io/omnissa/uem/<version>/<GOOS>_<GOARCH>/` (this machine's platform) holds a regular,
non-empty file whose name starts with `terraform-provider-uem`. A leftover temporary file from an interrupted
install, an empty file, a `.DS_Store`, another registry host such as
`local`, another platform's folder, and a directory do not count. Any version counts: the check does not look at
which version is there.

- **Yes** → acquires and installs the mirror ZIP the way Sections 4.4 and 4.5 describe (this needs a usable home
  directory: an unset or empty `HOME` stops the run with an error naming it), then
  re-detects the consumption mode so the corrected mode reaches every later `terraform init`/`plan`/`import`
  call in this run. This path installs the version this run needs (the one the compatibility table assigns,
  or the pinned one when the UEM version is not in the table). When that version differs from the pin, `onboard`
  then rewrites the pin in the tenant's `versions.tf` to it, so the pin agrees with what was just installed and
  the next run, against the same UEM version, finds no mismatch and does not offer to fix and download the
  same ZIP again; when the version is the pinned one nothing is rewritten. If the rewrite fails, the run stops
  with an error that names the step: `the provider <version> was installed, but updating the tenant provider
  pin to it failed: ...`. Nothing else is printed on a successful install (as shown above — the flow goes
  straight from the prompt into org-group/enumeration output), with two exceptions. A warning prints if the
  install reports success but re-detection still finds nothing (a broken mirror ZIP layout). And a
  warning prints if your Terraform CLI config has a `provider_installation` block that does not list the
  install directory as a `filesystem_mirror`, because Terraform then ignores that directory; it names the
  config file and the `filesystem_mirror { path = ... }` entry to add. The check stays quiet when the
  `provider_installation` block has a `network_mirror` or a `dev_overrides` block, or a `filesystem_mirror` that
  names the install directory and serves `omnissa/uem`, and when it cannot decide (a `path` that is not a plain
  literal string, an `include`/`exclude` list that holds anything other than quoted strings, or a file that cannot
  be read with confidence).
- **No** → prints a warning and proceeds anyway; the subsequent `terraform init` will very likely
  fail against the empty public Registry.

This is the prompt most first-time users actually hit — it fires on a completely fresh machine with
nothing installed yet, before Section 4.1's version check even matters. Section 5.5's demo capture pre-configures
`dev_overrides` (`TF_CLI_CONFIG_FILE`) before running `onboard`, so this prompt correctly does **not**
appear there — that demo is deliberately set up past this prompt, not evidence that the prompt is
rare.

### 4.3 If an installed provider doesn't match the tenant

When `onboard` detects a version **mismatch** it asks:

```text
Provider does not match this tenant. Fix it?
```

What counts as a mismatch is narrow: the provider version in the tenant's `versions.tf` differs from the version
the compatibility table (Section 4.1) assigns to the tenant's UEM version. Nothing else is compared. In
particular `ws1-tf` does not look at which provider build is actually installed (a mirror, a
`dev_overrides` binary, a file left by an earlier install): with the pin equal to the target and a different
version sitting in the mirror there is no prompt, and with a hand-edited pin there is a prompt even though the
right provider is installed. The pin is the `version` of the `uem` entry in the `terraform { required_providers { ... } }` block: a `version` inside a `#`, `//` or `/* */` comment, a commented-out `uem` entry, a heredoc line,
a string, or a `uem` object outside `required_providers` is never read as the pin, and the first live entry
wins. The version is compared as an exact string, so a hand-written range such as `~> 26.2` counts as a
mismatch. A `versions.tf` that cannot be parsed, or whose live `uem` version is not a plain quoted string (an
interpolation), is an error (`read pinned provider version: ...`), never a guess.

- **Yes** → `ws1-tf` runs a remedy chosen by how Terraform is resolving the provider (a `dev_overrides` config wins,
  as in Terraform, then an installed filesystem mirror, else registry):

  | Resolving via | Remedy |
  | --- | --- |
  | `registry` | rewrite the version pin in `versions.tf`, then run `terraform init -upgrade`, letting Terraform fetch the new version from the public Registry itself |
  | `mirror` (filesystem mirror) | not available automatically: `ws1-tf` prints `Automatic fix isn't available for this source/mode combination (...). Proceed with the version mismatch anyway?` and carries on only if you answer yes |
  | `dev_overrides` | not fixed automatically: Terraform is serving a locally built binary directly, so `ws1-tf` prints `cannot auto-fix under dev_overrides ...; update the override to a provider targeting UEM <major.minor>, or proceed anyway`, then asks `The provider could not be fixed automatically. Proceed with the version mismatch anyway?` |

  The registry remedy ends in the tenant directory (the one that holds
  `versions.tf`): the pin is rewritten to the version the table assigns, and `terraform init -upgrade` runs, so
  an existing `.terraform.lock.hcl` that still names the old version is re-selected (a plain `terraform init`
  refuses a lock that disagrees with the new pin). A failure names the step that failed, for example
  `terraform init -upgrade failed after pinning 26.2.0: ...`.

  The pin rewrite never touches version-looking text in a comment or a string, and it replaces the file
  atomically with its mode kept. It reads the pin back as a safety check; if that does not
  return the wanted version, `versions.tf` is put back as it was and the remedy fails with `the provider pin in
  versions.tf could not be updated to <version> reliably (the pin reader does not see the new version);
  versions.tf was left unchanged, edit the version by hand`. A pin whose version is not a plain quoted string (an
  interpolation) is refused with `no uem provider pin found in versions.tf`, and nothing is changed.

  Under `dev_overrides` nothing is changed. Answering yes to the proceed question continues the run and prints
  `warning: proceeding with pinned provider <pin> against a tenant needing <target>`; answering no stops
  `onboard` (`onboard halted: provider version mismatch, user declined`).

  **`dev_overrides` ranks first, as in Terraform.** Terraform applies a `dev_overrides` entry for `omnissa/uem`
  over every other installation method. On a machine that has both a `dev_overrides` block and an installed
  mirror, `ws1-tf` therefore detects `dev`, not `mirror`: installing a mirror or changing the pin would not change
  the provider Terraform runs. The `dev_overrides` remedy then adds one line after its usual explanation, and
  `onboard` prints the same line right after the `Using dev_overrides from ...` line at the start (Section 5.5):

  ```text
  note: a filesystem mirror with the omnissa/uem provider is also installed, but dev_overrides takes precedence: Terraform keeps using the override's binary, so installing a mirror or changing the provider pin will not take effect while the dev_overrides block stays
  ```
- **No** → `ws1-tf` asks `Proceed anyway with the current provider?`; declining either question
  aborts `onboard` cleanly, nothing else runs.

After any proceed-anyway answer (the warning line above was printed, and no remedy was applied) the run really
uses the pinned version: the work directory's `provider.tf` (next paragraph) and every enumeration step ask
Terraform for the pinned provider, not for the version the compatibility table assigns to the tenant.

After the remedy (or after a mismatch that was not fixed), `onboard` keeps its own working directory,
`<tenant>/.ws1tf-onboard-work`, whose `provider.tf` is rewritten on every run with the version in effect and
whose `.terraform.lock.hcl` survives between runs. When that directory is initialised (by `onboard` and by
`refresh-local` alike), `terraform init -upgrade` runs only if the lock records a different version of
`omnissa/uem` than the exact version `provider.tf` requires (a bare `26.2.0` or `= 26.2.0`); with no lock, the
same version, or a constraint that is not one exact version, it is an ordinary `terraform init`. Under
`dev_overrides` no init runs there at all.

### 4.4 Acquiring the provider ZIP (mirror mode)

To install the mirror, `ws1-tf` first needs the provider ZIP. It tries, in order:

0. **`--provider-zip <path>` / `WS1TF_PROVIDER_ZIP`** — an already-known ZIP location, for scripts
   and CI. Either one is used immediately and **never prompts**: it skips every other step below,
   including auto-discovery. `--provider-zip` wins whenever both are set. The same normalisation and
   validation described under step 2 apply — an invalid value (bad path, not a zip, wrong provider
   version, not a mirror ZIP) fails outright with the specific message, rather than falling back to
   the interactive prompt.
1. **`~/Downloads` auto-discovery (silent, exact match)** — looks for the exact, version-stamped
   filename `terraform-provider-uem-pre-release-<providerVersion>.zip`
   directly in `~/Downloads` — e.g., for a tenant that resolves to provider `26.2.0`, that's
   literally `terraform-provider-uem-pre-release-26.2.0.zip`. A valid match here is used straight
   away, with nothing printed. A file with that name that is not a valid provider mirror ZIP is not
   used: the reason is shown with the interactive prompt below instead.
2. **Interactive acquisition** — reached only when nothing above found or supplied a ZIP:
   - **Auto-discovery, with an offer** — before asking anything, `ws1-tf` looks (non-recursively)
     for that same version-stamped filename in `~/Downloads`, the current directory, and the
     `--path` repo root:
     - Exactly one match → `Found <path>. Use it? [y/N]`. Type `y` to use it; Enter (the default)
       declines and falls through to the plain prompt below (not a second offer).
     - Several matches (e.g. the same filename sitting in more than one of those directories) →
       they're listed, numbered, and a number picks one directly — typing a path instead still
       works.
     - None found → straight to the prompt below.
   - **The prompt, asked in a loop** — `Could not fetch <name>. Where did you put the ZIP? (path)`.
     Every invalid answer (a bad path, a directory that doesn't contain the ZIP, a file that fails
     validation) re-asks with the specific reason folded into the next prompt, rather than exiting.
     **An empty answer cancels** the whole acquisition, with a message saying to download the ZIP from
     the same place you got `ws1-tf`, then re-run and answer the prompt or pass the `--provider-zip`/
     `WS1TF_PROVIDER_ZIP` non-interactive alternative — never a crash.
   - **Pasted/dragged paths are normalised** before anything else touches them: surrounding
     whitespace is trimmed, one pair of surrounding single or double quotes is stripped,
     backslash-escaped characters are unescaped (`\ ` → space, `\(`/`\)` → `(`/`)`, and any other
     `\X` macOS's Finder produces when a file is dragged into Terminal), and a leading `~` expands to
     the home directory.
   - **A directory is accepted**, not just a file path: `ws1-tf` looks inside it (non-recursively)
     for the exact version-stamped filename. If it isn't there, the message says exactly which
     filename was looked for and in which directory, then re-asks.
   - **Validated before anything is installed from it** — the file must open as a real zip archive,
     and must contain entries under `mirror/registry.terraform.io/omnissa/uem/<providerVersion>/`, at least
     one of which is a regular, non-empty file whose name starts with `terraform-provider-uem`, and the
     version directory must hold such a provider file for this machine's platform (Section 4.5). Six distinct
     messages cover the ways it can fail:
     - `not a zip file`
     - `the zip has an entry with an unsafe path: "<name>"` — any entry under
       `mirror/registry.terraform.io/omnissa/uem/` (for any version) whose path below that prefix is not a plain
       relative path: a `..`, `.` or empty segment, a backslash, a NUL, a leading `/`, or a form that is not
       already clean. The whole archive is refused before its other entries are looked at.
     - `this zip contains provider version "X", but <providerVersion> is needed` (X is whatever other
       version directory the zip actually contains)
     - `not a provider mirror zip (no mirror/registry.terraform.io/omnissa/uem/ entries)`
     - `this zip has entries for provider version <providerVersion> but no non-empty terraform-provider-uem file`
       (a ZIP holding only a README, an empty placeholder or a symbolic link under the version directory would
       otherwise "install" and leave Terraform with no provider)
     - `the zip has no provider package for this machine (darwin_arm64) in provider version <providerVersion>; it
       only holds "darwin_amd64"` (the platforms it does hold are named), or, when no provider file sits in a
       `<os>_<arch>` directory at all, `the zip has no terraform-provider-uem file for this machine (...) in
       provider version <providerVersion>: expected mirror/registry.terraform.io/omnissa/uem/<version>/<os>_<arch>/terraform-provider-uem`.
       A packed `terraform-provider-uem_<version>_<os>_<arch>.zip` directly under the version directory is not accepted (Terraform
       does not find it there); the message then ends with a hint to unpack it into its `<os>_<arch>` directory.
       An arm64-only ZIP on an Intel Mac is refused here rather than "installing" a provider Terraform can't use.

(Note: the CLI's own release zip, `ws1-tf-<ver>.zip` from Section 3, is a different artifact from the
provider zip acquired here. Don't confuse the two.)

### 4.5 Installing the mirror

Once it has the ZIP, `ws1-tf`:

1. Extracts only the files of the provider version being installed — the entries under
   `mirror/registry.terraform.io/omnissa/uem/<providerVersion>/` — into the implied plugins directory
   (`~/.terraform.d/plugins/`, or `%APPDATA%\terraform.d\plugins\` on Windows),
   preserving the path structure underneath `mirror/` (e.g.
   `mirror/registry.terraform.io/omnissa/uem/26.2.0/darwin_arm64/terraform-provider-uem_v26.2.0` lands at
   `~/.terraform.d/plugins/registry.terraform.io/omnissa/uem/26.2.0/darwin_arm64/...`; on Windows the same
   layout sits under `%APPDATA%\terraform.d\plugins\`). Anything else
   in the ZIP (another provider, another version) is ignored. Every destination must sit inside that one
   version directory, `<plugins directory>/registry.terraform.io/omnissa/uem/<providerVersion>/`, so a ZIP
   cannot overwrite another provider's files or another UEM version's; it does replace the files of that same
   version if you already have them there (each file is written to a temporary file and renamed into place,
   so a re-install replaces the binary rather than rewriting it in place). It needs a real home directory: an
   empty or relative one is an error, not a relative install.

   The whole archive is checked **before anything is written**, and any of these refuses it:
   - an entry under `mirror/registry.terraform.io/omnissa/uem/` with an unsafe name (the rule of Section 4.4:
     no `..`, `.` or empty segment, no backslash or NUL, not absolute, already in clean form);
   - a version directory with no regular, non-empty `terraform-provider-uem*` file for **this machine's**
     `<os>_<arch>` directory (for example `darwin_arm64/`). A packed
     `terraform-provider-uem_<version>_<os>_<arch>.zip` directly under the version directory does NOT count:
     Terraform does not find a package there, so the refusal names it and says to unpack it into its
     `<os>_<arch>` directory. Packages for other platforms may sit beside it and are extracted too, but an
     archive that holds only other platforms is refused, so an arm64-only ZIP on an Intel Mac does not
     install;
   - an entry in the version directory that is a symbolic link, a device or any other non-regular file;
   - more than 1024 files in the version directory, a single entry over 512 MiB, or more than 2 GiB in all.

   Any destination that is, or sits below, a symlink inside the plugins directory is refused while writing.
2. Makes the Terraform CLI config use that mirror, without disturbing what is already there. The file edited
   is the one Terraform itself reads: the file `TF_CLI_CONFIG_FILE` names (a relative path is made absolute once,
   against the directory `ws1-tf` runs in, and `terraform` is handed that same absolute path), else `~/.terraformrc`,
   else `%APPDATA%\terraform.rc` on Windows. It is worked out before anything is extracted; if it cannot be (no
   `HOME`, or no `APPDATA` on Windows) the install stops with `cannot determine the Terraform CLI config file
   to edit (set TF_CLI_CONFIG_FILE, or make sure HOME is set; on Windows APPDATA)` and writes nothing. The
   plugins directory is checked the same way: on Windows an empty or relative `%APPDATA%` stops the install with
   an error naming it, before anything is written.
   - no file yet: it is created (mode 0600) with the block below, and its parent directory is created (mode
     0700) if it does not exist, for example for a `TF_CLI_CONFIG_FILE` that points into a new folder;
   - a file without an active `provider_installation` block: the block is appended, and the previous content
     is kept next to it as `<config file>.bak` (`.terraformrc.bak` for the default file; an existing `.bak`
     is replaced, so a second backup of this kind overwrites the first). Only an active block counts: a
     `provider_installation` that is commented out (`#`, `//` or `/* */`), or that only appears inside a string
     or a heredoc, is not a block;
   - a `provider_installation` block that already has a `filesystem_mirror` pointing at
     the plugins directory **and serving `omnissa/uem`**: nothing is written (idempotent). A mirror
     serves it when its `include` list is absent or has a pattern that matches `omnissa/uem` (a
     `[host/]namespace/type` pattern, `*` allowed in any part) and no `exclude` pattern matches it. A
     `filesystem_mirror` mentioned elsewhere, pointing at some other path, or restricted to other providers
     or excluding `omnissa/uem`, does not count and falls to the next case;
   - a `provider_installation` block that does not list this mirror: Terraform allows only one such
     block, so the file is left untouched and `ws1-tf` stops with a message that names the file and
     prints the exact `filesystem_mirror`/`direct` blocks to add inside your existing block. The
     provider files are already installed at that point; add the blocks and run `ws1-tf` again;
   - a CLI config whose name ends in `.json` (Terraform reads it as JSON): it is never edited. `ws1-tf` stops
     with a message that says so and prints the blocks in HCL syntax for you to translate and add.

   Terraform reads this file with an older HCL reader that accepts syntax ordinary HCL rejects, such as the
   quoted argument name in a `dev_overrides` entry (`"omnissa/uem" = "/path"`), so a config that does not
   parse as ordinary HCL is not an error. `ws1-tf` then reads it with a tolerant scan that skips comments
   (`#`, `//`, `/* */`), understands double-quoted strings and heredocs, and counts only active
   `provider_installation` blocks. A file the scan cannot read with confidence (an unterminated comment, string,
   heredoc or block) is left untouched and gets the same manual-merge message as above; it is never appended to.

   A symlinked config file is edited through the link (the `.bak` copy sits next to the file the link
   resolves to), and the write is atomic (mode preserved). When `TF_CLI_CONFIG_FILE` is set, that file, not
   `~/.terraformrc`, is the one merged.

   ```hcl
   provider_installation {
     filesystem_mirror {
       path    = "<plugins directory>"   # <home>/.terraform.d/plugins, or %APPDATA%\terraform.d\plugins on Windows
       include = ["omnissa/uem"]
     }
     direct {
       exclude = ["omnissa/uem"]
     }
   }
   ```

With that block in place, a plain `terraform init` in a config whose `versions.tf` declares
`source = "omnissa/uem"` resolves the provider from the local mirror — no dev-override warning, no
registry lookup for that address — as long as the pin in `versions.tf` names a version the mirror holds (the
install of Section 4.2 rewrites the pin for you when it differs from the version installed).

### 4.6 Console URL vs API host: the `cn`→`as` fallback

UEM consoles are conventionally reachable at `cnXXXX.<domain>`, but the REST API for that same
tenant can be served at a *different* host, `asXXXX.<domain>` (same number, same domain/port/path/
scheme — only the leading `cn` changes to `as`). If you enter the console URL during `init`/
`onboard` and it doesn't answer `/api/system/info`, `ws1-tf` derives the `as`-prefixed candidate
and probes that instead
— silently, on success: the
corrected URL is what gets written to `.env`/persisted, with only an informational
`console URL <cn-url> did not answer the API; using <as-url> instead` line printed. If *neither*
host answers, the error names both hosts tried.

The fallback is deliberately narrow, because the probe sends the same credentials to the derived host:

- It applies only when the **first** host label is `cn` followed by digits (`cn1234`, alone or followed by more
  labels; any capitalisation). `cnn.example.com`, `cname.example.com` or `portal.cn1234.example.com` get no
  fallback.
- The candidate keeps the scheme, port and path of what you entered, and never its username/password,
  query or fragment.
- Every URL `ws1-tf` prints in the `console URL ... did not answer` line and in the errors is shown as
  scheme, host, port and path only (see Section 5.1 for the URL rules).

**Caveat, stated plainly:** as of this writing, the fallback has never
been observed firing against a real tenant. In the lab tenants checked, the console host (`cn...`)
already answers `/api/system/info` directly, so the probe succeeds on the first try and the fallback
never runs. The `cn`→`as` swap logic itself passed its automated tests against stand-in servers for the two hosts,
just not yet a live tenant where the two hosts genuinely differ.

## 5. Usage

Every example below (except where noted) runs live against a real Workspace ONE UEM lab tenant,
scoped to two read-only org groups the credentials used for this doc can see: **Global** (a
Container, id `12345`) and **Acme** (a Customer, id `12346`) — these names/ids are consistent
placeholders substituted for the lab tenant's real org-group names and ids throughout this section;
every other id/uuid below is likewise a consistent stand-in for a real value the tenant returned
(the same real value always maps to the same placeholder). No `apply`/`destroy` was run and no API
writes were made anywhere in this section — `terraform plan` only reads.

### 5.1 First run / init wizard (journey Steps 1–2)

An empty folder, first `ws1-tf` invocation. Answering the wizard hands straight into setup — proving
the console URL and credentials live against the tenant, showing back the UEM version for you to
confirm, then listing real org groups to scope into, **before writing the scaffold or the credentials**
(the run's own action log is the one exception, see the note under the folder listing):

```text
$ ws1-tf --path /Users/alex/uem-terraform
You're in /Users/alex/uem-terraform. Initialize this path as your repo? [y/N]: y
Tenant name (e.g. dev, test, prod): prod
Console URL (instance_url): 
API tenant code (tenant_code / aw-tenant-code): 
Username (basic auth): 
Password (basic auth): 
OAuth client_id: 
OAuth client_secret: 
OAuth token URL (oauth2_token_url): 
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 2
Initialized /Users/alex/uem-terraform (tenant "prod"). Credentials written to /Users/alex/uem-terraform/prod/.env
```

**Why every field above shows nothing after the colon, not just the secret ones:**

- This is a real tenant. Every credential value — console URL, tenant code, username, password,
  client_id, client_secret, oauth token URL — was supplied by a **script**, sourced from environment
  variables, so none was ever typed at a keyboard or displayed on screen. This block is a composite:
  the credential prompts are from that scripted run, while the org-group picker header
  (`Page 1/1 · 2 items`, with its `(←/→ page, / search)` hint) is how the picker renders on a real
  terminal; with piped input the picker prints a plain numbered list and a plain prompt instead
  (see "Paging, search, and range selection in the pickers" below).
- The secret prompts (tenant_code, password, client_secret) fall back to a plain, unmasked read when
  there's no tty to mask, and never echo back what they read. Every other prompt (including
  the console URL) never echoes back what it read either, tty or not — it only writes the prompt
  label. So in a scripted run, literally nothing shows for any field, not just the three
  secret ones.
- Formatting note: the prompts up through "UEM console version:" don't print a trailing newline, so
  the raw captured output there is one long unbroken line — reformatted one-prompt-per-line here for
  readability, same as this doc's path normalization. The org-group list and everything after it are
  genuinely separate lines in the raw output.

The tenant name (`prod` above) becomes a directory name, so it must be 1–64 letters, digits, `.`, `_` or `-`
and start with a letter or digit (no spaces, slashes or other characters); `uem-artifacts` is reserved
for the repo's own use. Anything else is refused with a message saying so.

Three things worth calling out, all **real observed behavior**, not simplification for the doc:

- Every credential set you fill in completely is verified live against the tenant, independently:
  running `ws1-tf verify` right after this init shows **both** `basic: true` and `oauth: true` for
  this tenant (Section 5.2) — proving both sets verified — and the written `.env`'s `UEM_AUTH_METHOD` comes
  back `oauth2`, confirming OAuth2 was preferred once both verified, exactly as documented.
- **Org groups are not returned in console order.** `ws1-tf` sorts "Container" groups (organizing
  groups) before "Customer" groups, and within that, by ascending ID. This lab tenant only has one
  of each visible to these credentials, so the ordering isn't dramatic here, but the rule is the
  same one a larger tenant hits: read the list, don't assume position.
- **Org-group listing is now fully paged, never silently truncated.** `SearchOrgGroups`
  walks every page of the tenant's org-group search API and returns the complete list or an error —
  never a partial list — so there is no truncation warning to show, and won't be even against a
  tenant with far more org groups than fit on one API page. "Complete" is checked against the
  response's own `Total`: a response with no `Total` (missing or `null`) is an error, not an empty
  list, and so is an entry with no `Id`, a count that disagrees with the `Total`, a page that adds nothing
  new, or a list that changes while it is being read. The same applies to the smart-group
  search that `onboard` uses, and a response body of JSON `null` is an error for every UEM read.

#### Resulting folder structure

```text
$ find /Users/alex/uem-terraform -type f | sort
/Users/alex/uem-terraform/.gitignore
/Users/alex/uem-terraform/.ws1-tf.yaml
/Users/alex/uem-terraform/prod/.env
/Users/alex/uem-terraform/prod/README.md
/Users/alex/uem-terraform/prod/versions.tf
```

(`init` also writes its own action log under `.ws1tf/logs/` at the repo root, which is gitignored and
left out of the listing — see Section 5.8. The log file is created when `init` records its first action, and
the first thing `init` records is the first call to the console while it verifies your credentials, before
anything else is written. So once a console call was made, the `.ws1tf/` folder exists even when `init` then
fails, and a `--path` that does not exist yet has been created by that step; if `init` stops earlier (before
any call), nothing is created. If you mistype `--path` for `init`, you can therefore end up with an empty
directory that holds only a `.ws1tf/` folder. Commands that never record anything — `version`, `list`, or
`verify` against a path with no tenant — create no `.ws1tf/` folder and no directory at all.)

**A fresh clone has no `.env`.** The `.env` files are never committed, so after `git clone` the repo is
initialized (the marker file and the tenant folder with its `README.md` and `versions.tf` are committed) but
no tenant has credentials. `ws1-tf init` there, and the interactive menu when it finds no environment, offer to
create them instead of ending at an error that points back at `init`:

```text
No tenant has stored credentials (.env is not committed). Add them now?
```

Answering yes asks for a tenant name, then runs the same credential prompts, console check, version
confirmation and org-group pick as above. If the repo already has a tenant folder of that name (the committed
one), only its `.env` is written, never over an existing one, and the run ends with `Created credentials for tenant
"prod" against UEM version <version> (org group: <name>, id <id>). Written to <path>`; a name the repo does not have
yet gets the full scaffold, as in the first-run transcript. A symlink or a file where the tenant folder would go is
refused. Answering no (or declining the version confirmation) ends the command, on a terminal, with the
original error ``no tenant .env found under <dir> (run `ws1-tf init` first)``. With piped input that answers no,
`init` prints that message and shows the single-shot menu of Section 5.3, as it did before; piped input that
ends before the question is answered is an end-of-input error.

`.ws1-tf.yaml` (repo-root marker — records the schema version and the tenants added to this repo
through `init`):

```text
$ cat /Users/alex/uem-terraform/.ws1-tf.yaml
schema_version: 1
tenants:
  - prod
```

`.gitignore` (repo root — written when the repo is first set up, covers every tenant; `onboard` adds
the `/uem-artifacts/` rule if it is missing, so a repo created by an earlier
build is covered after its next `onboard` run — if you manage `.gitignore` yourself, make sure it
contains `/uem-artifacts/`, because secrets and downloaded app binaries live under it):

```text
$ cat /Users/alex/uem-terraform/.gitignore
# ws1-tf managed — never commit credentials or state
.env
.env.tmp-*
*.tfstate
*.tfstate.*
.terraform/
.terraform.lock.hcl
.ws1tf/
# Downloaded uem_mac_application binaries: kept in the repo folder (not your
# home folder), but never committed.
/uem-artifacts/
```

`prod/versions.tf` (the provider block every resource in this tenant uses):

```text
$ cat /Users/alex/uem-terraform/prod/versions.tf
terraform {
  required_providers {
    uem = {
      source  = "omnissa/uem"
      version = "26.2.0"
    }
  }
}

provider "uem" {
  # Credentials are supplied via UEM_* environment variables from this
  # tenant's .env file (never committed). See README.md.
}
```

`prod/README.md` (generated hand-off doc for this tenant):

```text
$ cat /Users/alex/uem-terraform/prod/README.md
# Workspace ONE UEM — Terraform-managed environment (prod)

Generated by ws1-tf. Provider: `omnissa/uem` `26.2.0`.

## Environment

- UEM console version: `26.2.1000.0`
- Org group scope: `Acme (id 12346)`

## Credentials

This tenant's UEM credentials live in `prod/.env` (gitignored, chmod 0600).
Before running Terraform by hand, load them:

    set -a; source prod/.env; set +a

## Generated configuration

`ws1-tf onboard` writes everything it generates into `prod/.ws1tf-onboard-work/`:
one `.tf` file per type (`profiles.tf`, `mac_applications.tf`, ...), `dependencies.tf`
(org group and smart group lookups), and `provider.tf`. A smart group that onboard
imports because another object targets it goes into that object's file (for example
`profiles.tf`), not `smart_groups.tf`. That folder is the Terraform root for the
onboarded objects, and it holds their state, so run Terraform there:

    set -a; source prod/.env; set +a
    terraform -chdir=prod/.ws1tf-onboard-work plan

`prod/versions.tf` records the provider source and version this tenant was
set up with. `ws1-tf onboard` reads the provider version from it and compares it with
the version this tenant's UEM version needs; when they differ, the fix it offers can
rewrite that version. Keep the file in place; it is also the starting point if you
write your own Terraform configuration at the tenant root.

## Workflow (manual apply)

1. `main` is a static mirror of the live UEM environment; keep it locked.
2. To change: branch → edit → PR into `main` → a human checks out `main` and runs
   `terraform apply` BY HAND. There is no automated apply.
```

`prod/.env` (the credentials — gitignored, never committed; **placeholder values shown here**,
never the real ones):

```text
$ cat /Users/alex/uem-terraform/prod/.env
# ws1-tf managed — UEM credentials. NEVER commit this file.
UEM_INSTANCE_URL='https://your-instance.awmdm.com'
UEM_TENANT_CODE='TCODE'
UEM_AUTH_METHOD='oauth2'
UEM_USERNAME='svc'
UEM_PASSWORD='pw'
UEM_CLIENT_ID='client-id'
UEM_CLIENT_SECRET='client-secret'
UEM_OAUTH2_TOKEN_URL='https://your-instance.awmdm.com/oauth/token'
```

Every value is written POSIX single-quoted, so `set -a; source prod/.env; set +a` loads each one exactly
as stored, whatever characters a credential contains (`$`, backticks, quotes, spaces).

**Editing `.env` by hand.** `ws1-tf` reads the file the way the shell does for the cases a hand edit uses:

- A value can be single-quoted, double-quoted or one unquoted word. Double-quoted values are read first as a
  Go-quoted string (the form earlier builds wrote) and otherwise with the shell's `\"`, `\\`, `\$` and
  `` \` `` escapes. A leading `export` and a trailing `# comment` are accepted (`KEY='a' # note`). When a key
  appears more than once the last assignment wins, as under `source`.
- A malformed value is an error that names the file and line, never the value: an unterminated quote, a
  dangling backslash, or text after an unquoted space (a shell would run that as a command, not assign it).
- The loader does not interpret `$`, backticks, command substitution or shell operators, so `UEM_PASSWORD=$X`
  loads the literal text `$X`, whereas `source` would expand it. Single-quote any value that holds such
  characters.
- `UEM_AUTH_METHOD` is used as written. When it is empty, `onboard` and `verify` infer the method the way the
  provider does: `oauth2` when both `UEM_CLIENT_ID` and `UEM_CLIENT_SECRET` are set, otherwise `basic` when both
  `UEM_USERNAME` and `UEM_PASSWORD` are set. When neither pair is complete, `onboard` stops with `no UEM auth
  method is configured and none can be inferred: set UEM_AUTH_METHOD (basic or oauth2) and the matching
  credentials in the tenant .env (...)` before it calls UEM; `verify` still prints its two diagnostic checks and
  then fails with the same message.
- When `ws1-tf` rewrites the file (a credential update, or the corrected URL after the `cn`→`as`
  fallback) it changes only the `UEM_*` assignments. Every other line (comments, extra
  variables, blank lines), the line order and an `export` prefix are kept. A symlinked `.env` is updated
  through the link, and the new content is fsynced before it replaces the old file. A first-time write never
  overwrites an existing `.env`.

**URL rules.** The console URL and the OAuth token URL are checked when you enter them and again whenever
they are used. Each must be `https` (a missing scheme means `https://`; plain `http` is accepted only for
`localhost`, `127.0.0.1` and `::1`), must have a host, and must not carry a username or password. The console
URL must also have no query (`?...`) or fragment (`#...`): it is rejected rather than stripped, because every
request is the URL plus `/api/...`. The token URL may carry a query. A rejected URL is never echoed back, and
errors from UEM calls name only the kind of failure (an HTTP status, `timed out`, `unreachable`, `unexpected
response shape`, or the JSON type that arrived), never a part of the response body or of a server-supplied
value.

### 5.2 Verify (journey Step 3)

Confirm connectivity on demand — after a password rotation, or before a teammate picks the repo back
up — without repeating the wizard:

```text
$ ws1-tf verify --path /Users/alex/uem-terraform/prod
basic: true  oauth: true  version: 26.2.1000.0
```

`verify` checks both basic and OAuth independently and prints both results as a diagnostic, but its exit status
follows the **configured** auth method (`UEM_AUTH_METHOD` in the tenant's `.env`): it exits non-zero when that
method did not authenticate, even if the other one did, because `onboard` and the provider use only the
configured method, so a green `verify` while it is broken would pass a script's gate and then fail the next
`onboard`. The `auth_method` it records in the action log's `tenant_context` line is that same method. When
`UEM_AUTH_METHOD` is empty, `verify` infers the method the way the provider does: `oauth2` when both
`UEM_CLIENT_ID` and `UEM_CLIENT_SECRET` are set, otherwise `basic` when both `UEM_USERNAME` and `UEM_PASSWORD`
are set; when neither pair is complete it still prints its two diagnostic checks and then fails with
`no UEM auth method is configured and none can be inferred: ...`. A value other than `basic` or `oauth2` fails with
`verification failed: UEM_AUTH_METHOD must be basic or oauth2`. `basic: true` means
`GET /api/system/info` answered with your username and password. `oauth: true` means both that the token
request succeeded **and** that `GET /api/system/info` answered with that token against your console URL
and tenant code, because the token endpoint is a separate host and a token alone proves nothing about the
instance. The version comes from the basic check, or from the OAuth probe when only OAuth works, so an
OAuth-only tenant shows it too. When a check fails, `verify` prints which one failed, with the reason, above the
summary line; an OAuth set that is wholly blank is skipped without a message and shows `oauth: false`. `verify`
does not use the `cn`→`as` host fallback of Section 4.6: it talks to the console URL as stored.
This tenant has both credential sets configured and both verify live — that's also *why* Section 5.1's init
inferred `oauth2` as the auth method (OAuth2 wins when both sets verify).

### 5.3 Interactive menu (journey Step 4)

Re-running bare `ws1-tf` against an already-initialized directory skips the wizard and enters a
menu. Its shape now depends on whether you're actually at a terminal:

- **A real terminal (stdin AND stdout are both TTYs)** — running `ws1-tf` by hand — gets the
  **interactive menu**: it *loops*. Pick an action, watch it run, then either press Enter to come
  back to the menu or `q` to quit. The one-environment transcript right below is the
  real output of the menu's own code path (driven end to end through `NewRootCommand().Execute()`
  with a scripted input, not hand-written prose), and the several-environments transcript further
  down is a genuine real-terminal capture — a live `tmux` session driving this exact binary, with
  a real pty, against two real UEM tenants.
- **Anything else** (piped stdin, a script, CI, or a non-TTY stdout) still gets the **original
  single-shot menu**, byte for byte unchanged: one selection, one dispatch, then exit — see
  [Non-interactive (single-shot) menu](#non-interactive-single-shot-menu) below.

Every action in the interactive menu runs against a specific **environment** — a tenant
subdirectory holding a `.env` (the same rule `verify`'s own tenant-directory resolution uses, so
"environment" here means exactly what `--path <tenant-dir>` already meant). With one environment
there's nothing to pick; with several, the menu asks you to pick one before the loop starts, and a
"Switch environment" item lets you change which one every later action targets, without
restarting `ws1-tf` or touching `--path`.

**One environment** — no picker, and the header names the only environment there is:

```text
$ ws1-tf --path /Users/alex/uem-terraform
Environment: prod — UEM 26.2.1000.0 · org group "Acme" (12346)
Available actions:
  1. Onboard existing resources
  2. List onboardable types
  3. Verify UEM connectivity
  4. Add an environment
  5. Update credentials for prod
  6. Show version
Select an action by number, q to quit: 6
ws1-tf 26.2.0 (commit a1b2c3d4e, built 2026-09-25)
Press Enter to return to the menu · q to quit: q
```

The header's `UEM <version>` and `org group "<name>" (<id>)` clauses are read straight out of this
tenant's own `README.md` (the one `init` wrote) — no API call is made to build the header. Either
clause is dropped if the README has no recorded value for it (for example, no org-group scope). An
unrecognized UEM version does not drop it: `init` warns that the version is not in the compatibility table,
still records the version the tenant reported, and scaffolds the default provider version (`26.2.0`).
With only one environment the trailing `(N environments)` clause is dropped entirely. The README and the
tenant folder names are repo content that another contributor may have written, so the environment name and
those two values are cleaned before they are printed (control characters become a space, format characters such
as bidirectional overrides are removed), in the header, in the environment picker and in the `Switch
environment` and `Update credentials` menu lines.

If the repo has no environment at all — a fresh clone, where no tenant has a `.env` — the interactive menu does
not stop at an error: it asks `No tenant has stored credentials (.env is not committed). Add them now?` and, on
yes, runs the add-credentials flow described in Section 5.1 under "A fresh clone has no `.env`", then continues
into the menu with the new environment. Declining ends the menu with the `no tenant .env found under <dir>` error.

At the "Select an action" prompt: a number dispatches that action, prints its output (or, on
failure, `error: ...`) and then waits at "Press Enter to return to the menu · q to quit" — Enter
redisplays the menu, `q` quits at once, exit code `0`. `q` or a blank Enter at the *main* prompt
also quits immediately. A closed stdin (`EOF`) is always its own distinct error
(`no input available (non-interactive session)`) — never confused with an explicit quit.

**Several environments** — a picker up front, plus a "Switch environment" item that changes which
environment every subsequent action (including "Update credentials for `<env>`") targets. Unlike
every other example in this section, this one spans **two separate real UEM tenants** rather than
two org groups on one tenant — captured from a real terminal: a live `tmux` session, a real pty,
driving this exact binary against both tenants, piped through a `sed` filter that masked each
tenant's console host and tenant code to this doc's usual placeholders (`your-instance.awmdm.com` /
`TCODE`) as the sole change to the byte stream. The environment names, org group names and ids are then
replaced with this doc's stand-ins (`prod`/`dev`, `Acme` (`12346`)/`Acme Dev` (`12347`)), and the repo path is normalized to this doc's
usual `/Users/alex/uem-terraform`, same as this doc's other path normalization. The credential
prompts below (the "Update credentials for dev" action) were answered with every field left
blank, on purpose, to reach the retry-then-fail path without touching either tenant's real
credentials — collapsed below to one instance of the 7 prompts plus a note, rather than all 22
blank answers across 5 attempts:

```text
$ ws1-tf --path /Users/alex/uem-terraform
Select the environment to use — Page 1/1 · 2 items
  1. prod
  2. dev
Select the environment to use (number) (←/→ page, / search): 1
Environment: prod — UEM 26.2.1000.0 · org group "Acme" (12346)   (2 environments)
Available actions:
  1. Onboard existing resources
  2. List onboardable types
  3. Verify UEM connectivity
  4. Switch environment (prod, dev)
  5. Add an environment
  6. Update credentials for prod
  7. Show version
Select an action by number, q to quit: 3
basic: true  oauth: true  version: 26.2.1000.0
Press Enter to return to the menu · q to quit: 
Environment: prod — UEM 26.2.1000.0 · org group "Acme" (12346)   (2 environments)
Available actions:
  1. Onboard existing resources
  2. List onboardable types
  3. Verify UEM connectivity
  4. Switch environment (prod, dev)
  5. Add an environment
  6. Update credentials for prod
  7. Show version
Select an action by number, q to quit: 2
TYPE                              STATUS  DETAILS
profile                           ready   Device profiles and their payload settings.
...(all 10 onboardable types printed — see §5.4 for the full table)...
smart_group                       ready   Smart groups and their criteria.
Press Enter to return to the menu · q to quit: 
Environment: prod — UEM 26.2.1000.0 · org group "Acme" (12346)   (2 environments)
Available actions:
  1. Onboard existing resources
  2. List onboardable types
  3. Verify UEM connectivity
  4. Switch environment (prod, dev)
  5. Add an environment
  6. Update credentials for prod
  7. Show version
Select an action by number, q to quit: 4
Select the environment to use — Page 1/1 · 2 items
  1. prod
  2. dev
Select the environment to use (number) (←/→ page, / search): 2
Press Enter to return to the menu · q to quit: 
Environment: dev — UEM 26.2.1100.0 · org group "Acme Dev" (12347)   (2 environments)
Available actions:
  1. Onboard existing resources
  2. List onboardable types
  3. Verify UEM connectivity
  4. Switch environment (prod, dev)
  5. Add an environment
  6. Update credentials for dev
  7. Show version
Select an action by number, q to quit: 6
Console URL (instance_url): 
API tenant code (tenant_code / aw-tenant-code): 
Username (basic auth): 
Password (basic auth): 
OAuth client_id: 
OAuth client_secret: 
OAuth token URL (oauth2_token_url): 
...(the username/password/client_id/client_secret/token-URL group repeats, every field left blank,
for 5 attempts total — collectCredentialSets' own retry cap)...
error: no complete credential set provided after 5 attempts (need EITHER a username+password, OR a client_id+client_secret+token URL)
Press Enter to return to the menu · q to quit: 
Environment: dev — UEM 26.2.1100.0 · org group "Acme Dev" (12347)   (2 environments)
Available actions:
  1. Onboard existing resources
  2. List onboardable types
  3. Verify UEM connectivity
  4. Switch environment (prod, dev)
  5. Add an environment
  6. Update credentials for dev
  7. Show version
Select an action by number, q to quit: 7
ws1-tf 0.0.0-dev (commit unknown)
Press Enter to return to the menu · q to quit: q
```

(`ws1-tf 0.0.0-dev (commit unknown)` above is genuinely what `Show version` printed: this capture used a
development build; see the `ws1-tf 26.2.0 (commit ..., built ...)` lines elsewhere in this section for what a released
build reports.)

"Add an environment" runs the exact same collect-credentials-then-verify flow `ws1-tf init` uses,
for a brand-new tenant name — it refuses a name that's already in use rather than silently
overwriting that environment's stored credentials. "Update credentials for `<env>`" runs the
credential-update flow (the same one the explicit `ws1-tf init` subcommand offers on an
already-initialized directory) directly against the environment CURRENTLY selected in the menu —
never against whatever `--path` happens to point at or whatever the working directory is, so
switching environments and then updating credentials updates the right one.

Options 1–3 (onboard/list/verify) behave exactly as Sections 5.4/5.5/5.2 describe, just scoped to
the selected environment; onboard with no `--type` still falls straight into Section 5.5's numbered
type menu.

#### Non-interactive (single-shot) menu

Piped stdin, a script, CI, or a non-TTY stdout gets the ORIGINAL menu, completely unchanged:

```text
$ ws1-tf --path /Users/alex/uem-terraform
This directory is already initialized.
Available actions:
  1. Re-run init / update credentials  (ws1-tf init --path <dir>)
  2. Show version  (ws1-tf version)
  3. Verify UEM connectivity  (ws1-tf verify --path <dir>)
  4. List onboardable types  (ws1-tf list)
  5. Onboard existing resources  (ws1-tf onboard --type <t>)
Select an action by number, or press Enter to exit: 2
ws1-tf 26.2.0 (commit a1b2c3d4e, built 2026-09-25)
```

Three real, captured outcomes for this prompt:

- **A number 1–5** dispatches that exact action in-process (shown above: `2` ran the real `version`
  command and printed its real output — not a hint to go run it yourself). Option 5 (onboard) with
  no `--type` falls straight into Section 5.5's numbered type menu, since `onboard`'s own `RunE` already
  handles the no-`--type`-on-a-real-terminal case.
- **A blank line (just Enter)** exits cleanly, no error, exit code `0`.
- **Anything else** — an out-of-range number, or non-interactive `EOF` (no terminal, stdin closed) —
  is reported as its own distinct error rather than silently falling through to exit:
  `error: invalid selection "9" (want a number between 1 and 5)` or
  `error: no input available (non-interactive session)`, respectively. This is deliberate:
  a single-shot ask, no retry loop, so an invalid choice and a
  closed pipe stay two distinguishable failures instead of blurring into one generic error.

This is the exact menu (and the exact `showMenu` code path) this CLI has always shown outside a real
terminal — nothing about it changed when the interactive loop above was added; every existing
non-interactive test still passes unmodified against it.

### 5.4 List / list --type (journey Step 5)

An honest, type-by-type answer to "what can I bring into Terraform today?" — **every one of the 10
onboardable types is enumerable now**:

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

The table fits the terminal: the TYPE and STATUS columns keep their width, and DETAILS wraps inside
its own column (the capture above is at 100 columns; `$COLUMNS` overrides the width, and output that
isn't a terminal gets 100). `--type` shows one type's details — what it onboards, the data source it
lists from, its Terraform import ID and any caveat. All 10 types are onboardable end to end today,
live-verified one by one in Section 5.5.

### 5.5 Onboard (journey Step 6)

`onboard` is the payoff: pick a type, see what's live, choose what to bring in, and let `ws1-tf`
generate Terraform code, import it into state, and check the result.

> **Where the work lives.** `onboard` writes the generated Terraform configuration and the Terraform state
> into a hidden folder inside the tenant directory, `<tenant>/.ws1tf-onboard-work/` (for example
> `/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/`). See "Resulting folder structure (onboard-work
> directory)" below for its contents. Use the provider version
`ws1-tf` installs for your tenant (Section 4.2); do not point Terraform at a hand-built provider
binary. The captures below were recorded with a pre-release provider loaded through Terraform's
`dev_overrides`, so Terraform prints its own (unrelated to `ws1-tf`) warning banner on every `plan`.
When `dev_overrides` are active, `onboard` also prints the CLI config file that set them and the
provider binary's age, so a stale build is visible. The binary is the first regular file in the override
directory named `terraform-provider-uem`, `terraform-provider-uem.exe` or `terraform-provider-uem_v<version>...`
(an archive such as `terraform-provider-uem_v1.zip`, or another provider's binary, is never taken for it); when
none is found the line names the config file only. It is printed whenever the CLI config sets an override for
`omnissa/uem` (which is also the mode Section 4.2 detects, because `dev_overrides` ranks first). When a
filesystem mirror holding the provider is installed as well, a second `note: ...` line follows (Section 4.3)
saying that the override wins and a mirror install or pin change will not take effect:

```text
Using dev_overrides from /Users/alex/dev.tfrc — provider binary /Users/alex/devbin/terraform-provider-uem (built less than a day ago)
```

Terraform's own banner, on every `plan`, looks like this:

```text
│ Warning: Provider development overrides are in effect
│
│ The following provider development overrides are set in the CLI configuration:
│  - omnissa/uem in <dev binary directory>
```

**`ws1-tf` starts Terraform with a cleaned environment.** Every `terraform` it runs gets your environment
minus the variables Terraform's Go library does not let a caller inherit: `TF_CLI_ARGS` and
`TF_CLI_ARGS_<command>`, `TF_VAR_*`, `TF_INPUT`, `TF_IN_AUTOMATION`, `TF_LOG`, `TF_LOG_CORE`, `TF_LOG_PATH`,
`TF_LOG_PROVIDER`, `TF_WORKSPACE`, `TF_REATTACH_PROVIDERS` and a few more. A `TF_CLI_ARGS_plan="-refresh=false"`
or `-target=...` in your shell therefore cannot change the plan whose result `onboard` reports, and
`TF_CLI_ARGS_init=-upgrade` cannot change an init. `TF_CLI_CONFIG_FILE`, `TF_DATA_DIR`,
`TF_PLUGIN_CACHE_DIR`, `PATH`, `HOME` and the `UEM_*` credentials are passed through (a relative
`TF_CLI_CONFIG_FILE` is passed as the absolute path `ws1-tf` itself reads, since `terraform` runs in the work directory). Terraform's own text
that reaches an `onboard` error (its stderr, and the diagnostics of the enumeration step) is cut to a bounded
size, with control and format characters replaced by `?` (newline and tab are kept), and at most 25 error and
25 warning diagnostics are listed, the rest counted (`... and N more error diagnostics not shown`, and the same
for warnings).

Before enumerating anything, `onboard` also checks that the resolved provider has every data source
and attribute this CLI reads (from `terraform providers schema -json`). A provider older than the CLI
stops the run at once with `the installed provider is older than this CLI — rebuild or reinstall it`,
instead of failing partway through.

The lists the provider returns are checked the same way before anything is imported. A `null` output
is an error (`the output is null, not a list (provider older or newer than the CLI?)`), never "nothing to
onboard". So is an item with a `null` or missing `profile_id`, `platform`, `uuid`, `script_uuid`,
`smart_group_id`, `smart_group_uuid` or `assignment_count` (whichever the type carries), or with a blank
uuid or platform or an id that isn't positive. The message names the data source, the item's index and
the attribute, never a value. If an enumerated item still reaches the import with no usable id or uuid, `onboard` stops with
`enumerated <type> #<n> has no usable id/uuid in the provider output; refusing to import it`.

**Where Section 4's checks fit in:** every `onboard` run hits both triggers, and neither fires visibly
below — for two different reasons. Section 4.2's bootstrap trigger doesn't fire because this demo
pre-configures `dev_overrides` before `onboard`
ever runs — `mode` is never `ModeRegistry` here, so there's nothing to bootstrap. Section 4.3's
version-mismatch trigger doesn't fire because this demo's UEM version (`26.2.1000.0`) resolves to
the one matrix entry already pinned in `versions.tf`, and a matched target is never a mismatch
against itself. Against a genuinely fresh machine (nothing installed), you'd see Section 4.2's
`Provider omnissa/uem is not installed locally...` prompt right after the org-group list (as shown
live in Section 4.2). Against a tenant whose UEM version *is* in the matrix and doesn't match the pin
already in `versions.tf`, you'd see Section 4.3's `Provider does not match this tenant. Fix it?` prompt
right after the org-group list: the version confirmation and the org-group pick both come first.

**`--path` can name the repo root or a tenant directory.** With one tenant under the repo root,
`--path <repo root>` is enough (every example here does that). With several, the command stops with an
error that lists the tenant directories:

```text
multiple tenants found under <dir>: <full tenant paths>; run again with --path <tenant-dir> to target one directly, e.g. --path <the first path>
```

Pass the tenant directory (for example `--path /Users/alex/uem-terraform/prod`) to choose one.

**Interrupting an import.** Once every prompt is answered and the import batch is running, the first
Ctrl-C does not kill `ws1-tf` on the spot. It prints `Interrupt received: ws1-tf will not exit mid-batch. If
terraform stops, this run's imports are rolled back; otherwise the batch completes. Press Ctrl-C again to force
quit.` Whether the batch stops is up to `terraform`: the terminal's Ctrl-C is delivered to it as well. If it
stops, the batch fails and `ws1-tf` removes this run's imports from state (and the temporary stub file and the
script files this run wrote) before exiting with `onboard interrupted: … (imports from this run were rolled
back where possible; re-run onboard to retry)`. If `terraform` does not stop (for example because the signal
reached only `ws1-tf`), the batch completes, the run carries on to its normal end, and `ws1-tf` prints
`note: an interrupt arrived during the import batch, but the batch completed; its imports were not rolled
back.` A second Ctrl-C force-quits; if that happens mid-batch, run `terraform plan` in the work directory
before the next `onboard` — an object with a state entry but no config block would show up as a destroy.

**What a failed run cleans up, and what it leaves.** The import batch is all-or-nothing: when any import fails,
`ws1-tf` removes the imports this run made from state. Two failures need an extra word. An import that reads back
**no attributes** from Terraform state (the provider returned an empty object) fails the batch with `imported
resource <address> has no attributes in state after import` and is rolled back, instead of writing an empty block
and reporting success. And when the generated resource blocks cannot be written to the `.tf` file after the
objects were generated, `ws1-tf` removes the files this run created for them before it rolls the imports back: the
script and sensor body files, `secrets.tf` (when this run created it) and the `secrets.json` keys written for the
objects (a `secrets.json` that did not exist before is removed; one that did is put back to its previous
contents). A body file or `secrets.tf` that already existed is never removed. If a removal itself fails, the
error says so. One thing is not undone: the app installers, icons and pkginfo files that the provider downloads
into `uem-artifacts/<tenant>/app-binaries/` while importing a `mac_application` stay on disk after a rollback or
a skipped import. They are outside git (the `/uem-artifacts/` rule).

**`onboard` reads its own org-group scope independently of `init`.** Even though Section 5.1 initialized
this repo scoped to **Acme** (id `12346`), every `onboard` run below still shows the full,
interactive org-group picker (unless `--org-group` is passed) and can target **either** org group —
confirmed live: the transcripts below deliberately pick **Global** (id `12345`, a different org
group than `init` used) because that's where this tenant's live fixture data actually lives.

**Omitting `--type` on a real terminal shows a numbered type menu instead of erroring —
and now lists all 10 enumerable types, not 2.** `ws1-tf onboard` with no `--type` and a real
interactive terminal prints every enumerable type (sourced from `typemap.KnownTypes()`, so the list
can never drift from what `--type` itself accepts) and prompts for a number. The type menu is a plain
numbered list with no header; the org-group and item pickers that follow it are paged (see
[Paging, search, and range selection](#paging-search-and-range-selection-in-the-pickers)):

```text
$ ws1-tf onboard --path /Users/alex/uem-terraform
  1. Profiles
  2. macOS Applications
  3. Application Assignments
  4. macOS Scripts
  5. Script Assignments
  6. macOS Sensors
  7. Sensor Assignments
  8. Update Deployments
  9. Purchased Application Assignments
  10. Smart Groups
Select a type to onboard (number): 1
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 1
2 profiles skipped: owned by child org group "Sales" (id 12350). Add --include-inherited to onboard them.
Select profiles to onboard — Page 1/1 · 3 items
  1. Custom Profile (AppleOsX, profile_id 12347)
  2. Restrictions Profile (AppleOsX, profile_id 12348)
  3. windows _device_profile (Windows 10, profile_id 12349)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5] (←/→ page, / search): all
Importing profiles: 0/3…
Importing profiles: 3/3 done.
Onboarded 3 profiles into /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/profiles.tf. Review required before commit/apply; no apply was run.
```

The `Importing profiles: 0/3…` / `3/3 done.` pair is the import pass's progress report. On a long
run it prints a line every 25 objects or every 30 seconds, whichever comes first, so a large import
never looks hung — a real 487-profile `--include-inherited` run printed `Importing profiles:
0/487…`, `25/487…`, `50/487…` and so on, plus an in-between line such as `246/487…` whenever 30
seconds passed first, and ended with `Importing profiles: 487/487 done.` `mac_application` prints
one `Importing <name> (<size>)` line per app instead (see its section below).

`--type <t>` keeps working unchanged, interactive or not. On a **non**-interactive/piped session
with no `--type`, the CLI does NOT show this menu or hang waiting for input — it returns the same
clear error it always has: `specify --type <t> (whole-environment onboarding lands in a later
increment)`.

#### Dependencies: how `onboard` handles referenced org groups and smart groups

Plenty of the objects you onboard don't stand alone — a profile or an assignment points at the org
group it belongs to, or at a smart group it targets. `onboard` never leaves those as bare ids sitting
inside the resource block. Every org-group reference is resolved and written into one shared
`dependencies.tf` file, generated in the same working directory as the resource file(s) you're
onboarding, and reused (not duplicated) across later runs. There are exactly two outcomes for an org
group, decided automatically — nothing for you to choose:

1. **Your credential can read it → a real, live lookup.** `onboard` writes a
   `data "uem_organization_groups"` block into `dependencies.tf`, and the referencing attribute
   points at that block's result instead of the bare id. Every later `terraform plan`/`apply`
   re-reads the live object through that lookup, so it always reflects the object's current, real
   value.
2. **Your credential can see it but can't read it directly → a documented, fixed value, never a live
   call.** This is an ordinary, expected shape in UEM: a child org group inherits *visibility* of
   things above it in the hierarchy, but UEM refuses direct reads of that org group from this
   account — so `onboard` uses the id/uuid the account can see instead (from the child's own
   `/children` self-entry) rather than ever attempting the refused direct read. `onboard` checks
   read access before ever writing a lookup, so it never generates a `data` block that would fail
   later at `terraform plan` time. The check counts an answer as readable only when it actually contains
   the object that was asked for (an org group's own entry in its children listing; for a smart group, its
   id, uuid and owning org group id, and the id must be the requested one). A response that lacks them
   is a hard error that stops the run, not a "can't read" case. When UEM refuses the read, it writes a plain
   `locals` entry instead —
   the object's known id/uuid, captured once — and points the reference at that. **No warning is
   printed for this case** — it's routine, not an error condition.
   Because the reference is a fixed local value, not a live call, `terraform plan` never makes an API
   call for it at all, so this reference can never fail a plan no matter what your access looks like
   later. The comment above the entry reads `# fixed value: this org group is outside this account's
   access`.

One related case does print a warning: an org group referenced only by its uuid that this account
can't see at all (it's not in the selected org group's tree). `onboard` can't look up its numeric
id, so the uuid stays in the resource as a fixed value, and the run says so once per org group:

```text
warning: the generated config refers to an org group this account can't see (00000000-0000-0000-0000-000000000003); its uuid was left as a fixed value
```

**A `locals` entry is read-only and is never turned into a managed resource automatically —
by design.** It's a fixed value Terraform will never apply changes to. If your account is later
granted direct access to that same object, the next `onboard` run notices, switches that one
reference over to a real `data` lookup, and prints a `note:` line saying so — but the old `locals`
entry itself is left in the file, unused, never deleted for you.

**A smart-group reference has THREE outcomes, not two, now that `uem_smart_group` is a managed
resource:**

1. **Owned by the org group you're onboarding, and readable → a managed `uem_smart_group` resource.**
   `onboard` imports it (by its numeric id, as `uem_smart_group.sg_<id>`) into the SAME working
   directory, alongside whatever you're
   onboarding, and rewrites the reference to `uem_smart_group.<name>.id` or `.uuid` (whichever the
   attribute needs) instead of a `data` lookup. Its `resource "uem_smart_group"` block is appended to
   the **referencing type's** file — a smart group a profile targets lands in `profiles.tf`, below the
   profiles — not to `smart_groups.tf`, which only holds smart groups you onboard with `--type
   smart_group`. The block is the same either way. This is exactly what `ws1-tf onboard --type
   smart_group` produces when you onboard a smart group directly — and the two paths share one
   result: the SAME physical smart group is never imported twice, whether it's referenced more than
   once in this run, already sits in this root's state from an earlier run, or was also onboarded
   directly via `--type smart_group` (in the same run or an earlier one). One clean `terraform plan`
   then manages both the smart group's own definition and everything that references it.
2. **Readable, but owned by a DIFFERENT org group → a real, live `data "uem_smart_groups"` lookup**,
   exactly like case 1 above for org groups. A lookup is a read, so it's always safe regardless of
   ownership — `onboard` never imports a resource it would then be managing from the wrong org
   group.
3. **Can't be resolved to a readable group → a documented, fixed `locals` value**, like org-group
   case 2 above: no warning, no live call. There are two different reasons this happens, and the
   comment above each `locals` entry says which one applies:
   - **The reference is a numeric id and UEM refused the read** (the group is outside this account's
     access). The comment reads `# fixed value: this smart group is outside this account's access`.
     It's promoted to a live lookup (or to a
     managed resource if it turns out to be owned) automatically on a later run once access is
     granted, with a `note:` line.
   - **The reference is a UUID that UEM's smart group search didn't return.** UEM can only resolve a
     smart group UUID through its search, and the search leaves out an organization group's own
     smart group — the one named after the org group, which UEM uses when something is assigned to
     the whole org group (live-confirmed; UEM has no read-by-UUID endpoint). This is not an access
     problem: the same group is often readable by its numeric id. The comment reads `# fixed value:
     UEM's smart group search doesn't return this group`. Assignment payloads
     (`application_assignment`, `script_assignment`, `sensor_assignment`, `update_deployment`, and VPP
     license groups) carry only the UUID, so these references stay a fixed value; it still plans
     clean.

A real `dependencies.tf` from a run that hit the second case — one live org-group lookup, and the
org group's own smart group kept as a fixed value:

```text
# ws1-tf dependency lookup — plan fails unless exactly one match
data "uem_organization_groups" "og_12346" {
  id = "12346"

  lifecycle {
    postcondition {
      condition     = length(self.organization_groups) == 1
      error_message = "expected exactly 1 organization group for id \"12346\", got a different count"
    }
  }
}

locals {
  # fixed value: UEM's smart group search doesn't return this group
  sg_88888888_8888_8888_8888_888888888888 = {
    smart_group_uuid = "88888888-8888-8888-8888-888888888888"
  }
}
```

Nothing here changes how an org-group reference resolves — organization groups still have no
resource, only the lookup/locals split above; owning-org-group management would mean onboard could
create or delete org groups, which it never does.

A short real example, from a live run against a second lab tenant: onboarding all 7 `profile` records
visible from the writable org group `1001` (with `--include-inherited`). Two references resolved as
real, live lookups — org group `1002` and smart group `2001`, both directly readable from `1001`
— used by 2 of the 7 profiles. The other 5 profiles referenced an org group (`1003`) and a smart group
(`2002`) that were visible but not directly readable, so both resolved to documented `locals` entries
instead. `onboard` printed **0 warnings** the whole run — every one of the 7 references resolved
cleanly, one way or the other — and `dependencies.tf` ended up with a `data
"uem_organization_groups" "og_1002"` / `data "uem_smart_groups" "sg_2001"` pair sitting alongside
a `locals { og_1003 = {...}, sg_2002 = {...} }` block. `terraform plan -detailed-exitcode` came back
`0` ("No changes") for all 7 profiles — the live-lookup path and the locals-fallback path both
planned clean, in the same run, against the same tenant.

(For the exact generated HCL and the one case that DOES still print a warning — an assignment's
parent skipped by owned-only filtering, unrelated to org-group/smart-group access — see "Two
warnings you will see in real use" a few subsections below.)

#### `profile`

```text
$ ws1-tf onboard --type profile --path /Users/alex/uem-terraform
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 1
2 profiles skipped: owned by child org group "Sales" (id 12350). Add --include-inherited to onboard them.
Select profiles to onboard — Page 1/1 · 3 items
  1. Custom Profile (AppleOsX, profile_id 12347)
  2. Restrictions Profile (AppleOsX, profile_id 12348)
  3. windows _device_profile (Windows 10, profile_id 12349)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5] (←/→ page, / search): all
Importing profiles: 0/3…
Importing profiles: 3/3 done.
Onboarded 3 profiles into /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/profiles.tf. Review required before commit/apply; no apply was run.
```

**No per-reference warnings here — `org_group_id` and `assigned_smart_groups` both resolve
automatically.** Rather than the old regex-on-generated-text heuristic (removed entirely, along with
the file it lived in), `onboard` resolves references with a static reference table (which attribute,
on which type, points at which other UEM object kind) walked structurally against the live tenant.
Each distinct org-group id and smart-group id/uuid the import set touches gets one `data` block in a
new shared `dependencies.tf` file in the same working directory, and the referencing attribute in the
resource block is rewritten from the raw literal to an expression reading that data block's result —
live-confirmed here for both `org_group_id` (all three profiles point at the same org group, so one
`og_12345` data block covers all three) and `assigned_smart_groups` (two distinct smart groups across
the three profiles, so two `sg_...` data blocks). A reference this mechanism can't resolve still
falls back to a literal-plus-warning — see "Two warnings you will see in real use" further down for
the case that still happens.

The real, complete generated Terraform file:

```text
$ cat /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/profiles.tf
# Custom Profile — UEM id 12347, org group "Global" (id 12345)
# captured from UEM 26.2.1000.0 on 2026-09-25
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_profile" "Custom_Profile" {
  assigned_smart_groups = [data.uem_smart_groups.sg_90001.smart_groups[0].smart_group_id]
  assignment_type = "Auto"
  custom_settings_list = [
    {
      custom_settings = "<dict><key>PayloadType</key><string>com.apple.example.test</string><key>PayloadIdentifier</key><string>com.example.sample.custom</string><key>PayloadDisplayName</key><string>Sample Custom</string><key>PayloadVersion</key><integer>1</integer><key>PayloadUUID</key><string>00000000-0000-0000-0000-000000000001</string><key>MyCustomKey</key><string>MyCustomValue</string></dict>"
    },
  ]
  description = "Minimal plist custom settings"
  is_active = true
  name = "Custom Profile"
  org_group_id = data.uem_organization_groups.og_12345.organization_groups[0].id
  platform = "AppleOsX"
  profile_context = "Device"
  profile_scope = "Production"
}

# Restrictions Profile — UEM id 12348, org group "Global" (id 12345)
# captured from UEM 26.2.1000.0 on 2026-09-25
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_profile" "Restrictions_Profile" {
  assigned_smart_groups = [data.uem_smart_groups.sg_90001.smart_groups[0].smart_group_id]
  assignment_type = "Auto"
  description = "Minimal sharing + spotlight restrictions"
  is_active = true
  name = "Restrictions Profile"
  org_group_id = data.uem_organization_groups.og_12345.organization_groups[0].id
  platform = "AppleOsX"
  profile_context = "Device"
  profile_scope = "Production"
  restrictions = {
    applications = {
      app_store = {
        allow_app_store_app_adoption = true
        require_admin_password_to_install_or_update_app = false
        restrict_app_store_to_software_updates_only = false
      }
      apple_music = {
        allow_music_service = false
      }
      camera = {
        allow_use_of_built_in_camera = false
      }
      game_centre = {
        allow_adding_game_center_friends = false
        allow_game_center_modification = false
        allow_multiplayer_gaming = false
        allow_use_of_game_center = false
      }
      restrict_which_applications_are_allowed_to_launch = false
      safari = {
        allow_deprecated_web_kit_tls = false
        allow_safari_auto_fill = false
      }
    }
    functionality = {
      spotlight = {
        allow_spotlight_suggestions = false
      }
    }
    sharing = {
      air_drop = false
      restrict_which_sharing_services_are_enabled = false
    }
  }
}

# windows _device_profile — UEM id 12349, org group "Global" (id 12345)
# captured from UEM 26.2.1000.0 on 2026-09-25
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_profile" "windows__device_profile" {
  assigned_smart_groups = [data.uem_smart_groups.sg_90002.smart_groups[0].smart_group_id]
  assignment_type = "Auto"
  is_active = true
  name = "windows _device_profile"
  org_group_id = data.uem_organization_groups.og_12345.organization_groups[0].id
  platform = "Windows 10"
  profile_context = "Device"
  profile_scope = "Production"
}
```

Every generated resource block also gets that two-line header comment ahead of the existing
`# REVIEW REQUIRED` line: the object's console name, its UEM id (and uuid, when the type reports a
separate one) and its owning org group (id, plus name when the type reports that directly), then the
UEM version and local date this run captured it on. Any part the generator doesn't know — no
separate uuid, no owning-OG attribute at all for that type, no UEM version wired in — is left out
entirely rather than printed empty.

Its new companion file — one `data` block per distinct org-group/smart-group value the import set
referenced (3 references, 3 distinct values, so 3 blocks: one org group, two smart groups):

```text
$ cat /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/dependencies.tf
# ws1-tf dependency lookup — plan fails unless exactly one match
data "uem_organization_groups" "og_12345" {
  id = "12345"

  lifecycle {
    postcondition {
      condition     = length(self.organization_groups) == 1
      error_message = "expected exactly 1 organization group for id \"12345\", got a different count"
    }
  }
}

# ws1-tf dependency lookup — plan fails unless exactly one match
data "uem_smart_groups" "sg_90001" {
  smart_group_id = "90001"

  lifecycle {
    postcondition {
      condition     = length(self.smart_groups) == 1
      error_message = "expected exactly 1 smart group for smart_group_id \"90001\", got a different count"
    }
  }
}

# ws1-tf dependency lookup — plan fails unless exactly one match
data "uem_smart_groups" "sg_90002" {
  smart_group_id = "90002"

  lifecycle {
    postcondition {
      condition     = length(self.smart_groups) == 1
      error_message = "expected exactly 1 smart group for smart_group_id \"90002\", got a different count"
    }
  }
}
```

(`Restrictions_Profile`'s `restrictions` block above is trimmed to the fields this tenant's fixture
actually populates, for length — the real file has one entry per nested attribute the schema
defines, `false`/`null` for everything the fixture didn't set. `windows _device_profile`'s literal
space in its name, and the resulting `windows__device_profile` resource address — `ws1-tf` turns every
character outside letters, digits, `_` and `-` into `_`, and prefixes a name that would start with a digit
or `-`, so the address is a valid identifier — are real, not typos in this doc.) Note there's no
`id`/`uuid` line in any block — those are Computed-only attributes (Terraform derives them from the
imported state itself), so the generator correctly leaves them out of the HCL you'd actually commit.
A map key is written bare when it is a valid HCL identifier and quoted otherwise; the words HCL reads as a
literal or an expression keyword (`true`, `false`, `null`, `for`, `if`, `in`) are always quoted, so a key with
one of those names stays valid HCL. Text that UEM supplies and that lands in a `#` comment (the record header,
the `UEM record` block, the path in the `secrets.tf` header) is put on one line first: control characters become
a space and format characters are removed, so a name cannot end its comment and turn the rest into live HCL or
hide text with a bidirectional override.

#### `mac_application`

```text
$ ws1-tf onboard --type mac_application --path /Users/alex/uem-terraform
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 1
2 mac applications skipped: owned by parent org group id 12344. Onboard them with --org-group 12344.
Select mac applications to onboard — Page 1/1 · 2 items
  1. MacOSDMGTestApp (uuid 00000000-0000-0000-0000-000000000005)
  2. MacOSPkgStyleTestApp (uuid 00000000-0000-0000-0000-000000000006)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5] (←/→ page, / search): all
Importing MacOSDMGTestApp (500 KB)
Importing MacOSPkgStyleTestApp (195.5 MB) — large installers can take several minutes; it only fails if the download stalls
Onboarded 2 mac applications into /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/mac_applications.tf. Review required before commit/apply; no apply was run.

Next: set -a; source /Users/alex/uem-terraform/prod/.env; set +a; terraform -chdir=/Users/alex/uem-terraform/prod/.ws1tf-onboard-work plan
action log: /Users/alex/uem-terraform/prod/.ws1tf/logs/20260925T215555Z-onboard.jsonl
```

Before each app downloads, `onboard` prints its name and size: in KB below 1 MB, otherwise in MB
with one decimal (`size unknown` when UEM doesn't report one). The download has no overall time
limit — a large installer can legitimately take minutes. It only fails if no bytes arrive for 60
seconds (`UEM_HTTP_STALL_TIMEOUT`), with `download of <name> stalled after N of M MB; check the network
and re-run onboard`. With Terraform logging on (`TF_LOG=INFO`), the provider also logs progress about
every 10%, but that only applies to a Terraform command you run yourself: `ws1-tf` starts every `terraform`
with `TF_LOG` and the other `TF_LOG*` variables blanked, so setting `TF_LOG` before `onboard` shows nothing.
When `onboard` has imported something it ends with the exact `Next:` plan command for this
repo; every run that recorded anything, including a failed one (where it comes before the `error:` line), also
prints the path of its action log, `action log: <path>`, on stderr as it exits (Section 5.8).

`mac_application` (with `application_assignment`) is the type where `--include-inherited` never applies (see
[Owned-only by default](#owned-only-by-default---include-inherited-and---org-group) below) — an
app owned by another org group is always skipped with one summary line per owning org group (the
`2 mac applications skipped: …` line above), never offered on the picker and never an error.

Only apps whose `type` is `Internal` are offered, for `mac_application` and for `application_assignment` alike.
An app whose type the provider reports as anything else is skipped with `N purchased/VPP apps skipped: not
importable as uem_mac_application; use --type purchased_application_assignment for their assignments.`. An app
for which the provider reports **no type at all** (absent or `null`) is
not assumed to be purchased: it is skipped with its own line, `N apps skipped: uem_mac_applications didn't report
their type (is the provider older than the CLI?).`

Each app is imported with the composite ID `<uuid>,<org_group_id>`: `onboard` only imports apps the
selected org group owns, so it knows the owner's numeric id, and UEM's app read never returns it. That
puts `org_group_id` in the generated config, resolved through the same `og_12345` org-group lookup in
`dependencies.tf` that `profile` uses — the provider needs it to create the app in a new environment.
Generated file:

```text
$ cat /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/mac_applications.tf
# MacOSDMGTestApp — UEM id 12351, uuid 00000000-0000-0000-0000-000000000005, org group "Global" (id 12345)
# captured from UEM 26.2.1000.0 on 2026-09-28
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_mac_application" "MacOSDMGTestApp" {
  app_version = "1.0.0.0"
  dmg_file_path = "${path.root}/../../uem-artifacts/prod/app-binaries/00000000-0000-0000-0000-000000000005/app.dmg"
  icon_file_path = "${path.root}/../../uem-artifacts/prod/app-binaries/00000000-0000-0000-0000-000000000005/icon.png"
  org_group_id = data.uem_organization_groups.og_12345.organization_groups[0].id
  plist_file_path = "${path.root}/../../uem-artifacts/prod/app-binaries/00000000-0000-0000-0000-000000000005/app.plist"
}
# UEM record (read-only: reported by UEM, not settable through the API; to change it, change the
# application files above, which replaces the application):
#   actual_file_version = "1.0"
#   app_id = "com.example.MacOSDMGTestApp"
#   app_provisioning_profile_uuid = "00000000-0000-0000-0000-000000000000"
#   app_size_in_kb = 17
#   application_file_hash = "5DD97C1558E1AF38BF041C0EADC48E1115BCFDA43AB44834CAEABA2F7A98B0CE"
#   application_name = "MacOSDMGTestApp"
#   application_url = ""
#   assume_management_of_user_installed_app = "No"
#   build_version = ""
#   category_list = []
#   change_log = ""
#   comments = "Synthetic test app. Not a real product."
#   display_name = ""
#   large_icon_blob_guid = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
#   launch_command = ""
#   launch_type = ""
#   mac_os_software_deployment_summary = {"is_managed":"True","pkginfo":"<2296-byte plist; see plist_file_path>"}
#   managed_by = "12345"
#   managed_by_uuid = "00000000-0000-0000-0000-000000000001"
#   medium_icon_blob_guid = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
#   minimum_operating_system = ""
#   platform = "AppleOsX"
#   rating = 0
#   sdk = "Disabled"
#   sdk_profile_id = null
#   sdk_profile_uuid = ""
#   small_icon_blob_guid = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
#   status = "Active"
#   supported_models = [{"id":14,"name":"MacBook Pro","uuid":""},{"id":113,"name":"Mac Studio","uuid":""},{"id":15,"name":"MacBook Air","uuid":""},{"id":16,"name":"Mac Mini","uuid":""},{"id":35,"name":"MacBook","uuid":""},{"id":30,"name":"iMac","uuid":""},{"id":31,"name":"Mac Pro","uuid":""}]
#   supported_models_name = ["MacBook Pro","Mac Studio","MacBook Air","Mac Mini","MacBook","iMac","Mac Pro"]

# MacOSPkgStyleTestApp — UEM id 12352, uuid 00000000-0000-0000-0000-000000000006, org group "Global" (id 12345)
# captured from UEM 26.2.1000.0 on 2026-09-28
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_mac_application" "MacOSPkgStyleTestApp" {
  app_version = "1.0.0.0"
  dmg_file_path = "${path.root}/../../uem-artifacts/prod/app-binaries/00000000-0000-0000-0000-000000000006/app.pkg"
  # UEM's icon isn't a recognised image; re-uploading it may be rejected.
  icon_file_path = "${path.root}/../../uem-artifacts/prod/app-binaries/00000000-0000-0000-0000-000000000006/icon"
  org_group_id = data.uem_organization_groups.og_12345.organization_groups[0].id
  plist_file_path = "${path.root}/../../uem-artifacts/prod/app-binaries/00000000-0000-0000-0000-000000000006/app.plist"
}
# UEM record (read-only: reported by UEM, not settable through the API; to change it, change the
# application files above, which replaces the application):
#   actual_file_version = "1.0"
#   app_id = "com.example.MacOSPkgStyleTestApp"
#   app_provisioning_profile_uuid = "00000000-0000-0000-0000-000000000000"
#   app_size_in_kb = 1
```

(The second app's `UEM record` block is cut after four lines here; the real file lists the same
fields as the first app's.)

Each app gets up to three files, downloaded next to each other under its uuid: the installer
(`app.dmg` or `app.pkg`, always set through `dmg_file_path`), the icon (`icon_file_path`) and the
Munki pkginfo (`plist_file_path`). The icon file is named for the image type the provider recognises
in it (`icon.png`, `icon.jpg` or `icon.gif`). When UEM's stored icon isn't one of those, it's saved
as a bare `icon` with no extension and the line above `icon_file_path` says so: the config still
plans clean, but UEM's upload may reject a file without an image extension if the icon is ever sent
again (for example when a change replaces the app). Point `icon_file_path` at a real image before
making such a change.

The `# UEM record` comment block under each resource lists what UEM reported about the app that the
provider can't set: the bundle id, file hash, size, owning org group (`managed_by`), icon blob ids,
supported models, and so on. It's there so you can review the whole object in one place. It's a
comment, so Terraform ignores it, and none of it becomes drift. UEM derives these values from the
installer, so the only way to change them is to change the files above, which replaces the app.

Each header names the owning org group, from the `org_group_id` the import recorded. In the
unexpected case that `onboard` doesn't know an app's owner id, it imports that app by its bare uuid,
leaves `org_group_id` out and says so: `warning: mac application "<name>" (uuid <uuid>): its owning
org group's id isn't known, so the generated config has no org_group_id; add it before creating this
app in another environment`.

**Apps onboarded by an earlier `ws1-tf` build have no `org_group_id`.** Those builds imported each
app by its bare uuid, so the resource can't create the app in a new environment. Re-running
`onboard` doesn't change them — they're already in the repo, and it skips them. Add the owning org
group's numeric id to each resource yourself (the `managed_by` value in its `UEM record` block):

```hcl
resource "uem_mac_application" "MacOSDMGTestApp" {
  app_version = "1.0.0.0"
  org_group_id = 12345
  # ...
}
```

The next plan shows an in-place update that only fills `org_group_id` in state — `+ org_group_id =
12345` on each app you edited, and `0 to destroy` — and the apply makes no API call. The provider
replaces an app only when an `org_group_id` it already knows changes; filling in one a bare-uuid
import left empty is an in-place, state-only update.

The downloaded `.dmg`/`.pkg` files themselves live on disk at
`/Users/alex/uem-terraform/uem-artifacts/prod/app-binaries/<uuid>/app.dmg` (repo-relative — the
CLI's generated `provider.tf` for this run sets `app_binary_storage_path` accordingly, see
Section 5's `provider.tf` listing below), never under a hidden `~/.uem-provider` home-directory
folder.

**Upgrading from `~/.uem-provider`:** older repos may still have `dmg_file_path`/`plist_file_path`
pointing at the old `~/.uem-provider/app-binaries/<uuid>/...` default. Move the `<uuid>/`
directories under `<repo>/uem-artifacts/<tenant>/app-binaries/` (or just re-run `onboard`, which
downloads them fresh at the new location) and update the path in your `.tf`. Because identity is
the file's content SHA-256, not its path, this shows up as an in-place path update on the next
plan — never a re-upload or a destroy/recreate. An explicitly configured `app_binary_storage_path`
or `UEM_APP_BINARY_DIR` is unaffected either way. See the `uem_mac_application` resource docs'
"Upgrading from ~/.uem-provider" section for the full details.

Whichever way you move them, the files now sit under `<repo>/uem-artifacts/`, which must be ignored by
git: `onboard` adds `/uem-artifacts/` to the repo-root `.gitignore` if the rule is missing (a repo
created by an earlier build may not have it), and if you manage `.gitignore` yourself, make sure it
contains `/uem-artifacts/`.

#### `application_assignment`

Reuses `mac_application`'s own enumerate/select calls (an assignment is keyed 1:1 by the same app
UUID) — same org-group picker, same 2 apps:

```text
$ ws1-tf onboard --type application_assignment --path /Users/alex/uem-terraform
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 1
2 application assignments skipped: their mac applications are owned by parent org group id 12344. Onboard them with --org-group 12344.
Select mac applications whose assignments to onboard — Page 1/1 · 2 items
  1. MacOSDMGTestApp (uuid 00000000-0000-0000-0000-000000000005)
  2. MacOSPkgStyleTestApp (uuid 00000000-0000-0000-0000-000000000006)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5] (←/→ page, / search): all
Importing application assignments: 0/2…
Importing application assignments: 2/2 done.
Onboarded 2 application assignments into /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/application_assignments.tf. Review required before commit/apply; no apply was run.
```

Since `application_assignment` reuses `mac_application`'s own enumerate/select calls, the same
parent-owned-app skip applies here too (a parent-owned app is never offered for auto-onboarding as an
assignment's parent either).

No warnings here either. Two different resolution paths fire in this one run: `application_uuid`
(each assignment's parent app) resolves directly to the parent resource's address, because both
`MacOSDMGTestApp` and `MacOSPkgStyleTestApp` are already in this working directory's Terraform state
— they were onboarded as `mac_application` in the subsection just above. `assignments[].distribution.smart_groups`
resolves the same way `assigned_smart_groups` did for `profile`: a `data "uem_smart_groups"` lookup in
`dependencies.tf`. Generated file:

```text
$ cat /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/application_assignments.tf
# Assignments of MacOSDMGTestApp — application uuid 00000000-0000-0000-0000-000000000005
# captured from UEM 26.2.1000.0 on 2026-09-25
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_application_assignment" "MacOSDMGTestApp" {
  application_uuid = uem_mac_application.MacOSDMGTestApp.uuid
  assignments = [
    {
      distribution = {
        effective_date = "2026-09-22T20:00:00Z"
        name = "smartgroup-1"
        smart_groups = [data.uem_smart_groups.sg_00000000_0000_0000_0000_000000000004.smart_groups[0].smart_group_uuid]
      }
      priority = 0
      restriction = {
        remove_on_unenroll = true
      }
    },
  ]
}

# Assignments of MacOSPkgStyleTestApp — application uuid 00000000-0000-0000-0000-000000000006
# captured from UEM 26.2.1000.0 on 2026-09-25
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_application_assignment" "MacOSPkgStyleTestApp" {
  application_uuid = uem_mac_application.MacOSPkgStyleTestApp.uuid
  assignments = [
    {
      distribution = {
        effective_date = "2026-09-22T20:00:00Z"
        name = "smartgroup-1"
        smart_groups = [data.uem_smart_groups.sg_00000000_0000_0000_0000_000000000004.smart_groups[0].smart_group_uuid]
      }
      priority = 0
      restriction = {
        remove_on_unenroll = true
      }
    },
  ]
}
```

`application_assignment` has no console-name attribute and no owning-OG attribute of its own (an
assignment rule is keyed by its parent application's uuid), so its header names the parent app and
its uuid instead: `# Assignments of <app> — application uuid <uuid>`.

An app with no assignment rules in UEM is still onboarded, as an explicit empty list, with a comment
saying so — managing it means a later `apply` removes any rule someone adds in the console:

```text
# Assignments of MacOSDMGTestApp — application uuid 00000000-0000-0000-0000-000000000005
# captured from UEM 26.2.1000.0 on 2026-09-27
# no assignments in UEM
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_application_assignment" "MacOSDMGTestApp" {
  application_uuid = uem_mac_application.MacOSDMGTestApp.uuid
  assignments = []
}
```

`dependencies.tf` in that same working directory picks up one new block for this run's single
distinct smart-group value (the `og_12345`/`sg_90001`/`sg_90002` blocks from the `profile` run stay
untouched — this run doesn't reference any of those):

```text
$ cat /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/dependencies.tf
# ws1-tf dependency lookup — plan fails unless exactly one match
data "uem_smart_groups" "sg_00000000_0000_0000_0000_000000000004" {
  smart_group_uuid = "00000000-0000-0000-0000-000000000004"

  lifecycle {
    postcondition {
      condition     = length(self.smart_groups) == 1
      error_message = "expected exactly 1 smart group for smart_group_uuid \"00000000-0000-0000-0000-000000000004\", got a different count"
    }
  }
}
```

**If the parent app hadn't already been onboarded in this same working directory**, `onboard` would
auto-import it first — through the same owned-only path as onboarding `mac_application` directly —
and print a line like `Auto-onboarded 2 mac applications (parents of the selected assignments) into
/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/mac_applications.tf.` before
writing `application_assignments.tf`, and only then rewrite `application_uuid` to the newly-imported
resource's address. That codepath doesn't fire in this doc's captures because every assignment
example here reuses a parent type onboarded in an earlier subsection of this same transcript.

#### `mac_script`

```text
$ ws1-tf onboard --type mac_script --path /Users/alex/uem-terraform
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 1
2 mac scripts skipped: owned by child org group "Sales" (id 12350). Add --include-inherited to onboard them.
Select mac scripts to onboard — Page 1/1 · 2 items
  1. script-1 (APPLE_OSX, uuid 00000000-0000-0000-0000-000000000007)
  2. script-2 (APPLE_OSX, uuid 00000000-0000-0000-0000-000000000008)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5] (←/→ page, / search): all
Importing mac scripts: 0/2…
Importing mac scripts: 2/2 done.
Onboarded 2 mac scripts into /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/mac_scripts.tf. Review required before commit/apply; no apply was run.
```

`uem_mac_script` manages macOS scripts only. UEM's script list includes every platform's scripts,
so `mac_script` and `script_assignment` offer only the macOS ones and skip the rest with one line,
for example `12 non-macOS scripts skipped: uem_mac_script manages macOS scripts only.` (none here:
this org group's scripts are all macOS).

No warnings here — `organization_group_uuid` resolves the same way `org_group_id` did for `profile`,
just read back as `.uuid` instead of `.id` since the attribute is the uuid form (`uem_organization_groups`
only filters by numeric id, so the CLI resolves the uuid `00000000-0000-0000-0000-000000000001` to its
numeric id `12345` internally and keys the lookup the same way). Generated file:

```text
$ cat /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/mac_scripts.tf
# script-1 — uuid 00000000-0000-0000-0000-000000000007, org group "Global" (id 12345)
# captured from UEM 26.2.1000.0 on 2026-09-25
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_mac_script" "script-1" {
  allowed_in_catalog = false
  description = ""
  execution_context = "SYSTEM"
  name = "script-1"
  organization_group_uuid = data.uem_organization_groups.og_12345.organization_groups[0].uuid
  platform = "APPLE_OSX"
  platform_architecture = "UNKNOWN"
  script_data = filebase64("${path.module}/../scripts/script-1.sh")
  script_type = "BASH"
  timeout = 30
  user_interaction = false
}

# script-2 — uuid 00000000-0000-0000-0000-000000000008, org group "Global" (id 12345)
# captured from UEM 26.2.1000.0 on 2026-09-25
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_mac_script" "script-2" {
  allowed_in_catalog = true
  catalog_display = {
    action_type = "Run_Script"
    catalog_icon_url = "publicblob/dddddddd-dddd-dddd-dddd-dddddddddddd/BlobHandler.pblob"
    categories = ["1"]
    display_desc = "test"
    display_name = "new script"
    post_action_text = "Rerun"
    pre_action_text = "Run"
  }
  description = ""
  execution_context = "SYSTEM"
  name = "script-2"
  organization_group_uuid = data.uem_organization_groups.og_12345.organization_groups[0].uuid
  platform = "APPLE_OSX"
  platform_architecture = "UNKNOWN"
  script_data = filebase64("${path.module}/../scripts/script-2.py")
  script_type = "PYTHON"
  timeout = 30
  user_interaction = false
}
```

Every attribute UEM returned is written out, including empty ones like `description = ""` and
defaults like `platform_architecture = "UNKNOWN"`, so the plan compares against exactly what UEM
holds. A script offered in the Intelligent Hub catalog (`allowed_in_catalog = true`) also gets its
`catalog_display` block: the catalog name, description, button labels and icon.

`mac_script` has no separate numeric id — its `id` is the UUID — so its header shows the uuid alone,
where `mac_application` and `smart_group` show both a UEM id and a uuid.

**Script and sensor bodies are real files.** UEM stores a script body (`script_data`) and a sensor's
code (`code`) as base64. `onboard` decodes each one and writes the exact bytes to a file in the tenant
directory — `prod/scripts/<name>.<ext>` for scripts, `prod/sensors/<name>.<ext>` for sensors — and the
generated config reads it back with `filebase64(...)`, so you can review and edit scripts as ordinary
files in version control. The extension comes from `script_type` / `language`: `BASH` gives `.sh`,
`ZSH` `.zsh`, `PYTHON` `.py`, anything else `.txt`. Nothing about the bytes is changed (line endings,
a byte-order mark or a missing final newline are kept exactly), so `filebase64` of the file equals
UEM's value and the plan stays clean. Two different bodies with the same name get `-2`, `-3`, … and a
later run reuses a file that already holds the same body. If UEM's value isn't standard padded base64,
the value stays inline as a literal instead, since a file couldn't reproduce it exactly. These files
belong in your repository alongside the `.tf` files.

**Script and sensor bodies are committed as UEM returned them.** A body can hold a password or token, and
ws1-tf does not scan or redact it (`uem_mac_script.script_data` and `uem_mac_sensor.code` are written
verbatim so the plan stays clean). Each file is written readable by you only (mode `0600`), but the
repo-root `.gitignore` excludes `.env` files, Terraform state and working files, `.ws1tf/` and
`/uem-artifacts/`, not `scripts/` or `sensors/`, so `git add` picks those up. Read them before you commit.

No new `dependencies.tf` capture here: `og_12345` was already declared by the `profile` run earlier
in this same working directory, and `onboard` reuses that existing block (matched by its filter value)
instead of emitting a duplicate — the same idempotency `dependencies.tf` gets across separate
`onboard` runs applies within a single run that references a value another run already declared.

#### `script_assignment`

Reuses `mac_script`'s own enumeration:

```text
$ ws1-tf onboard --type script_assignment --path /Users/alex/uem-terraform
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 1
2 script assignments skipped: owned by child org group "Sales" (id 12350). Add --include-inherited to onboard them.
Select mac scripts whose assignments to onboard — Page 1/1 · 2 items
  1. script-1 (APPLE_OSX, uuid 00000000-0000-0000-0000-000000000007)
  2. script-2 (APPLE_OSX, uuid 00000000-0000-0000-0000-000000000008)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5] (←/→ page, / search): all
Importing script assignments: 0/2…
Importing script assignments: 2/2 done.
Onboarded 2 script assignments into /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/script_assignments.tf. Review required before commit/apply; no apply was run.
```

No warnings here. `assignment_uuid` is the assignment's own identifier, not a pointer at another UEM
object, so it isn't in the static reference table at all — it stays a plain literal and no warning is printed for it.
`memberships[].smart_group_uuid` is this run's one real reference, and it reuses the SAME
`sg_00000000_0000_0000_0000_000000000004` data block the `application_assignment` run above already
declared in `dependencies.tf` — same object, same declared block, matched by value — reading
`.smart_group_uuid` off that result, as the `application_assignment` run does. `script_uuid` (the
assignment's parent) resolves directly to the parent resource's address: `script-1` and `script-2`
are already in this working directory's state from the `mac_script` run above. `mac_script` has no
`uuid` attribute in its schema at all — its uuid is stored in the Computed `id` attribute instead —
so the rewrite reads `.id`, not `.uuid`. Generated file:

```text
$ cat /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/script_assignments.tf
# Assignments of script-1 — script uuid 00000000-0000-0000-0000-000000000007
# captured from UEM 26.2.1000.0 on 2026-09-25
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_script_assignment" "script-1" {
  assignments = [
    {
      assignment_uuid = "22222222-2222-2222-2222-222222222222"
      deployment_mode = "AUTO"
      memberships = [
        {
          smart_group_name = "smartgroup-1"
          smart_group_uuid = data.uem_smart_groups.sg_00000000_0000_0000_0000_000000000004.smart_groups[0].smart_group_uuid
        },
      ]
      name = "script-1-assignment"
      priority = 1
      script_deployment = {
        trigger_events = []
        trigger_schedule = ""
        trigger_type = ""
      }
      show_in_catalog = false
    },
  ]
  script_uuid = uem_mac_script.script-1.id
}

# Assignments of script-2 — script uuid 00000000-0000-0000-0000-000000000008
# captured from UEM 26.2.1000.0 on 2026-09-25
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_script_assignment" "script-2" {
  assignments = [
    {
      assignment_uuid = "33333333-3333-3333-3333-333333333333"
      deployment_mode = "AUTO"
      memberships = [
        {
          smart_group_name = "smartgroup-1"
          smart_group_uuid = data.uem_smart_groups.sg_00000000_0000_0000_0000_000000000004.smart_groups[0].smart_group_uuid
        },
      ]
      name = "script-2-assignment"
      priority = 1
      script_deployment = {
        trigger_events = []
        trigger_schedule = ""
        trigger_type = ""
      }
      show_in_catalog = false
    },
  ]
  script_uuid = uem_mac_script.script-2.id
}
```

#### `mac_sensor`

`uem_mac_sensor` is macOS-only, and `onboard` filters out any non-macOS sensor the tenant returns, with its own explicit skip message (distinct
from the owned/inherited skip line — this one is a platform filter, live-confirmed against a tenant
that has exactly one non-macOS sensor mixed in with two macOS ones):

```text
$ ws1-tf onboard --type mac_sensor --path /Users/alex/uem-terraform
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 1
1 non-macOS sensor skipped: uem_mac_sensor manages macOS sensors only.
Select sensors to onboard — Page 1/1 · 2 items
  1. sensor_1 (uuid 00000000-0000-0000-0000-000000000009)
  2. sensor_2 (uuid 11111111-1111-1111-1111-111111111111)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5] (←/→ page, / search): all
Importing sensors: 0/2…
Importing sensors: 2/2 done.
Onboarded 2 sensors into /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/sensors.tf. Review required before commit/apply; no apply was run.
```

No "N sensors skipped: owned by ..." line here — this org group owns every macOS sensor it can see
directly; the platform-filter line took the position that line would otherwise occupy. No warning
either: `organization_group_uuid` resolves the same way it did for `mac_script`, reusing the already-declared
`og_12345` block and reading `.uuid` off it. Generated file:

```text
$ cat /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/sensors.tf
# sensor_1 — uuid 00000000-0000-0000-0000-000000000009, org group "Global" (id 12345)
# captured from UEM 26.2.1000.0 on 2026-09-25
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_mac_sensor" "sensor_1" {
  code = filebase64("${path.module}/../sensors/sensor_1.sh")
  description = ""
  execution_architecture = "EITHER64OR32BIT"
  execution_context = "SYSTEM"
  language = "BASH"
  name = "sensor_1"
  organization_group_uuid = data.uem_organization_groups.og_12345.organization_groups[0].uuid
  response_data_type = "STRING"
}

# sensor_2 — uuid 11111111-1111-1111-1111-111111111111, org group "Global" (id 12345)
# captured from UEM 26.2.1000.0 on 2026-09-25
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_mac_sensor" "sensor_2" {
  code = filebase64("${path.module}/../sensors/sensor_2.sh")
  description = ""
  execution_architecture = "EITHER64OR32BIT"
  execution_context = "SYSTEM"
  language = "BASH"
  name = "sensor_2"
  organization_group_uuid = data.uem_organization_groups.og_12345.organization_groups[0].uuid
  response_data_type = "STRING"
}
```

#### `sensor_assignment`

Reuses `mac_sensor`'s own (platform-filtered) enumeration:

```text
$ ws1-tf onboard --type sensor_assignment --path /Users/alex/uem-terraform
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 1
1 non-macOS sensor skipped: uem_mac_sensor manages macOS sensors only.
Select sensors whose assignments to onboard — Page 1/1 · 2 items
  1. sensor_1 (uuid 00000000-0000-0000-0000-000000000009)
  2. sensor_2 (uuid 11111111-1111-1111-1111-111111111111)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5] (←/→ page, / search): all
Importing sensor assignments: 0/2…
Importing sensor assignments: 2/2 done.
Onboarded 2 sensor assignments into /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/sensor_assignments.tf. Review required before commit/apply; no apply was run.
```

No warnings here either. `assignment_uuid` is (as with `script_assignment`) the assignment's own
identifier, not a reference, so it's left untouched. `smart_group_uuids` (plural, uuid-form) resolves
the same way `script_assignment`'s `smart_group_uuid` did — reusing the already-declared
`sg_00000000_0000_0000_0000_000000000004` block, reading `.smart_group_uuid` off it. `sensor_uuid`
(the assignment's parent) resolves directly to the parent resource's address: `sensor_1` and
`sensor_2` are already in state from the `mac_sensor` run above, and like `mac_script`,
`uem_mac_sensor` has no `uuid` attribute — its uuid lives in the Computed `id` — so the rewrite reads
`.id`. Generated file:

```text
$ cat /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/sensor_assignments.tf
# Assignments of sensor_1 — sensor uuid 00000000-0000-0000-0000-000000000009
# captured from UEM 26.2.1000.0 on 2026-09-25
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_sensor_assignment" "sensor_1" {
  assignments = [
    {
      assignment_uuid = "44444444-4444-4444-4444-444444444444"
      event_triggers = []
      name = "sensor_1_assignment"
      ranking = 1
      smart_group_uuids = [data.uem_smart_groups.sg_00000000_0000_0000_0000_000000000004.smart_groups[0].smart_group_uuid]
      trigger_type = "SCHEDULE"
    },
  ]
  sensor_uuid = uem_mac_sensor.sensor_1.id
}

# Assignments of sensor_2 — sensor uuid 11111111-1111-1111-1111-111111111111
# captured from UEM 26.2.1000.0 on 2026-09-25
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_sensor_assignment" "sensor_2" {
  assignments = [
    {
      assignment_uuid = "55555555-5555-5555-5555-555555555555"
      event_triggers = []
      name = "sensor_2_assignment"
      ranking = 1
      smart_group_uuids = [data.uem_smart_groups.sg_00000000_0000_0000_0000_000000000004.smart_groups[0].smart_group_uuid]
      trigger_type = "SCHEDULE"
    },
  ]
  sensor_uuid = uem_mac_sensor.sensor_2.id
}
```

#### `update_deployment`

This type's enumeration does a per-deployment detail lookup, so it's visibly slower than the others
— real, not a hang:

```text
$ ws1-tf onboard --type update_deployment --path /Users/alex/uem-terraform
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 1
Select update deployments to onboard — Page 1/1 · 1 item
  1. update-deployment-1 (uuid 66666666-6666-6666-6666-666666666666)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5] (←/→ page, / search): all
Importing update deployments: 0/1…
Importing update deployments: 1/1 done.
Onboarded 1 update deployment into /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/update_deployments.tf. Review required before commit/apply; no apply was run.
```

This tenant's fixture org group has exactly one update deployment, so the "N update deployments skipped: UEM
didn't report which org group owns them." defensive message (for an empty-owner detail lookup) was not
observed live — as documented, that path exists for when the per-deployment detail comes back
empty, which this fixture doesn't hit. No warnings printed: `organization_group_uuid` and
`smart_group_uuids` each reuse the already-declared `og_12345`/`sg_00000000_0000_0000_0000_000000000004`
blocks (same pattern as `mac_sensor`/`sensor_assignment` above). `update_uuid` is different from the
other two — it points at an OS-update catalog entry, which this provider has no resource for at all,
so it's a deliberate, permanent external reference: it stays a raw literal, now with an inline comment
explaining why instead of a warning. Generated file:

```text
$ cat /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/update_deployments.tf
# update-deployment-1 — uuid 66666666-6666-6666-6666-666666666666, org group "Global" (id 12345)
# captured from UEM 26.2.1000.0 on 2026-09-25
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_update_deployment" "update-deployment-1" {
  deployment_start_time = "2026-09-24T09:58:00.000Z"
  deployment_type = "DOWNLOAD_ONLY"
  name = "update-deployment-1"
  notifications = []
  organization_group_uuid = data.uem_organization_groups.og_12345.organization_groups[0].uuid
  smart_group_uuids = [data.uem_smart_groups.sg_00000000_0000_0000_0000_000000000004.smart_groups[0].smart_group_uuid]
  update_uuid = "77777777-7777-7777-7777-777777777777"  # ws1-tf: external reference (OS-update catalog entry, not a managed object), kept as a literal
}
```

#### `purchased_application_assignment` — the VPP skip behavior

This tenant's purchased (VPP — Apple's Volume Purchase Program, its bulk app-licensing program)
apps all live under **Acme** (id `12346`), and **none of the 7 have
any assignment rule configured** — the real, unmodified default-tenant-state output for that case:

```text
$ ws1-tf onboard --type purchased_application_assignment --path /Users/alex/uem-terraform
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 2
No purchased app assignments to onboard: the 7 purchased apps in this org group have no assignment rules.
```

Under **Global** (owned-only default), these same 7 apps are owned by Acme, not Global, so they're
reported as inherited instead, and nothing is onboarded either way:

```text
$ ws1-tf onboard --type purchased_application_assignment --path /Users/alex/uem-terraform
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 1
7 purchased app assignments skipped: owned by child org group "Acme" (id 12346). Add --include-inherited to onboard them.
No purchased app assignments to onboard in org group "Global" (id 12345).
```

Both are the same underlying real behavior asked from two angles: `list`/enumerate finds every
purchased app the org group can see, the ownership filter (or, in the first transcript, the org
group's own direct ownership) decides which ones are even candidates, and — independently of
ownership — any candidate with no assignment rule is skipped. When none has a rule, that's the
single `No purchased app assignments to onboard: …` line above; when only some do, the rest are
counted in `N purchased apps skipped: no assignment rules to import.` above the picker. This
tenant's 7 fixture purchased apps happen to have zero assignment rules configured, so no
`purchased_application_assignment` resource was actually generated in this doc's captures — the
skip behavior itself is the real, verified default-state output this section documents.

#### `smart_group`

Onboarding smart groups directly, with `--org-group` in place of the org-group picker:

```text
$ ws1-tf onboard --type smart_group --org-group 12345 --path /Users/alex/uem-terraform
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
2 smart groups skipped: owned by child org group "Sales" (id 12350). Add --include-inherited to onboard them.
Select smart groups to onboard — Page 1/1 · 7 items
  1. All Corporate Dedicated Devices (uuid 99999999-9999-9999-9999-999999999999)
  2. All Corporate Shared Devices (uuid bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb)
  3. All Devices (uuid cccccccc-cccc-cccc-cccc-cccccccccccc)
  4. All Employee Owned Devices (uuid eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee)
  5. macos-app-managed (uuid ffffffff-ffff-ffff-ffff-ffffffffffff)
  6. macos-app-managed-excl (uuid 22222222-2222-2222-2222-222222222222)
  7. windows-devices (uuid 33333333-3333-3333-3333-333333333333)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5] (←/→ page, / search): all
Importing smart groups: 0/7…
Importing smart groups: 7/7 done.
Onboarded 7 smart groups into /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/smart_groups.tf. Review required before commit/apply; no apply was run.

Next: set -a; source /Users/alex/uem-terraform/prod/.env; set +a; terraform -chdir=/Users/alex/uem-terraform/prod/.ws1tf-onboard-work plan
action log: /Users/alex/uem-terraform/prod/.ws1tf/logs/20260928T055352Z-onboard.jsonl
```

The generated file (the first three blocks and the last; the other three have the same shape):

```text
$ cat /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/smart_groups.tf
# All Corporate Dedicated Devices — UEM id 90011, uuid 99999999-9999-9999-9999-999999999999, org group "Global" (id 12345)
# captured from UEM 26.2.1000.0 on 2026-09-27
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_smart_group" "All_Corporate_Dedicated_Devices" {
  criteria_type = "All"
  managed_by_org_group_id = "12345"
  name = "All Corporate Dedicated Devices"
  organization_group_ids = ["12345"]
  ownerships = ["corporatededicated"]
}

# All Corporate Shared Devices — UEM id 90012, uuid bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb, org group "Global" (id 12345)
# captured from UEM 26.2.1000.0 on 2026-09-27
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_smart_group" "All_Corporate_Shared_Devices" {
  criteria_type = "All"
  managed_by_org_group_id = "12345"
  name = "All Corporate Shared Devices"
  organization_group_ids = ["12345"]
  ownerships = ["corporateshared"]
}

# All Devices — UEM id 90013, uuid cccccccc-cccc-cccc-cccc-cccccccccccc, org group "Global" (id 12345)
# captured from UEM 26.2.1000.0 on 2026-09-27
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_smart_group" "All_Devices" {
  criteria_type = "All"
  managed_by_org_group_id = "12345"
  name = "All Devices"
}

# windows-devices — UEM id 90017, uuid 33333333-3333-3333-3333-333333333333, org group "Global" (id 12345)
# captured from UEM 26.2.1000.0 on 2026-09-27
# REVIEW REQUIRED — generated by ws1-tf, verify before apply
resource "uem_smart_group" "windows-devices" {
  criteria_type = "All"
  managed_by_org_group_id = "12345"
  name = "windows-devices"
  platforms = ["WinRT"]
}
```

Each block carries only the criteria UEM reports for that group — `ownerships`, `platforms`,
`management_types`, `organization_group_ids` and so on — and the resource address comes from the
group's name. The next `terraform plan` is a no-op, but it warns once about `All Devices`, because
a group with `criteria_type = "All"` and no criteria really does match every device:

```text
Warning: Smart Group Matches Every Device

  with uem_smart_group.All_Devices,
  on smart_groups.tf line 26, in resource "uem_smart_group" "All_Devices":
  26: resource "uem_smart_group" "All_Devices" {

criteria_type = "All" with no criteria configured: this smart group matches EVERY device in its managing organization group. Add criteria (for example platforms or ownerships) to narrow it, or ignore this warning if that is intended.
```

That warning is expected for UEM's built-in "All Devices" groups; it's there to catch a group you
write by hand and forget to narrow.

A smart group that another onboarded object targets is imported for you as a dependency (see
[Dependencies](#dependencies-how-onboard-handles-referenced-org-groups-and-smart-groups)), named
`sg_<UEM id>` and written into the referencing type's file. Onboarding smart groups afterwards
recognises it, counts it in the picker header and skips it:

```text
Select smart groups to onboard — Page 1/1 · 1 item · 1 already onboarded
  1. macOS devices (uuid 44444444-4444-4444-4444-444444444444)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5] (←/→ page, / search): all
Smart group "macOS devices" is already in this repo (uem_smart_group.sg_90004), skipped.
```

#### Profiles UEM can't serialise: skipped, not an error

Some profiles can't be read over the UEM REST API at all: UEM answers the profile read with HTTP 400
`Invalid Payload Key.` because it can't serialise one of the profile's payloads. On one lab
tenant this affected, for example, macOS profiles carrying Dock, Login Window, Restrictions, Security &
Privacy, Disk Encryption or Firewall payloads. These profiles aren't importable, so `onboard` skips
them, prints one count, and onboards the rest of your selection normally. The skipped profiles are
named in the run's action log (an `import_skipped` record each; see "The action log" below):

```text
94 profiles skipped: UEM can't serialise their payload over the API (400 Invalid Payload Key) (names in the action log).
```

(The unsupported-payload check in the next section reads each profile before the import does. A profile whose
read already fails there is reported by that section's "could not read" warning instead of this line.)

An object UEM answers with HTTP 500 when it's read is skipped the same way, for any type. That line
names the objects (up to 8, then "… and N more"):

```text
1 profile skipped: UEM returned a server error when reading it (Smart Card policy, id 3002). It can't be imported until UEM can read it.
```

Only those two errors are skipped: a 400 `Invalid Payload Key` on a profile and a 500 on any object. Any
other failure reading an object, including HTTP 502, 503 and 504 (a gateway or availability problem
rather than a fault of one object), still stops the run and rolls back what the run imported. If UEM
answers HTTP 500 for every object you selected, the run fails with an error that says it looks like a UEM
outage, instead of ending with nothing onboarded.

#### Profiles with a payload the provider doesn't support yet: skipped

`uem_profile` doesn't model every payload yet. Before importing a profile, `onboard` reads it and checks
for a non-empty payload the provider can't carry: Associated Domains, Exchange ActiveSync (native mail
client), Mail, Setup Assistant, Smart Card and SSO Extensions (the profile keys `AssociatedDomains`,
`EasNativeMailclient`, `EmailList`, `SkipSetupAssistant`, `SmartCard` and `SsoExtensionList`). That key
list is the only thing checked, for a profile of any platform; a payload the provider doesn't model that is
not on it isn't detected. VPN and Exchange (Microsoft Outlook) profiles are not on this list — see
the next section.
Importing such a profile would drop that payload from your configuration, and a later apply would
remove it from UEM, so `onboard` skips it. One line counts the skipped profiles and says which
payloads caused it; each skipped profile, with its payloads, is an `import_skipped` record in the
action log.

A profile whose custom attributes carry a script is skipped the same way, because UEM re-encodes
the script when it's written, so every later apply would try to change it. Custom attributes
without a script import normally:

```text
5 profiles skipped: they hold a payload the provider API does not support yet (SsoExtensionList 3, SmartCard 1, custom attribute scripts 1).
```

The rest of your selection is onboarded normally. Each payload is removed from this check once the
provider supports it.

A profile whose pre-check read fails, or whose answer is not a real read of that profile, can't be shown to be
safe, so it is skipped too, with a warning that counts the profiles and names the failure class (never a server
message). A real read has a non-empty `General` object, and a `General.ProfileId`, when it carries one, that is the
profile asked for; so `null`, `{}`, a bare message or error object, a proxy page, or the answer for a different
profile all count as unreadable. Importing it anyway could succeed on a later read and drop a payload the
pre-check never saw.
Each one is an `import_skipped` record with the reason `payload_check_unverified`:

```text
warning: 2 profiles skipped: the unsupported-payload check could not read them from UEM (returned 500 for 1, unreachable for 1), so they can't be verified safe to import.
```

#### Secrets in generated profiles: `secrets.json` and `secrets.tf`

UEM never returns some profile secrets when they're read back, whatever it actually holds. For these
`uem_profile` fields, `onboard` writes an entry to `secrets.json` (in the `uem-artifacts/` tree,
which the repo-root `.gitignore` excludes — see below) and points the generated resource at it, instead of a
misleading `= null` line (a `null` would read as "no password", when the truth is "UEM won't say"):

- `network_list[].password`, `network_list[].user_password`, `network_list[].proxy_password`
- `credentials_list[].certificate_payload`, `credentials_list[].certificate_password`
- `vpn_list[].password`, `vpn_list[].shared_secret`, `vpn_list[].vpn_password`
- `eas_microsoft_outlook.password`

`<repo>/uem-artifacts/<tenant>/secrets/secrets.json` — directory mode `0700`, file mode `0600`, and
kept out of git by the `/uem-artifacts/` rule in the repo-root `.gitignore` (`onboard` adds that rule
if it is missing; if you manage `.gitignore` yourself, make sure it contains `/uem-artifacts/`):

```json
{
  "uem_profile.Corp_WiFi.network_list[\"CorpNet\"].password": ""
}
```

Keys are the resource address plus attribute path; a `network_list`/`credentials_list`/`vpn_list`
entry is indexed by its own name (`service_set_identifier` / `credential_name` / `connection_name`)
when it has one, or a bare
numeric position (`[0]`) when it doesn't — and also when two entries of the same list share a name,
in which case every entry of that list uses its position, so each gets its own slot. A blank (`""`) value means UEM never returned this field
when this run imported it — fill it in by hand before `plan`/`apply`. Re-running `onboard` merges
into this same file: a key you've already filled in is never overwritten, a brand-new key is added,
and every other key is left untouched.

`onboard` also writes `secrets.tf` alongside the other generated files (only when at least one
secret exists, and only when it isn't there yet: a `secrets.tf` you have edited — for example to read
from a secret store — is never overwritten). It reads `secrets.json` relative to its own directory
(`${path.module}`), so it works whichever directory you run Terraform from. Its header comment sums up the rules this section explains:

```hcl
# ws1-tf: generated by `ws1-tf onboard` — do NOT hand-edit the locals
# block below; edit ../../uem-artifacts/prod/secrets/secrets.json itself instead.
#
# ../../uem-artifacts/prod/secrets/secrets.json holds the write-only secrets (network, credential, VPN and
# Exchange passwords and certificate payloads) that UEM never returns on read.
# Every generated resource that has one references
# local.secrets["<resource address>.<attribute path>"] != "" ?
# local.secrets["<resource address>.<attribute path>"] : null — there is no
# plan-time error. A blank ("") value in ../../uem-artifacts/prod/secrets/secrets.json renders as null, and the
# provider never sends a null write-only secret. UEM keeps the value it
# already has for network passwords and credential certificates, so
# plan/apply are a clean no-op for those. VPN and Exchange secrets are
# different: UEM clears one an update leaves out, so the provider refuses
# that update until the secret is filled in. Fill in a key only when you
# need that value applied — most importantly before the FIRST apply against
# a NEW environment, since a blank secret there means the real value is
# never sent at all.
#
# The first apply after filling a key sends that value to UEM in place:
# UEM never returns a write-only field on read, so Terraform state has no
# way of knowing the value already matches what's configured — expect a
# one-time in-place update for it, not drift.
#
# Terraform state itself still stores every secret value once applied
# (state is not encrypted at rest by default) — protect it the same way
# you protect ../../uem-artifacts/prod/secrets/secrets.json: a remote backend with encryption at rest is advised
# for any state that holds these values.
#
# To swap in a real secret store later, replace the locals block below
# with a data source that produces the SAME shape: a string map keyed by
# the exact "<resource address>.<attribute path>" keys ../../uem-artifacts/prod/secrets/secrets.json uses today,
# e.g.:
#   data "vault_kv_secret_v2" "ws1tf" { ... }
#   locals { secrets = data.vault_kv_secret_v2.ws1tf.data }
locals {
  secrets = sensitive(jsondecode(file("${path.module}/../../uem-artifacts/prod/secrets/secrets.json")))
}
```

and the generated resource references it directly, as a conditional — **there is no plan-time
error**:

```hcl
resource "uem_profile" "Corp_WiFi" {
  # 1 secrets are read from secrets.json
  network_list = [
    {
      password               = local.secrets["uem_profile.Corp_WiFi.network_list[\"CorpNet\"].password"] != "" ? local.secrets["uem_profile.Corp_WiFi.network_list[\"CorpNet\"].password"] : null
      service_set_identifier = "CorpNet"
    },
  ]
}
```

A blank (`""`) secrets.json value renders as `null`, and a `null` write-only secret is never sent.
What UEM does then depends on the payload (tested on a live tenant):

- Network (`network_list`) passwords and credential (`credentials_list`) certificates: UEM keeps the
  value it already has, so `plan`/`apply` are a clean no-op for that attribute.
- VPN (`vpn_list`) and Exchange (`eas_microsoft_outlook`) secrets: UEM clears a secret an update
  leaves out. `onboard` now imports VPN and Exchange (Microsoft Outlook) profiles too, and the
  post-onboard `terraform plan` is still 0 changes right after import — a blank secrets.json value
  renders as `null`, and Terraform never sends a `null` write-only attribute, so there is nothing to
  clear on a plan with no other changes. What's different from network/credential secrets is what
  happens next: **any later change to that profile that still leaves the secret blank is blocked at
  plan time**, even a change unrelated to the VPN/Exchange payload, because (unlike
  `network_list`/`credentials_list`) UEM wipes an omitted VPN/Exchange secret on update instead of
  keeping it. The provider checks this at plan time and fails the plan with an error naming the
  secret; fill it in `secrets.json` before applying that change.

After the import, one line counts the write-only secrets this run wrote to `secrets.json`. A
second line counts write-only secret fields in settings the provider doesn't manage yet (for
example Android or Windows payloads). Those aren't in `secrets.json`; they're part of the import
completeness report. Each profile's secret fields are in its `import_completeness` action-log
record:

```text
26 write-only secrets in 8 profiles are tracked in ../../uem-artifacts/prod/secrets/secrets.json (UEM never returns them).
130 more write-only secret fields are in settings the provider doesn't manage yet (see import completeness).
```

When a run adds blank secrets, `onboard` prints a reminder listing the new ones, at most 8 of them
followed by "… and N more (see secrets.json)"; the full list is a `secrets_blank` record in the
action log. A later run that adds none prints nothing. The reminder only lists blanks added by
this run; to find every blank secret, including ones from earlier runs, check `secrets.json`
directly:

```text
2 new secrets blank in ../../uem-artifacts/prod/secrets/secrets.json. Fill them before applying to a NEW environment; a blank secret is not sent.
uem_profile.Corp_WiFi.network_list["CorpNet"].password
uem_profile.Corp_WiFi.network_list["CorpNet"].user_password
```

**Fill in a key only when you actually need that value applied** — most importantly before the
first `apply` against a brand-new environment, since a blank secret there means the real value is
never sent at all, not merely deferred. **The first `apply` after filling a key sends that value to
UEM in place** — UEM never returns a write-only field on read, so Terraform state has no way of
knowing the value already matches what's configured; expect a one-time in-place update for it (not
suppressed, not drift).

**Terraform state itself still stores every secret value once applied** (state is not encrypted at
rest by default) — protect it the same way you protect `secrets.json`: a remote backend with
encryption at rest is advised for any state that holds these values.

**Swapping in a real secret store later:** replace `secrets.tf`'s `locals` block with a data source
that produces the same shape — a string map keyed by the exact same `"<resource address>.<attribute
path>"` keys `secrets.json` uses today, for example:

```hcl
data "vault_kv_secret_v2" "ws1tf" { ... }
locals {
  secrets = data.vault_kv_secret_v2.ws1tf.data
}
```

**Only those fields are treated as secrets, and the match is on the exact attribute name.** An attribute
named exactly `password`, `user_password`, `proxy_password`, `certificate_payload`, `certificate_password`,
`shared_secret` or `vpn_password` is never written to the file with a value: the nine fields above go to `secrets.json`, and any attribute with one of those names
that is not routed there is replaced with `<REDACTED — manual entry required>`, with
`# ws1-tf: secret redacted; supply the real value before apply` at the end of the line.

**Everything else UEM returns is written exactly as UEM returned it.** ws1-tf does not look for
secret-shaped names or secret-shaped content. An attribute with a name such as `psk`, `passphrase` or
`client_secret`, a secret inside a text value (for example a `Password` key in a `custom_settings`
property list), and the bodies of scripts and sensors all land in the generated files in clear text.
Review the generated files for secrets before you commit them.

#### Import completeness: UEM fields the provider doesn't manage yet

After the import, `onboard` reads each imported object from UEM again (the same endpoint and
version the provider reads) and compares every field UEM reports against what Terraform state
captured. A field no attribute carries is a setting the provider doesn't manage yet: it's left out
of the generated config, and UEM keeps its current value. `onboard` lists those fields so nothing is
dropped silently. Only field names are reported, never
values. When every field was captured, nothing is printed.

**Only four types are checked: `profile`, `mac_application`, `mac_script` and `mac_sensor`.** The
assignment types (`application_assignment`, `script_assignment`, `sensor_assignment`,
`purchased_application_assignment`), `update_deployment` and `smart_group` are not read back, so for
them silence does not mean "everything was captured". If the read of an object fails, the check prints
`warning: import completeness not checked for <address>: ...` for that object and the onboard still
succeeds.

A real run, onboarding two profiles on a lab tenant:

```text
Importing profiles: 0/2…
Importing profiles: 2/2 done.
Import completeness: 4 UEM field(s) are not managed by the provider yet and were left out of the generated config (UEM keeps them as they are):
  General.AllowRemoval, General.EnableProvisioning, General.IsManaged, General.IsProvisionedForOobe — on all 2 profiles
Details per resource: the action log's "import_completeness" records, keyed by "address".
Onboarded 2 profiles into /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/profiles.tf. Review required before commit/apply; no apply was run.
```

The count on the first line is distinct field names. Below it, fields missing from exactly the same
objects share one line, which says how many of the run's objects of that type it applies to — `on
all 2 profiles`, or `on 1 of 2 mac scripts` when only some are affected:

```text
Import completeness: 3 UEM field(s) are not managed by the provider yet and were left out of the generated config (UEM keeps them as they are):
  is_idempotent, version — on all 2 mac scripts
  catalog_display.use_default_icon — on 1 of 2 mac scripts
Details per resource: the action log's "import_completeness" records, keyed by "address".
```

A large run stays readable: a line names at most 8 fields and then says `… and N more (see the
action log)`, and at most 10 lines are printed, followed by `… and N more field groups (see the
action log)`. From a 388-profile `--include-inherited` run:

```text
Import completeness: 1059 UEM field(s) are not managed by the provider yet and were left out of the generated config (UEM keeps them as they are):
  …
  VpnList[].ConnectAutomatically, VpnList[].ConnectionName, VpnList[].ConnectionType, VpnList[].DeadPeerDetectionRate, VpnList[].EAPPassword, VpnList[].EnableVPNOnDemand, VpnList[].EncryptionLevel, VpnList[].ExcludeLocalNetworks … and 15 more (see the action log) — on 18 of 388 profiles
  WebClips.FullScreen, WebClips.Icon, WebClips.Label, WebClips.PrecomposedIcon, WebClips.Removable, WebClips.URL — on 17 of 388 profiles
  VpnList[].CommunicationServer, VpnList[].VpnUserAuthentication — on 17 of 388 profiles
  … and 73 more field groups (see the action log)
Details per resource: the action log's "import_completeness" records, keyed by "address".
```

(The first `…` stands for seven field-group lines cut here.) The last line is the way into the
full detail: the run's action log has one `import_completeness` record per imported object, keyed
by its Terraform `address`, with every uncaptured field and any write-only secret fields among them
(see [Reading the action log](#58-reading-the-action-log)). Other types report the same way — for
example `ManagedByOrganization — on all 3 mac applications` or `timeout — on all 2 sensors` on that
tenant.

#### Two warnings you will see in real use — both benign, read them once and move on

1. **A reference `onboard` genuinely could not resolve to a lookup or a managed resource.** None of
   the transcripts above trigger this — every org-group, smart-group, and assignment-parent reference
   in this doc's captures resolves cleanly (see each `#### <type>` subsection above) — but one real
   case still produces a warning and keeps the reference as a raw literal rather than emitting a
   lookup that would fail at `terraform plan`:
   - An assignment's parent was skipped by owned-only filtering (not owned by the selected org group,
     `--include-inherited` wasn't passed):
     `warning: uem_script_assignment.script-1: parent mac_script 00000000-0000-0000-0000-000000000001 is not owned by the selected org group 12345 (skipped by owned-only filtering), so it was not imported and the reference stays a raw literal; use --include-inherited to import it`

   An org-group or smart-group reference this credential can't read is NOT a warning. This is a real,
   expected case for an API account scoped to a child org group that sees an item inherited from an
   ancestor it can't read directly. `onboard` probes access before emitting a lookup (the same call
   the data source itself would make). When the probe is refused, it writes a documented, read-only
   `locals` entry with the known id and uuid into `dependencies.tf`, and the reference points at it,
   so `terraform plan` makes no API call for it:

   ```hcl
   locals {
     # fixed value: this org group is outside this account's access
     og_12346 = {
       id   = "12346"
       uuid = "00000000-0000-0000-0000-000000000002"
     }
   }
   ```

   The reference reads `local.og_12346.id` (smart groups: `local.sg_<id>.smart_group_id` or
   `.smart_group_uuid`). Once the account is granted access, the next `onboard` run emits a real
   `data` lookup, repoints references to it, and prints a `note:` line; the old `locals` entry stays,
   unused. A smart group referenced only by UUID that UEM's search doesn't return (typically an org
   group's own smart group) gets the same kind of entry, with a comment that says so instead of
   claiming an access problem; see the Dependencies section above.

   Everything else — every reference the earlier subsections captured — resolves silently: a `data`
   block in `dependencies.tf` and a rewritten expression, no warning at all.
2. **`WARNING: terraform plan is NOT a no-op after import...`** — NOT shown in any transcript above
   (every plan in this doc's captures came back a genuine no-op — see Section 5.5's closing plan check).
   `ws1-tf` runs `terraform plan` against the just-imported state internally specifically to catch this, and prints the warning loudly whenever
   that plan reports a change. A plan that would destroy or replace anything is not a warning but an error:
   `onboard` fails, naming the addresses, and says not to apply (a destroy means state holds an object with no
   config block, and applying would delete the real tenant object). The warning's trigger is
   platform-agnostic and attribute-agnostic: it fires
   whenever the provider's `Read()` doesn't perfectly round-trip a populated attribute back into the
   same value the generated config already has, regardless of which attribute or which OS/type is
   involved. Review the generated file before `apply` when you see it; do not `apply` blind.

#### Paging, search, and range selection in the pickers

**Behavior:** every item picker (the numbered list `onboard` shows for profiles, mac applications,
application assignments, mac scripts, script assignments, mac sensors, sensor assignments, update
deployments, purchased application assignments, and smart groups) and the org-group picker now page and search
when the run is interactive (a real terminal). Instead of one flat numbered list, a
large tenant's items are paged — a 498-item list does not print 498 lines before you can
type anything.

On a real terminal, each picker shows a header line, a page of items sized to fit your terminal
(minimum 10 rows, falling back to 20 when the terminal doesn't report a size), and an input prompt:

```text
Select profiles to onboard — Page 2/25 · 498 items · filter: 'mac' · 3 already onboarded
```

The `· filter: '...'` segment only appears once you've searched, and `· N already onboarded`
only appears when this work directory's Terraform state already has some of the enumerated items
imported (the same fact the post-selection "N profiles already in this repo, skipped"
line reports — surfaced earlier here, at picker time, so you're not confused mid-selection about
why an item you expected behaves oddly). Both the item count and the page count reflect the
active filter, not the unfiltered total.

Keys, identical between every item picker and the org-group picker:

- **Left/Right arrow** — page back/forward. Paging never renumbers anything: item numbers are
  global across the active filter's full list, not per-page, so you can type `23` after only ever
  having seen page 1.
- **Digits, comma, hyphen, letters** — build up the selection you're typing (e.g. `23,25-28`, or
  `all`), echoed as you type.
- **Backspace/Delete** — erases the last character you typed.
- **Enter** — submits what you've typed. An empty buffer is a no-op (it will never submit an
  accidental empty selection). The item pickers accept `all`, plain numbers, and inclusive ranges
  freely mixed (`3,7-9,12`); the org-group picker is still a single pick (Enter with exactly one
  valid number picks it immediately — no `all`/ranges/comma-lists there). A selection that is
  malformed or out of range does not end the picker: it prints what was wrong, clears what you typed
  and asks again.
- **`/`** — starts typing a search: it matches case-insensitively against each item's display
  name, and Enter applies it (resetting to page 1). Esc while typing a search cancels just the
  text you were typing, leaving whatever filter was already active untouched.
- **Esc** (not while typing a search) — clears the currently active search filter, if any. A lone
  Esc acts after 50 ms; other escape sequences (modified arrows, function keys, Alt+key) are read in
  full and ignored, so they never leak characters into what you're typing.
- **Ctrl-C** — cancels the picker.

Names that come from UEM, and text that comes from the repo (the README, folder names), are cleaned before they
are printed, so a name can't move the cursor, break a line or hide the text around it. One rule covers the
terminal lines: control characters (and the Unicode line and paragraph separators) become a space, and Unicode
format characters (bidirectional overrides, zero-width joiners, the BOM) are removed. It applies to:

- item lines in the pickers and in the plain numbered list, and names in generated header comments (the
  interactive picker then also drops any control character that is left);
- org-group lines, the UEM version in the confirmation prompts, `verify`'s version line, the `Importing <name>`
  line, the server-error skip line, and the interactive menu's header, environment picker and menu labels
  (Section 5.3).

Two places differ. The `list` table drops control characters, and tab, CR and LF become a space.
`refresh-local` shows control characters as visible escapes (Section 5.9).

The transcripts in this doc show what a real terminal leaves on screen once you submit: the header
line, the page of items, and the prompt with its `(←/→ page, / search)` hint. Every list here fits
on one page, so each header reads `Page 1/1`.

The **same range/`all` grammar works non-interactively too** (piped/redirected stdin). There is no
header or paging then — the full numbered list prints once, followed by the plain
`Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5]` prompt, which accepts
`3-7`/`3,7-9,12`-style ranges exactly like the paged picker does — one shared implementation, so a
scripted/non-interactive caller gets the same range support for free.

#### Owned-only by default: `--include-inherited` and `--org-group`

**Behavior:** `onboard` imports only items whose OWNING org group is the one you selected — not
merely items VISIBLE under it. An org group can see items it doesn't own: a child org group inherits
everything visible to its ancestors, and some list endpoints also surface descendant-owned items.
Importing an inherited item is the trap this default avoids: if you later also `onboard` the other
org group into a second Terraform config, that same UEM object would end up managed by two separate
Terraform states at once — the next `apply` from either side can then fight the other, or worse,
delete what the other created. Items skipped this way are never silent — one line per other owner,
live-confirmed with **two distinct owners in the same run** (selecting **Acme**, owned-only, for
`profile`):

```text
$ ws1-tf onboard --type profile --path /Users/alex/uem-terraform
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 2
2 profiles skipped: owned by org group 00000000-0000-0000-0000-000000000003. Add --include-inherited to onboard them.
3 profiles skipped: owned by parent org group "Global" (id 12345). Add --include-inherited to onboard them.
No profiles to onboard in org group "Acme" (id 12346).
```

Acme owns none of the profiles it can see — every one of the 5 visible profiles belongs to one of
two other owners (Global itself, and a third org group these credentials can't select directly but
whose ownership is still detected) — so both skip lines appear and nothing is onboarded.

Items owned by an org group BELOW the selected one are skipped the same way, and the line says
`child`. Selecting **Global** for `profile`, the profiles its child org group Sales owns are visible
but not onboarded:

```text
2 profiles skipped: owned by child org group "Sales" (id 12350). Add --include-inherited to onboard them.
```

The line names the owner as precisely as UEM lets `onboard` tell: `parent`/`child` plus the name
when the owner is in the selected org group's own tree (`parent org group id <n>` when only its id
is known), or the bare uuid when it isn't. A run prints one line per owner — a real owned-only
`profile` run under an org group with both a parent and a child printed a `parent org group id …`
line and a `child org group "…" (id …)` line, one for each direction.

With `--include-inherited`, `profile` can also see profiles on platforms `uem_profile` can't import
(for example AppleVision, AppleTv or IotPrinter). They are left out of the picker with one line,
such as `6 profiles skipped: their platform (AppleTv, AppleVision, IotPrinter) is not supported by
the provider.`, and never reach the import.

Pass `--include-inherited` to import every item with a KNOWN owner from the selected scope,
regardless of which org group that owner is — useful when you deliberately want one Terraform
config to be the single source of truth across a whole org group tree. This does NOT extend to an
item whose owner could not be determined at all (an empty owner field — handled defensively for
`update_deployment`, whose per-deployment detail lookup can come back as an empty record; not
observed live in this doc's captures, since this tenant's fixture org group's one update deployment
has a known owner) — those are always skipped, with their own message ("N update deployments skipped: UEM didn't
report which org group owns them."), never imported by any flag, since importing an item with an unknown
owner defeats the whole point of ownership-based filtering. `script_assignment` and
`sensor_assignment` follow their parent type's ownership (they reuse the same enumerator), so
`--include-inherited` on those assignment types has the same effect as on their parent type.

**Exception: `update_deployment`.** The update-deployments endpoint filters strictly by org group, with
no ancestor or descendant visibility (confirmed against a 26.2 tenant), so `--include-inherited` cannot
widen it: only deployments the selected org group itself owns are listed.

**Exception: `mac_application` and `application_assignment`.** `--include-inherited` has NO effect
for these two types — a parent-owned (ancestor-owned) app is always skipped, never imported, no
matter what flags are passed. Root cause: the app binary download a `mac_application` import needs
(`BlobsV2_Get`) is authorized by DIRECT ownership of the org group you're onboarding into, not by
visibility — an ancestor-owned app's download is refused regardless of size, even though the same app
is fully visible and enumerable. There is no other download path to fall back to. Skipped apps get
one summary line per owning org group, with the `--org-group` to use instead:

```text
1 mac application skipped: owned by parent org group id 12344. Onboard it with --org-group 12344.
```

To onboard that app, run `onboard --type mac_application` again with `--org-group` (or the
interactive picker) pointed at the org group that actually owns it.

Live-verified in a **fresh** repo (a separate `.ws1tf-onboard-work/` with no prior `profile` onboard
in it — see the note at the end of this subsection for why that matters):

```text
$ ws1-tf onboard --type profile --include-inherited --path /Users/alex/uem-terraform-wide
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 2
Select profiles to onboard — Page 1/1 · 5 items
  1. Compliance Test Profile (Apple iOS, profile_id 12350)
  2. Custom Profile (AppleOsX, profile_id 12347)
  3. Restrictions Profile (AppleOsX, profile_id 12348)
  4. Windows_Test_Profile (Windows 10, profile_id 12351)
  5. windows _device_profile (Windows 10, profile_id 12349)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5] (←/→ page, / search): all
Importing profiles: 0/5…
Importing profiles: 5/5 done.
Onboarded 5 profiles into /Users/alex/uem-terraform-wide/prod/.ws1tf-onboard-work/profiles.tf. Review required before commit/apply; no apply was run.
```

(This run's org-group and smart-group references resolve the same way as the `profile` capture
earlier in this doc — no warnings, matching `data` blocks written into this fresh working directory's
own `dependencies.tf`.) All 5 profiles Acme can see (owned by itself, by Global, and by the
third, not-directly-selectable owner) came in — no skip lines at all with `--include-inherited`, and
the count (5) matches exactly the 2+3 the owned-only capture above reported as skipped.

**Onboarding overlapping scopes into the SAME repo skips cleanly instead of erroring.** An
owned-only run followed by a wider `--include-inherited` run into that same `.ws1tf-onboard-work/`
commonly reaches some of the same items twice, which would otherwise risk a raw Terraform "Duplicate
resource" error partway through the import batch, or an orphaned
state entry (a resource in state with no matching config block, so the next `plan` would propose
destroying something nobody asked to remove). A second run now recognizes every
item it already onboarded, skips each one with a single summary line, imports only what's genuinely
new, and does so all-or-nothing — no partial batch, no orphaned state.

The same rule covers one object that appears **twice in a single selection**. A UEM list can return an object
on two pages when something is created or deleted while the list is being read, and importing both copies would
put one UEM object under two Terraform addresses (two resources managing it). The first copy is imported and
the repeat is skipped and reported like any other object that is already in the repo, under the address of the
copy that was kept: `Profile "<name>" is already in this repo (<address>), skipped.` when it is the only skip, or
`N profiles already in this repo, skipped: <addresses>` when there are several.

A related case is an address that a `.tf` file in the work directory already declares while the state has no
such resource (a hand-written block, or one left by an earlier failed run). Terraform's own "Duplicate resource"
error is replaced by `<address> is already declared in <file>; remove it or import it manually`. The file is found
by parsing the `.tf` files as HCL and looking for a top-level `resource` block with those labels, so a `resource`
line inside a comment or a heredoc is not mistaken for a declaration.

Live-verified in a **fresh** repo (`/Users/alex/uem-terraform-cross-run`): first the same owned-only
`profile` onboard under **Global** shown earlier in this section —

```text
$ ws1-tf onboard --type profile --path /Users/alex/uem-terraform-cross-run
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 1
2 profiles skipped: owned by child org group "Sales" (id 12350). Add --include-inherited to onboard them.
Select profiles to onboard — Page 1/1 · 3 items
  1. Custom Profile (AppleOsX, profile_id 12347)
  2. Restrictions Profile (AppleOsX, profile_id 12348)
  3. windows _device_profile (Windows 10, profile_id 12349)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5] (←/→ page, / search): all
Importing profiles: 0/3…
Importing profiles: 3/3 done.
Onboarded 3 profiles into /Users/alex/uem-terraform-cross-run/prod/.ws1tf-onboard-work/profiles.tf. Review required before commit/apply; no apply was run.
```

(This run's org-group and smart-group references resolve the same way as the `profile` capture
earlier in this doc — no warnings, matching `data` blocks written into this fresh working directory's
own `dependencies.tf`.) Then, in that SAME repo, `--include-inherited` under **Acme** — the
wider scope that reaches those same 3 profiles again, plus 2 more it hasn't seen before:

```text
$ ws1-tf onboard --type profile --include-inherited --path /Users/alex/uem-terraform-cross-run
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 2
Select profiles to onboard — Page 1/1 · 5 items · 3 already onboarded
  1. Compliance Test Profile (Apple iOS, profile_id 12350)
  2. Custom Profile (AppleOsX, profile_id 12347)
  3. Restrictions Profile (AppleOsX, profile_id 12348)
  4. Windows_Test_Profile (Windows 10, profile_id 12351)
  5. windows _device_profile (Windows 10, profile_id 12349)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5] (←/→ page, / search): all
3 profiles already in this repo, skipped: uem_profile.Custom_Profile, uem_profile.Restrictions_Profile, uem_profile.windows__device_profile
Importing profiles: 0/2…
Importing profiles: 2/2 done.
Onboarded 2 profiles into /Users/alex/uem-terraform-cross-run/prod/.ws1tf-onboard-work/profiles.tf. Review required before commit/apply; no apply was run.
```

No warnings here either. `Compliance_Test_Profile` and `Windows_Test_Profile` both reference
`org_group_id=12352` — the same third, not-directly-selectable owner introduced earlier in this
subsection (uuid `00000000-0000-0000-0000-000000000003`), just its numeric id this time — and
`Compliance_Test_Profile` also references a new smart group, `90003`. `dependencies.tf` in this same
working directory picks up two new blocks for this run (`og_12352`, `sg_90003`); the `og_12345`,
`sg_90001`, and `sg_90002` blocks the first run in this repo already declared are left exactly as
they were — `onboard`'s dependency lookups are idempotent the same way the resources themselves are:
a second run reuses any already-declared block for a value it's seen before (matched by that block's
filter value) instead of emitting a duplicate, so this repo never ends up with two data blocks for the
same org group or smart group. Exit code 0 — no raw Terraform error, no manual state surgery needed.
`terraform state list` in that same working directory shows exactly the 5 resources now in
`profiles.tf` — the 3 from the first run plus these 2, no duplicates and nothing orphaned — and the
very next `terraform plan` there comes back a genuine no-op:

```text
$ terraform state list
uem_profile.Compliance_Test_Profile
uem_profile.Custom_Profile
uem_profile.Restrictions_Profile
uem_profile.Windows_Test_Profile
uem_profile.windows__device_profile

$ terraform plan
No changes. Your infrastructure matches the configuration.
```

Re-running the original owned-only command a third time, in that same repo, now skips everything and
imports nothing — the fix makes every re-run of a command idempotent, not just the first
cross-scope one:

```text
$ ws1-tf onboard --type profile --path /Users/alex/uem-terraform-cross-run
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Select the org group to target — Page 1/1 · 2 items
  1. Global (Container, id 12345)
  2. Acme (Customer, id 12346)
Select the org group to target (number) (←/→ page, / search): 1
2 profiles skipped: owned by child org group "Sales" (id 12350). Add --include-inherited to onboard them.
Select profiles to onboard — Page 1/1 · 3 items · 3 already onboarded
  1. Custom Profile (AppleOsX, profile_id 12347)
  2. Restrictions Profile (AppleOsX, profile_id 12348)
  3. windows _device_profile (Windows 10, profile_id 12349)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5] (←/→ page, / search): all
3 profiles already in this repo, skipped: uem_profile.Custom_Profile, uem_profile.Restrictions_Profile, uem_profile.windows__device_profile
```

— no `Onboarded ...` line, and `terraform plan` in that working directory is still a clean no-op.

`--org-group <id|uuid>` replaces ONLY the numbered org-group picker with a non-interactive
value — the UEM-version confirm prompt and the item-selection prompt still run exactly as shown
elsewhere in this doc. The value is validated against the org groups your credentials can see, and
`onboard` (and `init`) fails loudly if it matches none. Note `init`'s `--org-group` only scopes
`init` itself (it's recorded into the scaffolded repo's README `OrgGroupScope`) — it does NOT carry
over to `onboard`, which reads its own `--org-group` flag independently on every run. A numeric value is
matched against the visible org-group ids with no extra API call. Any other value is read as a uuid: org-group
uuids are not in the listing, so `ws1-tf` reads each visible org group's uuid in turn until one matches (case
insensitive). Some org groups can be visible but not directly readable by a scoped account (see Dependencies
above). A group the credential cannot read, meaning a read that fails with HTTP 404 or 403, or with HTTP 400
and a body that says both "not found" and "access", is skipped, so one such group ahead of the match does not
stop the run; if nothing matches, the error says how many org groups could not be read and were skipped. Any
other failure, such as an authentication or network error, still **stops the run with an error**. A blank
`--org-group` (an empty value, or only spaces, as from an unset shell variable) is an error,
`--org-group needs a non-empty value (a numeric org group id or a uuid)`, raised before any call to UEM, on
both `onboard` and `init`; it never falls back to the interactive picker. The two transcripts below, `init` and
`onboard`, were captured live:

```text
$ ws1-tf init --path /Users/alex/uem-terraform-pinned --org-group 12345
Tenant name (e.g. dev, test, prod): prod
Console URL (instance_url): 
API tenant code (tenant_code / aw-tenant-code): 
Username (basic auth): 
Password (basic auth): 
OAuth client_id: 
OAuth client_secret: 
OAuth token URL (oauth2_token_url): 
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
Initialized /Users/alex/uem-terraform-pinned (tenant "prod"). Credentials written to /Users/alex/uem-terraform-pinned/prod/.env
```

Note the org-group picker never appears — `--org-group 12345` (Global) replaced it entirely, and
the resulting `README.md` records `Org group scope: Global (id 12345)`.

```text
$ ws1-tf onboard --type mac_application --org-group 12345 --path /Users/alex/uem-terraform-pinned
UEM console version: 26.2.1000.0
Proceed against UEM version 26.2.1000.0? [y/N]: y
2 mac applications skipped: owned by parent org group id 12344. Onboard them with --org-group 12344.
Select mac applications to onboard — Page 1/1 · 2 items
  1. MacOSDMGTestApp (uuid 00000000-0000-0000-0000-000000000005)
  2. MacOSPkgStyleTestApp (uuid 00000000-0000-0000-0000-000000000006)
Onboard all, or select (comma-separated numbers, ranges ok) [all/1,2,3-5] (←/→ page, / search): all
Onboarded 2 mac applications into /Users/alex/uem-terraform-pinned/prod/.ws1tf-onboard-work/mac_applications.tf. Review required before commit/apply; no apply was run.
```

Same behavior on `onboard`: the numbered org-group picker is skipped entirely, but the
version-confirm and item-selection prompts still run exactly as always. The interactive numbered
picker (shown throughout this doc's other transcripts) stays the default whenever `--org-group` is
omitted.

**`--name <substring>` ** is `onboard`'s equivalent narrowing flag for the item picker: a
case-insensitive substring match against each item's display name, applied BEFORE either the paged
picker or the non-interactive plain prompt ever sees the list — replacing only the picker's input,
the same way `--org-group` replaces only the org-group picker's input (the selection step itself
still runs; `--name` only narrows what's selectable). `ws1-tf onboard --type profile --name
passcode` shows only profiles whose name contains "passcode". When nothing matches, `onboard` prints
`no items match --name "passcode"` and exits cleanly (not an error) — the same shape as the
existing 'No profiles to onboard in org group "<name>" (id <n>).' diagnostic.

**Nested values render as native HCL.** Every field this
tenant's profiles actually set (including nested lists/objects like `custom_settings_list` and
`restrictions` above) renders as native HCL, not a flattened JSON string — `terraform plan` does not
fail on shape alone.
Whether a plan comes back as a no-op still depends on the provider's `Read()` round-trip fidelity
(warning 2 above), not on the attribute's shape or the type/platform involved — and every plan this
doc captured did come back clean (see the final check below).

#### Resulting folder structure (onboard-work directory)

`onboard` works in a dedicated, reusable working directory under the tenant (`.ws1tf-onboard-work/`),
separate from the tenant's own `versions.tf`/`README.md`/`.env`:

```text
$ find /Users/alex/uem-terraform -type f | sort
/Users/alex/uem-terraform/.gitignore
/Users/alex/uem-terraform/.ws1-tf.yaml
/Users/alex/uem-terraform/prod/.env
/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/application_assignments.tf
/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/dependencies.tf
/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/mac_applications.tf
/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/mac_scripts.tf
/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/profiles.tf
/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/provider.tf
/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/script_assignments.tf
/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/sensor_assignments.tf
/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/sensors.tf
/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/smart_groups.tf
/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/terraform.tfstate
/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/terraform.tfstate.backup
/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/update_deployments.tf
/Users/alex/uem-terraform/prod/README.md
/Users/alex/uem-terraform/prod/scripts/script-1.sh
/Users/alex/uem-terraform/prod/scripts/script-2.py
/Users/alex/uem-terraform/prod/sensors/sensor_1.sh
/Users/alex/uem-terraform/prod/sensors/sensor_2.sh
/Users/alex/uem-terraform/prod/versions.tf
```

(`purchased_application_assignment` generated no file in this tenant, per the VPP skip behavior
above; a Terraform run also leaves timestamped `.tfstate.<n>.backup` files behind after multiple
`apply`/import runs in the same directory, omitted here for brevity. The listing is also trimmed:
it leaves out the action logs (`.ws1tf/logs/*.jsonl` under the repo root and under the tenant,
Section 5.8), `uem-artifacts/prod/app-binaries/<uuid>/...` (downloaded `mac_application` installers,
pkginfo and icons), `uem-artifacts/prod/secrets/secrets.json` and the `secrets.tf` that reads it when
a profile had write-only secrets (Section "Secrets in generated profiles"), and the `.ws1tf/reports/`
files `refresh-local` writes.)

`provider.tf` (regenerated on every `onboard` run, in the onboard-work directory, so a hand edit to it
is lost — sets
`app_binary_storage_path` to a repo-relative path, computed from the actual hop count between the
onboard-work directory and `<repo>/uem-artifacts/<tenant>/app-binaries`, never hardcoded):

```text
$ cat /Users/alex/uem-terraform/prod/.ws1tf-onboard-work/provider.tf
terraform {
  required_providers {
    uem = { source = "omnissa/uem", version = "26.2.0" }
  }
}
provider "uem" {
  app_binary_storage_path = "../../uem-artifacts/prod/app-binaries"
}
```

The file is replaced through a temporary file in the same directory and a rename, so a symlink at that path is
replaced rather than written through. The work directory must be a real directory: if `.ws1tf-onboard-work` is a
symlink (a clone can carry one), `onboard` stops with `onboard working dir <dir> is a symlink or not a directory;
refusing to write through it`.

### 5.6 Final safety check: a clean `terraform plan`

After onboarding the types above into the same repo, this is a real, live `terraform plan` in the
onboard-work directory. The refresh pass shown covers eight resource types (listed after the transcript); the
`uem_smart_group` resources onboarded in Section 5.5 do not appear in it, so this transcript does not
demonstrate a clean plan for them:

```text
$ set -a; source /Users/alex/uem-terraform/prod/.env; set +a; terraform -chdir=/Users/alex/uem-terraform/prod/.ws1tf-onboard-work plan
uem_sensor_assignment.sensor_1: Refreshing state... [id=00000000-0000-0000-0000-000000000009]
uem_sensor_assignment.sensor_2: Refreshing state... [id=11111111-1111-1111-1111-111111111111]
uem_script_assignment.script-2: Refreshing state... [id=00000000-0000-0000-0000-000000000008]
uem_application_assignment.MacOSPkgStyleTestApp: Refreshing state... [id=00000000-0000-0000-0000-000000000006]
uem_mac_sensor.sensor_1: Refreshing state... [id=00000000-0000-0000-0000-000000000009]
uem_update_deployment.update-deployment-1: Refreshing state... [id=66666666-6666-6666-6666-666666666666]
uem_script_assignment.script-1: Refreshing state... [id=00000000-0000-0000-0000-000000000007]
uem_mac_application.MacOSDMGTestApp: Refreshing state...
uem_profile.Custom_Profile: Refreshing state... [id=12347]
uem_profile.Restrictions_Profile: Refreshing state... [id=12348]
uem_application_assignment.MacOSDMGTestApp: Refreshing state... [id=00000000-0000-0000-0000-000000000005]
uem_mac_script.script-1: Refreshing state... [id=00000000-0000-0000-0000-000000000007]
uem_mac_sensor.sensor_2: Refreshing state... [id=11111111-1111-1111-1111-111111111111]
uem_mac_application.MacOSPkgStyleTestApp: Refreshing state...
uem_mac_script.script-2: Refreshing state... [id=00000000-0000-0000-0000-000000000008]
uem_profile.windows__device_profile: Refreshing state... [id=12349]

No changes. Your infrastructure matches the configuration.

Terraform has compared your real infrastructure against your configuration and found no differences, so no changes are needed.
```

(The `Warning: Provider development overrides are in effect` banner Terraform itself always prints
under `dev_overrides` is omitted here — it's Terraform's own harness-mode notice, not part of the
clean-plan check, and is already shown once at the top of Section 5.5.) `No changes.` — each of the 8 types
refreshed above (`application_assignment`, `mac_application`, `mac_script`, `mac_sensor`, `profile`,
`script_assignment`, `sensor_assignment`, `update_deployment`) round-trips cleanly. That is 8 of the 9 types
that onboarded a resource in this tenant; the ninth, `smart_group`, is not shown in this pass.
`purchased_application_assignment` onboarded none, per its VPP skip behavior.

### 5.7 Menu ↔ flag parity (journey Step 7)

Every menu action is also a flag-driven command — nothing about `ws1-tf` requires a human at a
keyboard. (The block below is an illustration — the menu half and the command half are shown side by
side, and `...` marks elided lines — not one captured run.)

```text
# A human, driving the interactive menu:
$ ws1-tf --path /Users/alex/uem-terraform
This directory is already initialized.
Available actions:
  4. List onboardable types  (ws1-tf list)
  ...

# The exact same action, driven by flag — no menu, no prompts:
$ ws1-tf list
TYPE                              STATUS  DETAILS
profile                           ready   Device profiles and their payload settings.
...
```

A script — or an agent — can drive the entire flow non-interactively using the exact commands the
menu points at, and (per Section 5.3) the menu's own numbered selections now dispatch those same commands
directly rather than just naming them.

### 5.8 Reading the action log

Every `ws1-tf` run that does something writes a JSON Lines log (one JSON object per line) of everything it
actually did: every `terraform` command it ran, every call it made to the UEM API, and every file it wrote. This
is useful for two things — diagnosing "what exactly happened?" without asking someone to reproduce
the run, and manually replaying one step (e.g. re-running a single API call by hand to see its full
response).

**When it exists, and where it lives.** The log file is created when the run records its first action, not when
the command starts. A run that records nothing — `version`, `list`, a mistyped `--path`, a refusal that happens
before any work, `verify` in a folder with no tenant `.env` — leaves no `.ws1tf/` folder behind, creates no
directory for the mistyped path and prints no `action log:` line. Otherwise the run prints the log's path on
stderr as it exits (on a failed run, just before the `error:` line):

```text
action log: /Users/alex/uem-terraform/prod/.ws1tf/logs/20260925T215555Z-onboard.jsonl
```

It's one file per run, named by timestamp and command, under a tenant's own `.ws1tf/logs/` directory
— next to that tenant's `.env`, so it's easy to tell which credentials a run used. That applies to
`onboard`, `verify` and `refresh-local` (`list` makes no UEM call and records nothing, so it writes no log).
Other commands, and any of these when no tenant can be resolved, log under the repo root given by `--path` (the
current directory by default). It's gitignored, same as `.env` and `*.tfstate`, and is created readable
by you only (directory `0700`, file `0600`): never checked in, never shared unless you choose to.

**One exception, and it's called out in the log itself.** Driving `ws1-tf` through the interactive
numbered menu (Section 5.3) — rather than running a command like `ws1-tf onboard --type ...` directly —
writes its log to the outer repo directory instead of the specific tenant's directory. That's because the
log's directory is fixed before you've even picked a menu number, so `ws1-tf` doesn't yet know which
tenant (if any) the action will end up touching. To make this obvious rather than surprising, the very
first thing the menu logs, right when you pick a number, is a `menu_action` line naming what you picked and
(when it applies) which tenant directory it would otherwise have used:

```json
{"type":"menu_action","ts":"2026-09-25T21:55:40Z","action":"onboard","tenant_dir":"/Users/alex/uem-terraform/prod"}
```

**The record types.** Each line is one JSON object. The `type` field tells you which of these it is:

| `type` | What it means |
|---|---|
| `run_start` | Always the first line: the CLI version, the command you ran, a timestamp and **every** flag the command defines, whether you set it or not (an unset flag shows its default, often empty). A password/secret/token/key-shaped flag, and any flag that names or identifies a tenant object (`--name`, `--id`, `--uuid`, `--exclude-id`, `--exclude-uuid`, `--org-group`), is written as `"<redacted>"` even when empty, never the real value. |
| `tenant_context` | Written once a tenant's details are known: the UEM version detected, the auth method used, the Terraform provider source/version, and whether `dev_overrides` (a local development provider build) was active. |
| `terraform_cmd` | One real `terraform` subprocess the CLI ran: its exact command line, the directory it ran in, its exit code (`0` success; a `plan -detailed-exitcode` that found changes is logged as `2`, which is not a failure; `-1` when the failure was not a plain process exit), and how long it took. The `terraform version -json` check that Terraform's Go library runs lazily, just before the first command that needs the version, is logged as its own record (exit code `0`). |
| `uem_api_call` | One real call to the UEM REST API: the HTTP method, the request path **including its query string** (paging parameters, for example), the response status, and how long it took. For the OAuth token request only the escaped path of the token URL is recorded, never its host, query or fragment. |
| `file_written` | One file `ws1-tf` wrote to your repo (a generated `.tf` file, `.env`, etc.) — just the path, never its contents. |
| `menu_action` | Only from the interactive menu (see above): which action you picked, and its tenant directory when one applies. |
| `selection` | One finished picker: the item type and three counts — enumerated, left after any filter, and selected. Counts only; never item names or ids. |
| `refresh_drift` | One `refresh-local` check for one type: how many objects were checked, how many drifted, how many of them had at least one attribute applied (a count of objects, not of attributes) and how many attributes were expression-backed and left for review (a count of attributes; an expression is anything but a plain literal, so arithmetic, templates and `var.` references count). Counts only. |
| `import_skipped` | One object `onboard` skipped instead of importing: its address, import id, the reason (`invalid_payload_key`, `server_error`, `unsupported_payload` or `payload_check_unverified`) and, for an unsupported payload, which payloads. The console shows counts, and for a `server_error` skip it also names up to 8 of the objects. |
| `import_completeness` | One imported resource's completeness check: its address, the UEM fields it didn't capture, and its write-only secret fields. The console shows one summary for all of them. |
| `secrets_blank` | Every blank `secrets.json` key the run added, with each quoted object name in a key replaced by an ordinal (`uem_profile.Corp_WiFi.network_list[#1].password`), so this record does not name a tenant object (other records do, see below). The console reminder prints the full keys, at most 8 of them. |

The log holds no credentials: no request or response body, no HTTP header, no password/token/API key.
The instance host is always written as the literal text `${UEM_INSTANCE_URL}`, never your real UEM hostname,
and a line is reproducible once you `export UEM_INSTANCE_URL=...` (or, simpler, `source` the tenant's `.env`
as in Section 5.6). It does include local file paths, the `--path` you passed, the command lines of the
`terraform` runs (an `import` line carries the resource address, which is derived from the object's name, and
its id), the resource addresses in the `import_skipped` and `import_completeness` records, and UEM API request
paths and query strings, which can carry object ids — so read it through before you share it outside your team.

**Replaying a `terraform_cmd` line.** `cd` into `dir`, then run `argv` as printed:

```json
{"type":"terraform_cmd","ts":"2026-09-25T21:56:35Z","argv":["/opt/homebrew/bin/terraform","import","-no-color","-input=false","-lock-timeout=0s","-lock=true","uem_mac_script.Example_Script","00000000-0000-0000-0000-000000000007"],"dir":"/Users/alex/uem-terraform/prod/.ws1tf-onboard-work","exit_code":0,"duration_ms":715}
```

```text
cd /Users/alex/uem-terraform/prod/.ws1tf-onboard-work
terraform import -no-color -input=false -lock-timeout=0s -lock=true uem_mac_script.Example_Script 00000000-0000-0000-0000-000000000007
```

**Replaying a `uem_api_call` line.** It's a plain HTTPS request — `curl` it with `${UEM_INSTANCE_URL}` and
your own auth, the same way the CLI itself authenticates (Section 5.1's `.env`):

```json
{"type":"uem_api_call","ts":"2026-09-25T21:56:27Z","method":"GET","path":"/api/system/groups/12345","api_version":"1","status":200,"duration_ms":136}
```

```text
$ set -a; source /Users/alex/uem-terraform/prod/.env; set +a
$ curl --config - \
    -H "Accept: application/json;version=1" \
    "${UEM_INSTANCE_URL}/api/system/groups/12345" <<EOF
user = "$UEM_USERNAME:$UEM_PASSWORD"
header = "aw-tenant-code: $UEM_TENANT_CODE"
EOF
```

(Basic auth shown above since that's what this example tenant used — `tenant_context`'s `auth_method` line
tells you which one a given run used; for `oauth2`, fetch a bearer token from `UEM_OAUTH2_TOKEN_URL` first
and put `header = "Authorization: Bearer <token>"` in the config in place of the `user` line. The secrets
(credentials, tenant code and bearer token) go to `curl` through its config on stdin rather than as `-u` or
`-H` arguments, so they never appear in the process list; only the non-secret `Accept` header is an
argument. A value containing `"` or `\` needs escaping in a curl config file.)

**A full example**, redacted (real org-group ids/uuids swapped for the same placeholder values used
throughout this doc) — one `mac_script` onboarded end to end, from the moment `dev_overrides` was detected
through the clean post-import `terraform plan`. It is trimmed to representative lines (a real run has
more `uem_api_call` and `file_written` lines, and a `terraform_cmd` record for the lazy `terraform version -json`
check before the first Terraform command). The `run_start` line is written in the current format: it lists all
of `onboard`'s flags, with `--name` and `--org-group` redacted:

```jsonl
{"type":"run_start","ts":"2026-09-25T21:55:55Z","cli_version":"ws1-tf 26.2.0 (commit a1b2c3d4e, built 2026-09-25)","command":"onboard","flags":{"consumption-mode":"","help":"false","include-inherited":"false","name":"<redacted>","org-group":"<redacted>","path":"/Users/alex/uem-terraform","provider-zip":"","type":"mac_script"},"instance_url":"${UEM_INSTANCE_URL}"}
{"type":"uem_api_call","ts":"2026-09-25T21:56:09Z","method":"GET","path":"/api/system/info","api_version":"1","status":200,"duration_ms":285}
{"type":"tenant_context","ts":"2026-09-25T21:56:24Z","uem_version":"26.2.1100.0","auth_method":"basic","provider_source":"omnissa/uem","provider_version":"26.2.0","tf_cli_config":"/Users/alex/ws1tf-devbin/dev.tfrc","dev_overrides_active":true}
{"type":"terraform_cmd","ts":"2026-09-25T21:56:24Z","argv":["/opt/homebrew/bin/terraform","init","-no-color","-input=false","-backend=true","-get=true","-upgrade=false"],"dir":"/Users/alex/uem-terraform/prod/.ws1tf-onboard-work","exit_code":0,"duration_ms":113}
{"type":"terraform_cmd","ts":"2026-09-25T21:56:27Z","argv":["/opt/homebrew/bin/terraform","providers","schema","-json","-no-color"],"dir":"/Users/alex/uem-terraform/prod/.ws1tf-onboard-work","exit_code":0,"duration_ms":2461}
{"type":"uem_api_call","ts":"2026-09-25T21:56:27Z","method":"GET","path":"/api/system/groups/12345","api_version":"1","status":200,"duration_ms":136}
{"type":"terraform_cmd","ts":"2026-09-25T21:56:35Z","argv":["/opt/homebrew/bin/terraform","import","-no-color","-input=false","-lock-timeout=0s","-lock=true","uem_mac_script.Example_Script","00000000-0000-0000-0000-000000000007"],"dir":"/Users/alex/uem-terraform/prod/.ws1tf-onboard-work","exit_code":0,"duration_ms":715}
{"type":"uem_api_call","ts":"2026-09-25T21:56:36Z","method":"GET","path":"/api/system/groups/12345/children","api_version":"1","status":200,"duration_ms":128}
{"type":"file_written","ts":"2026-09-25T21:56:36Z","path":"/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/dependencies.tf"}
{"type":"file_written","ts":"2026-09-25T21:56:36Z","path":"/Users/alex/uem-terraform/prod/.ws1tf-onboard-work/mac_scripts.tf"}
{"type":"terraform_cmd","ts":"2026-09-25T21:56:38Z","argv":["/opt/homebrew/bin/terraform","plan","-no-color","-input=false","-detailed-exitcode","-lock-timeout=0s","-out=/tmp/ws1tf-plan-3019384068.tfplan","-lock=true","-parallelism=10","-refresh=true"],"dir":"/Users/alex/uem-terraform/prod/.ws1tf-onboard-work","exit_code":0,"duration_ms":1899}
```

The `terraform init` line in that example is the plain form (`-upgrade=false`), which is what runs when the work
directory's lock already matches the version in `provider.tf` or there is no lock. When `onboard` or
`refresh-local` re-selects an older lock (Section 4.3), and when a provider remedy runs, the same record shows
`-upgrade=true`.

### 5.9 `refresh-local`: fixing drift in place

Once you've onboarded something, UEM keeps moving — someone edits a profile in the console, a script's
push mode changes, an org group id gets renumbered. `ws1-tf refresh-local` finds that drift and, with your
confirmation, updates ONLY the drifted values in your `.tf` files — keeping the comments, the order of
the attributes, every other attribute's value and every dependency-closure reference
(`data.uem_organization_groups...`, `data.uem_smart_groups...`) as it was. A value it rewrites is replaced
whole, and whitespace may be re-aligned (both described below). It can also add an attribute the file
does not have yet, or remove one (see "An attribute UEM now reports as null" below). **It never writes to
UEM** — it only ever reads from UEM (via a `terraform plan -refresh-only`) and writes to your local
`.tf`/state files.

**Command forms:**

```bash
ws1-tf refresh-local                       # check every managed type
ws1-tf refresh-local profile               # check only profiles
ws1-tf refresh-local profile --id 12345    # check one specific profile
ws1-tf refresh-local profile --uuid <uuid> # check one specific profile, by uuid
ws1-tf refresh-local profile --exclude-id 12345   # check every profile EXCEPT this one
ws1-tf refresh-local mac_script --exclude-uuid <uuid>   # same, by uuid
ws1-tf refresh-local --force               # skip the prompt below, same as choosing option 1
```

**What you see** — one line per changed attribute, grouped by object (an illustrative sample of the
formats, assembled from the command's output rather than one captured run):

```text
Checked 3 mac_applications, 42 profiles against UEM. 2 changed.

  uem_profile.Restrictions_Profile
    description: "Old text" -> "Updated text"
    org_group_id: expression data.uem_organization_groups.og_12345.organization_groups[0].id -> UEM now reports 12399 (expression-backed, needs your review, not auto-applied)

  uem_mac_application.Escrow_Buddy
    push_mode: "Auto" -> "OnDemand"

left alone: uem_profile.Restrictions_Profile has plain-value changes but also changes refresh-local does not apply (org_group_id (expression)); applying only part of a resource would hide the rest from state while the .tf keeps the old value. Update the .tf by hand, then re-run.

1) Apply the 1 plain-value change(s) locally
2) Cancel -- no files changed
3) Save a report only -- no files changed
Choose (1-3):
```

**A resource with any change that isn't a plain value is left alone entirely.** In the sample,
`Restrictions_Profile` has a plain `description` change but also an expression-backed `org_group_id` change.
`refresh-local` does not write the `description` for it and does not refresh its state: a refresh-only apply
pulls *all* of a resource's drift into state, so applying only part of it would leave the `.tf` holding the old
value for the attributes it did not write, hide that drift from the next run, and let the next `terraform apply`
push the stale value back to UEM. It prints a `left alone:` line naming the resource and the changes that kept
it out (each as `attribute (class)`) and leaves the resource for you to update by hand. The number in option 1
counts only the plain-value changes of resources that are entirely plain (1 in the sample), and when no resource
qualifies there is no prompt at all.

**Not every change gets applied automatically**, and that's deliberate:

- **Expression-backed** — the `.tf` value you have today is anything other than a plain literal. The test is
  on how the value is written, not on whether Terraform could evaluate it. A plain literal is a quoted string
  or a heredoc with no interpolation, a number (a negative number such as `-1` counts), `true`, `false`,
  `null`, or a list or object made only of those. Everything else is an expression: a reference
  (`data.`, `local.`, `var.`), arithmetic or any other operator, a conditional, a function call, a `for`
  expression, a splat or an index, a value in parentheses, a string or heredoc with `${...}` or `%{...}`
  in it (even one wrapped around a literal, such as `"${"a"}"`; the escapes `$${` and `%%{` are literal text),
  and a list or object that holds any of these, including an object whose key is an expression such as
  `(var.k)`. That includes values that need no input to evaluate, like `60 * 5`. Overwriting an expression
  with UEM's raw reported value would silently turn a maintained, re-resolved value into a dead one — so it's
  shown, never touched. You decide by hand.
- **Computed-only attributes are left out entirely, at every depth.** An attribute the provider's schema
  marks as computed and not settable (a server-maintained value such as a record id or timestamp) can't be
  written into a `.tf` file — Terraform rejects a configured value for it — so a drift in one is not listed or
  applied. That includes one nested inside a list or an object (an id, a counter or a timestamp inside an
  assignment's element, for example): it is removed from the drift before anything is classified, shown or
  written, and an attribute whose only changed values are such server-maintained ones drops out of the list.
- **Instances and module resources are manual.** A resource that Terraform addresses with an index
  (`count`/`for_each`, for example `uem_profile.p["a"]`) or inside a module has no single block to
  rewrite, so its changes are listed as manual and are not counted in "Apply N change(s)".
- **Structural ("manual")** — a list whose length changed, at any depth inside the attribute. `refresh-local`
  never guesses how to re-pair elements positionally, so such a change is never auto-applied to an attribute
  that already exists in the `.tf`: one written as a plain literal is reported as manual (structural), an
  expression-backed or sensitive one keeps that class. That is a correct outcome for a genuinely ambiguous
  change, not a gap to close. The line names the list by its dotted path and shows that list's own lengths, for example
  `restrictions.rules: list changed (2 -> 3 elements), can't apply safely: ["a", "b", "c"] (manual, needs your review)`
  (for a list at the top of the attribute the path is just the attribute name, as in `assigned_smart_groups`).
  Any other changed value inside the same attribute gets its own `path: old -> new (manual, needs your
  review)` line, because the whole attribute is left for review. An attribute that is missing from the `.tf`
  is not held back: it is appended whole, even when its value is a list, because there is no existing list to
  pair elements with.
- **A Required attribute that UEM reports as null is manual.** Terraform rejects a null for an attribute the
  provider schema marks as required, so writing one would break the file. When the value to write holds a
  null for a required attribute, or an object in it leaves a required nested attribute out (at any depth,
  inside lists and objects too), the attribute is shown as manual with the reason
  `<path> is required by the provider schema but UEM now reports it as null, which Terraform rejects`, and is
  never written. `refresh-local` does not write `= null` for a required attribute.
- **Override files and JSON files are manual.** A resource that is defined or overridden in a Terraform
  override file (`override.tf` or `*_override.tf`, and their `.tf.json` forms) or in any `*.tf.json` file has no
  single literal in an editable file to rewrite, so a change that would otherwise be a plain-value change is
  listed as manual with the reason
  `defined or overridden in an override file or a .tf.json file, which refresh-local does not edit` (a sensitive
  attribute keeps its sensitive handling). It reads those files only to detect this and never edits them.
- **Sensitive** — an attribute is sensitive when **any** sensitive value sits anywhere inside it: either
  the provider schema marks an attribute in it sensitive (e.g. `client_secret`), or the refresh plan itself
  marks something in it sensitive (`before_sensitive`/`after_sensitive`, which also covers a value that is
  sensitive only because it came from a sensitive variable or another resource's sensitive output). The whole
  top-level attribute is then treated as sensitive, so a change that replaces an object or list holding one
  sensitive field is covered, not just the field itself. A sensitive attribute never shows its real value
  anywhere: not in this list, not in the saved report, not in the action log. You'll see
  `client_secret: (sensitive value changed)`, and it is manual: it is never applied automatically (the
  report lists it as `skipped (sensitive)`), because auto-applying would write the secret as plain text
  into a file you commit, and you couldn't review what was written. Update that value by hand.

**An attribute UEM now reports as null.** When an attribute that held a value is now null in UEM and it is not
a required one, `refresh-local` writes `attribute = null`. If the verification plan after the apply still shows
a difference only for attributes it wrote as null, it removes those attribute lines instead and prints
`note: <address>: writing null for <attributes> did not produce a clean plan; removed the attribute line(s)
instead`. An attribute that UEM reports but the `.tf` file does not have at all is appended to the end of
its block. So the file can gain or lose attribute lines, not only change values.

**Numbers** are compared and written at full precision: an id above 2^53 that changed by 1 is detected, shown
with all its digits and written with all its digits, and `1` and `1.0` are the same number, not a change.

Just before it writes, `refresh-local` re-checks that each attribute is still a plain literal (or still
absent) in the file as it is now, with the same rule as above, and skips any that you changed to an expression
in the meantime. A heredoc without `${...}` or `%{...}` counts as a literal and is applied (it is then written
as a quoted string). An attribute skipped this way is listed as
`skipped (not literal)` in the saved report, and, because refreshing only part of a resource would hide the rest
of its drift, the resource is then not refreshed in state: a `left alone: <address>: <attributes> is no longer a
plain literal in <file>; state was not refreshed for this resource so the drift stays visible` line says so.
`refresh-local` also refuses to write a file that wouldn't parse, refuses to replace a file whose owner write bit
is clear (a read-only file), and, if the file changed between its read and the write, writes nothing and tells
you to run `refresh-local` again. A symlinked `.tf` is followed rather than replaced. The write is atomic, the
file's mode is kept, and a file that uses CRLF line endings keeps them.

**Backups.** Before the first time a file is rewritten in a run, its original text is saved next to it as
`<file>.ws1tf-bak` (mode `0600`). The name never overwrites anything: if it is taken, `<file>.ws1tf-bak.1`,
`.2` and so on is used. It doesn't end in `.tf`, so Terraform ignores it, and for a symlinked `.tf` it sits next to
the file the link points to. If a later step fails (the refresh-only apply, or a verification plan that is not
clean), the error names the files this run already edited and their backups —
`files already edited by this run: <file> (original saved as <backup>)` — and nothing is rolled back: the edited
files stay as they are. When the refresh-only apply or the verification plan itself fails after `refresh-local`
wrote a `= null`, the error also names each resource and the attributes it wrote as null (Terraform can reject a
null where a value is needed) and tells you to remove that line or restore the original from the backup, then
run `refresh-local` again.

**A rewritten value is replaced whole.** `refresh-local` writes the new value of an attribute in place of the
old one in one piece, it does not edit inside it. A comment inside a rewritten multi-line object or list
literal is lost, and a heredoc that is rewritten becomes a quoted string. Comments above or beside the
attribute, in other attributes and in other blocks are kept.

**Formatting.** The edit goes through HCL's own formatter, which works on the whole file, as `terraform fmt`
does: comments, attribute order and every value outside the changed ones are kept, but whitespace and the
alignment of `=` signs can shift. For a file that is already `terraform fmt`-clean, which is what `ws1-tf`
generates, the rest of the file is byte-for-byte unchanged; a hand-edited file that isn't gets its
whitespace normalised along with the drifted values.

**Which files it reads.** It reads the `*.tf` and `*.tf.json` files in the tenant's `.ws1tf-onboard-work`
directory, skipping the files Terraform itself ignores: names starting with `.` (such as macOS `._main.tf`) or
`#`, and names ending in `~`. Only the plain `*.tf` files that are not override files are ever edited.

**It stops instead of guessing.** `refresh-local` aborts with an error, rather than reading the answer as
"every attribute was removed" or "nothing drifted", when it has no refresh plan or no provider schema to work
from (a missing schema would otherwise under-redact secrets), when a drift entry has no readable "after"
object and is not a removal, or when an attribute that changed is unknown in the plan so its UEM value cannot
be read. Attribute names, map keys and values that come from the plan are shown escaped, never raw (control
characters appear as visible `\xNN`-style escapes), in the terminal and in the saved report, and so are
resource addresses in both. In the report's command line, control characters in your flag values
become spaces and backticks become quotes, so a value cannot break out of the Markdown code span.

After option 1 (or `--force`), `refresh-local` also runs a `terraform apply -refresh-only` (so local
*state* matches too — this still never touches real UEM) and a verification `terraform plan`, to confirm
the objects it just touched now show no drift. If that verification isn't clean, it says so explicitly
(which object, which attribute) rather than silently continuing. A difference that is left carries the hint
`the object may have changed in UEM while the prompt was open; run refresh-local again`, because the apply
reads UEM again after the `.tf` was written from the values read earlier. A plan that cannot confirm an object (it is
missing from the verification plan, or shows a change with no visible difference) is reported as `could not
verify the plan is clean after apply`, not as clean.

**Two footer lines** you may also see, from a check that's independent of drift on already-tracked objects:

```text
3 object(s) exist in UEM but aren't managed here yet — run `ws1-tf onboard` to bring them in.
1 object(s) are missing in UEM: remove from .tf manually or re-create it — refresh-local never deletes.
```

The first means live UEM has objects this repo has never onboarded — `refresh-local` only tells you about
them; it doesn't onboard them itself. **That check covers apps only** (`mac_application` and
`application_assignment`, which share one enumeration), and only on a run that isn't narrowed with `--id` or
`--uuid`. For every other type, such as profiles, scripts and sensors, the check is not made, so the absence
of this line does not mean "nothing new in UEM". The check is scoped like `onboard`'s default: it counts only
apps owned by the org group recorded in the tenant's `README.md` when `init` ran (which is not necessarily the
org group you onboarded from), enumerated with the provider version pinned in the work directory's
`provider.tf`. When either is unknown — the README has no org-group scope, or `provider.tf` has no readable
pin — the check is skipped without a message. **Known limitation:** the org group is read from the README's
`- Org group scope:` line by plain line matching, not by parsing the Markdown, so a commented-out or fenced
copy of that line on a line of its own is read too (and when several well-formed lines match, the last one is used). If
the enumeration itself fails (an unreachable console, a provider that can't be resolved), the run carries on:
it prints `warning: skipped the new-in-console check: <reason>` and no footer. The second means a
`.tf`-declared, state-tracked object's own refresh indicates it no longer exists in real UEM —
`refresh-local` never removes it from your files or state for you (that would risk a later `apply` trying to
re-create it); you decide whether to delete the block or re-create the object.

Every run's full detail — every attribute's old/new value, which were applied vs. left for review, and
both footer sections — is saved to `<tenant-dir>/.ws1tf/reports/<timestamp>-refresh-local.md` any time you
choose option 1 or 3, or run with `--force` when at least one change qualifies (best-effort: a report-write
failure is a warning, never a failed command); nothing is saved on option 2, and when no change qualifies there
is no prompt and no report. A second run in the same second gets a `-1`, `-2`, … suffix instead of
overwriting the first report, and the reports directory and files are readable by you only.

**What `--id` and `--uuid` match.** The filters compare against what the Terraform state holds. `--id` and
`--exclude-id` match the resource's `id`. `--uuid` and `--exclude-uuid` match the object's uuid, which is the
resource `id` itself for scripts, sensors and update deployments, and the **parent object's** uuid for
assignments: `application_uuid` for application and purchased-application assignments, `script_uuid` for
script assignments and `sensor_uuid` for sensor assignments. For every other type it is the resource's `uuid`.
So one `--uuid` value selects (or `--exclude-uuid` drops) every assignment resource in the state that has that
parent. Uuids are compared case-insensitively, and so are `--id` and `--exclude-id` for scripts, sensors and
update deployments, whose `id` is a uuid.

**Filters that match nothing.** If the type/`--id`/`--uuid`/`--exclude-*` filters you pass leave nothing in the
Terraform state, `refresh-local` says `no matching resources: …; nothing was refreshed.` and does nothing,
rather than falling back to checking everything. A single `--id`, `--uuid`, `--exclude-id` or `--exclude-uuid`
value that matches no resource of the requested type (every type when none is given) is reported as
`warning: --<flag> <value> matched no resource in the Terraform state`. An unmatched `--exclude-id` or
`--exclude-uuid` stops a `--force` run (`... refusing to apply with --force, because the exclusion would
protect nothing; ...; nothing was refreshed`), since `--force` applies without a review and an exclusion that
protects nothing would let it apply to the very object you meant to leave out; without `--force` it stays a
warning. `--exclude-uuid` also refuses to run, with or without `--force`, when a resource of the requested type
has no uuid in the state, because that resource cannot be told apart from an excluded one. An empty or blank
value for `--id`, `--uuid`, `--exclude-id` or `--exclude-uuid` (for example `--uuid "$X"` with `X` unset) is
an error (`--uuid needs a non-empty value`), never a filter that matches objects without an id.

