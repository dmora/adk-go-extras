# adk-go-extras

Community storage backends for [Google ADK (Agent Development Kit)](https://google.golang.org/adk) for Go.

## Packages

### `artifact/sqlite` — SQLite-backed `artifact.Service`

A persistent, local `artifact.Service` implementation backed by SQLite via GORM. Designed for desktop apps, CLI tools, and local development where GCS is unnecessary.

```go
import "github.com/dmora/adk-go-extras/artifact/sqlite"

db, _ := gorm.Open(sqlitedriver.Open("artifacts.db"), &gorm.Config{})
service, _ := sqlite.NewService(db)

// Use with ADK runner:
r, _ := runner.New(runner.Config{
    ArtifactService: service,
    // ...
})
```

**Features:**
- Implements `artifact.Service` (Save, Load, Delete, List, Versions)
- Auto-versioning on save
- User-scoped artifacts (`user:` prefix)
- Binary artifact support (images, files via `genai.Part.InlineData`)
- Auto-migration via GORM
- CGo-free via `github.com/glebarez/sqlite`

## Status

Under development. Not yet published.

## License

Apache 2.0
