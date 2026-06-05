<!--
Sync Impact Report
- Version change: template placeholder -> 1.0.0
- Modified principles:
  - Template principle slot 1 -> I. Simplicity First
  - Template principle slot 2 -> II. Minimal Dependencies
  - Template principle slot 3 -> III. Go-Only Implementation
  - Template principle slot 4 -> IV. Verified Current Documentation
- Added sections:
  - Engineering Constraints
  - Delivery Workflow
- Removed sections:
  - Unused fifth principle placeholder section
- Templates requiring updates:
  - ✅ .specify/templates/plan-template.md
  - ✅ .specify/templates/spec-template.md
  - ✅ .specify/templates/tasks-template.md
  - ✅ AGENTS.md (reviewed, no update required)
  - ✅ .specify/templates/commands/*.md (directory not present, no update required)
- Follow-up TODOs:
  - None
-->
# goagent Constitution

## Core Principles

### I. Simplicity First
Every change MUST start from the simplest design that can satisfy the requirement.
New layers, abstractions, indirection, code generation, or configuration surface
area are forbidden unless the simpler alternative is documented and rejected for
a concrete requirement. Rationale: complexity compounds faster than features and
is the main source of maintenance cost.

### II. Minimal Dependencies
Imports and dependencies MUST be kept to the minimum necessary set. The Go
standard library is the default choice. Any external module MUST have a written
justification in the plan or spec explaining why the standard library or an
existing approved dependency is insufficient. Rationale: fewer dependencies
reduce supply-chain risk, upgrade overhead, and incidental complexity.

### III. Go-Only Implementation
All production code, automation, and repository-owned implementation artifacts
MUST be written in Go. TypeScript, Python, C++, and parallel implementations in
other languages are not allowed in this repository unless this constitution is
amended first. Rationale: a single implementation language keeps the toolchain,
review surface, and operational model coherent.

### IV. Verified Current Documentation
When a feature depends on an external library, framework, SDK, API, CLI tool,
or cloud service, the implementation plan and related work MUST be based on
current documentation retrieved through Context7 before coding begins. Plans,
specs, or reviews MUST record the consulted source when the external dependency
meaningfully shapes the design. Rationale: outdated API assumptions create
rework and hidden defects.

## Engineering Constraints

- Repository changes MUST preserve a Go-only toolchain.
- `go.mod` MUST stay lean; add a module only when its necessity is documented.
- New packages and directories MUST be justified by the feature shape, not
  created preemptively.
- Generated or vendored artifacts MUST not introduce a hidden dependency on
  TypeScript, Python, C++, or another second implementation stack.

## Delivery Workflow

- Every spec MUST identify the simplest viable design, any proposed dependency
  additions, and whether current external docs were required.
- Every plan MUST pass a constitution check covering simplicity, dependency
  minimization, Go-only scope, and Context7 verification for external tech.
- Every task list MUST use Go paths and include dependency-introduction work
  only when the spec or plan explicitly justifies it.
- Code review and self-review MUST reject changes that add unjustified
  abstraction, extra dependencies, or non-Go implementation paths.

## Governance

This constitution is the highest-priority project policy for `goagent`.
Specifications, plans, tasks, and implementation changes MUST comply with it.

Amendments MUST be made through an explicit constitution update that records the
reason, the dependent templates reviewed, and any migration impact on active
work.

Versioning policy for this constitution follows semantic versioning:
- MAJOR for incompatible principle removals or redefinitions.
- MINOR for new principles or materially expanded governance.
- PATCH for clarifications and wording-only refinements.

Compliance review is required at specification, planning, task generation, and
code review time. Any exception MUST be documented in the relevant artifact and
is invalid unless the constitution is amended first.

**Version**: 1.0.0 | **Ratified**: 2026-06-05 | **Last Amended**: 2026-06-05
