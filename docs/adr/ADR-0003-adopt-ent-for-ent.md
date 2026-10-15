---
id: ADR-0003
ticket_id: DEN-058
title: "Adopt ent for ent"
status: Accepted
date: 2026-10-15
tier_selected: "Tier 3: Ecosystem Registry"
package_name: "ent"
source_uri: "https://pkg.go.dev/entgo.io/ent"
rationale: "Mapped canonical ecosystem package 'ent' from public registry."
---

# ADR-0003: Adopt ent for ent

## Context
Intent: 
Evaluated library reuse options for capability 'ent'.

## 3-Tier Library Reuse Evaluation
- **Capability Requested**: `ent`
- **Target Language**: `go`
- **Tier 1 (Source References)**: No Match
- **Tier 2 (Pool Members)**: No Match
- **Tier 3 (Ecosystem Registries)**: Matched (Selected)
- **Selected Tier**: `Tier 3: Ecosystem Registry`

## Decision
Adopt `ent` via Tier 3: Ecosystem Registry. Mapped canonical ecosystem package 'ent' from public registry.

## Consequences
- Action: add_registry_dep
- Package Spec: `entgo.io/ent v0.14.0`
- Import Path: `import "entgo.io/ent"`
