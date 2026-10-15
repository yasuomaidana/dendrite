---
id: ADR-0002
ticket_id: DEN-058
title: "Adopt carla-sensor-rs for kinematics"
status: Accepted
date: 2026-10-15
tier_selected: "Tier 1: Source Reference"
package_name: "carla-sensor-rs"
source_uri: "https://github.com/cool-japan/carla-sensor-rs"
rationale: "Matched Tier 1 Source Reference 'carla-sensor-rs' registered via dendrite ref."
---

# ADR-0002: Adopt carla-sensor-rs for kinematics

## Context
Intent: 
Evaluated library reuse options for capability 'kinematics'.

## 3-Tier Library Reuse Evaluation
- **Capability Requested**: `kinematics`
- **Target Language**: `rust`
- **Tier 1 (Source References)**: Matched (Selected)
- **Tier 2 (Pool Members)**: No Match
- **Tier 3 (Ecosystem Registries)**: No Match
- **Selected Tier**: `Tier 1: Source Reference`

## Decision
Adopt `carla-sensor-rs` via Tier 1: Source Reference. Matched Tier 1 Source Reference 'carla-sensor-rs' registered via dendrite ref.

## Consequences
- Action: slice_and_import
- Package Spec: `carla-sensor-rs (ref: ref-carla-sensor)`
- Import Path: ``
