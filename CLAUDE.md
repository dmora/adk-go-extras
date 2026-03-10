# adk-go-extras — Project Guide

## What This Repo Is

`adk-go-extras` is a community module for reusable extensions to
[Google ADK (Agent Development Kit)](https://google.golang.org/adk) for Go.

The repository is meant to hold importable extras that plug into ADK Go
projects without bloating core ADK or mixing library code with large samples.

## Current Direction

The repo structure mirrors ADK Go's main extension points:

```text
adk-go-extras/
├── artifact/   # artifact.Service implementations
├── memory/     # memory.Service integrations
├── plugin/     # reusable ADK lifecycle plugins
├── session/    # session.Service implementations
├── tool/       # standalone tools and toolsets
├── examples/   # runnable examples that use the packages above
└── internal/   # repo-local helpers, primarily for tests
```

The first concrete package under development is `artifact/sqlite`.

## Module

```text
module github.com/dmora/adk-go-extras
go 1.24.4
```

Match the Go version expected by the targeted `google.golang.org/adk` release.

## Package Design Rules

- Mirror real ADK Go interfaces and hooks.
- Keep each integration in its own import path.
- Avoid root-level umbrella APIs.
- Keep examples separate from reusable packages.
- Prefer small packages over cross-cutting abstractions.

## Compatibility

Track compatibility explicitly against `google.golang.org/adk`.

The initial target is `v0.5.x`. Assume interface churn is possible while ADK Go
remains pre-v1.

## Current Focus: `artifact/sqlite`

`artifact/sqlite` is the first implemented package in the repo. It provides a
SQLite-backed `artifact.Service` for local persistent artifact storage.

When extending or maintaining that package:

- match ADK's artifact semantics instead of inventing new behavior
- use ADK's in-memory and cloud implementations as behavioral references
- keep the caller in control of the `*gorm.DB`
- keep the copied contract suite in `internal/testutil/artifacttest` aligned
  with upstream ADK expectations

## Verification

Use:

```bash
go test ./...
go test -race ./artifact/sqlite
go vet ./...
```
