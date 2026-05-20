# contract-tests

Reserved for the Go ↔ Rust client contract test described in the GUI design
plan.

Goal: spin up a fresh `cloakd`, walk a deterministic script (init → unlock →
create one secret of each type → open endpoints → list → close → lock) through
both `pkg/client` (Go) and the Rust port at
`apps/cloak-gui/src-tauri/src/client/`, and diff every response shape.
Mismatches fail CI before the GUI ships with drift.

Implementation is a Phase 0 follow-up; the placeholder lives here so the
project layout matches the design doc and the CI workflow has a target to
wire up.
