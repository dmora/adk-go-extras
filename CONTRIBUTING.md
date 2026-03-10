# Contributing

`adk-go-extras` is intended to host focused, importable extensions for
`google.golang.org/adk`.

## What Belongs Here

- Artifact storage backends
- Memory service integrations
- Session service backends
- Reusable ADK plugins
- Standalone tools and toolsets
- Small runnable examples that demonstrate those packages

## What Does Not Belong Here

- Changes to ADK Go core behavior that should live in `google/adk-go`
- Large end-to-end sample applications better suited to a samples repository
- Repo-wide abstractions added before two or more packages actually need them

## Package Layout

Add new integrations under the extension point they implement:

- `artifact/<name>`
- `memory/<name>`
- `plugin/<name>`
- `session/<name>`
- `tool/<name>`

Keep examples under `examples/`.

## Conventions

- Prefer one concrete integration per package.
- Document ADK Go compatibility in package docs or the README when relevant.
- Add tests with each non-trivial package.
- Keep external dependencies minimal and justified.
- Avoid adding umbrella APIs at the module root.

## Before Opening a Large PR

For a new backend or plugin with non-trivial API or lifecycle behavior:

- open an issue or short design note first
- state which ADK Go interface or hook it targets
- explain why it belongs in extras instead of core or samples

## Verification

Before submitting changes, run:

```bash
make qa          # Full quality gate (lint + test + vet + vulncheck)
make check       # Fast check (lint + test)
make cover       # Test with coverage report
```
