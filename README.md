# adk-go-extras

`adk-go-extras` is a community module for reusable extensions to
[Google ADK (Agent Development Kit)](https://google.golang.org/adk) for Go.

The goal is to keep `adk-go` core-focused while providing a separate home for
importable extras that ADK Go projects can plug in package-by-package.

## Scope

This repository is organized around ADK Go's real extension points:

- `artifact/...` for artifact storage backends
- `memory/...` for memory service integrations
- `plugin/...` for reusable lifecycle plugins
- `session/...` for session service backends
- `tool/...` for standalone tools and toolsets
- `examples/` for runnable integration examples

## Status

The first concrete package is now available:

### `artifact/sqlite`

A local, persistent SQLite-backed `artifact.Service` for desktop apps, CLI
tools, and development workflows.

```go
import (
	sqlitedriver "github.com/glebarez/sqlite"
	sqliteartifact "github.com/dmora/adk-go-extras/artifact/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

db, err := gorm.Open(sqlitedriver.Open("artifacts.db"), &gorm.Config{
	Logger: logger.Default.LogMode(logger.Silent),
})
if err != nil {
	// handle error
}

sqlDB, err := db.DB()
if err != nil {
	// handle error
}
sqlDB.SetMaxOpenConns(1)
sqlDB.SetMaxIdleConns(1)

if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
	// handle error
}

artifactService, err := sqliteartifact.NewService(db)
if err != nil {
	// handle error
}
```

Supported single-process configuration for concurrent artifact writes is one
shared `*gorm.DB` with `SetMaxOpenConns(1)` plus WAL mode. If you use a noisier
GORM logger, prefer parameterized or silent SQL logging so artifact payloads do
not end up in logs.

Current behavior:

- Implements `google.golang.org/adk/artifact.Service`
- Preserves ADK artifact semantics for versioning, user-scoped files, and
  not-found handling
- Stores `genai.Part` as JSON in SQLite
- Uses a caller-owned `*gorm.DB`
- Covered by a copied ADK contract suite plus SQLite-specific persistence and
  concurrency tests

## Design Principles

- Mirror ADK Go extension points instead of creating generic buckets.
- Keep each integration in its own importable package path.
- Keep runnable examples separate from library packages.
- Prefer small, focused packages over a large kitchen-sink module API.
- Track compatibility explicitly against `google.golang.org/adk`.

## Compatibility

The initial target is `google.golang.org/adk v0.5.x`.

Because ADK Go is still pre-v1, package compatibility should be considered
explicit rather than assumed.

## Contributing

Open an issue or design note before adding a large new integration. Small,
well-scoped extras are preferred over broad abstractions.

See [CONTRIBUTING.md](CONTRIBUTING.md) for repository conventions.

## License

Apache 2.0. See [LICENSE](LICENSE).
