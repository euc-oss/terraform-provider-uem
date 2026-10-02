# ws1-tf CLI — Documentation

`ws1-tf` is a guided command-line tool, a companion to the Omnissa Workspace ONE UEM Terraform
provider (`omnissa/uem`), that brings an existing Workspace ONE UEM environment into Terraform. It
verifies your tenant credentials against the real console, scaffolds a ready-to-use repo, and
generates and imports Terraform code for your existing objects, checked for a clean
`terraform plan`. This documentation targets UEM 26.2 and provider v26.2.0-beta.1.

- **[`getting-started.md`](./getting-started.md)** — the hands-on guide: installing the CLI, how it
  finds and installs the `omnissa/uem` provider, and every command (`init`, `verify`, `list`,
  `onboard`, `refresh-local`) with real transcripts. It covers all 10 onboardable types, the
  dependency closure (lookups, managed smart groups, fixed values), the skip behaviours, the
  pickers, and the per-run action log.
- **[`onboarding-journey.md`](./onboarding-journey.md)**: a step-by-step walk through your
  onboarding journey, from installing the CLI to onboarding your first resource, with the terminal
  output you see at each step.
