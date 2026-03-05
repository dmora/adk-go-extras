# adk-go-extras — Project Guide

## What Is This

`adk-go-extras` provides community storage backends for [Google ADK (Agent Development Kit)](https://google.golang.org/adk) for Go. The first package is a SQLite-backed `artifact.Service` implementation.

ADK ships with two artifact backends: `InMemoryService` (ephemeral) and `gcsartifact` (Google Cloud Storage). There is no local persistent backend. This project fills that gap.

## Module

```
module github.com/dmora/adk-go-extras
go 1.24.4
```

Match ADK's Go version requirement (`google.golang.org/adk@v0.5.0` requires Go 1.24.4).

## Architecture

```
adk-go-extras/
├── artifact/
│   └── sqlite/
│       ├── service.go       # artifact.Service implementation
│       └── service_test.go  # tests against the interface contract
├── go.mod
├── go.sum
├── CLAUDE.md
└── README.md
```

One package. One interface. One GORM model.

## The Interface to Implement

From `google.golang.org/adk@v0.5.0/artifact/service.go`:

```go
type Service interface {
    Save(ctx context.Context, req *SaveRequest) (*SaveResponse, error)
    Load(ctx context.Context, req *LoadRequest) (*LoadResponse, error)
    Delete(ctx context.Context, req *DeleteRequest) error
    List(ctx context.Context, req *ListRequest) (*ListResponse, error)
    Versions(ctx context.Context, req *VersionsRequest) (*VersionsResponse, error)
}
```

### Request/Response Types

| Type | Required Fields | Optional Fields |
|------|----------------|-----------------|
| `SaveRequest` | `AppName`, `UserID`, `SessionID`, `FileName`, `Part *genai.Part` | `Version int64` (if unset, auto-increments) |
| `SaveResponse` | `Version int64` | — |
| `LoadRequest` | `AppName`, `UserID`, `SessionID`, `FileName` | `Version int64` (0 = latest) |
| `LoadResponse` | `Part *genai.Part` | — |
| `DeleteRequest` | `AppName`, `UserID`, `SessionID`, `FileName` | `Version int64` (0 = delete all versions) |
| `ListRequest` | `AppName`, `UserID`, `SessionID` | — |
| `ListResponse` | `FileNames []string` | — |
| `VersionsRequest` | `AppName`, `UserID`, `SessionID`, `FileName` | — |
| `VersionsResponse` | `Versions []int64` | — |

### Key Semantics (from the InMemory reference implementation)

1. **Versioning**: Versions start at 1 and auto-increment. Each `Save()` creates version = max_existing + 1.
2. **User-scoped artifacts**: Filenames prefixed with `"user:"` are shared across all sessions for a given `(AppName, UserID)` pair. Internally, replace `SessionID` with the literal string `"user"` for storage.
3. **Filename validation**: Filenames cannot contain `/` or `\`. The `SaveRequest.Validate()` method checks this — call it before processing.
4. **Load version=0**: Returns the latest version (highest version number).
5. **Delete version=0**: Deletes ALL versions of that artifact.
6. **Delete non-existing**: Not an error (no-op).
7. **Load/Versions not found**: Return `fmt.Errorf("artifact not found: %w", fs.ErrNotExist)`.
8. **Part contents**: `genai.Part` has either `.Text` (string) or `.InlineData` (struct with `MIMEType string` and `Data []byte`). Both must be supported.
9. **List** includes both session-scoped AND user-scoped artifacts for the given `(AppName, UserID)`.
10. **Versions** returns versions in descending order (newest first) — matching the InMemory implementation's `ordered.Rev(Version)` key encoding.

### Upcoming Breaking Change

[PR #575](https://github.com/google/adk-go/pull/575) may add a 6th method `GetArtifactVersion` to the interface. Not merged yet as of v0.5.0. Monitor but don't implement proactively.

## GORM Model

```go
type StorageArtifact struct {
    AppName   string `gorm:"primaryKey;column:app_name"`
    UserID    string `gorm:"primaryKey;column:user_id"`
    SessionID string `gorm:"primaryKey;column:session_id"`
    FileName  string `gorm:"primaryKey;column:file_name"`
    Version   int64  `gorm:"primaryKey;column:version"`
    Data      []byte `gorm:"column:data;type:blob"`      // JSON-serialized genai.Part
    CreatedAt int64  `gorm:"column:created_at;autoCreateTime"`
}
```

### Why JSON-serialize `genai.Part`

`genai.Part` is a struct with optional fields (`Text`, `InlineData`, `FileData`, `FunctionCall`, etc.). Rather than mapping each field to a column, serialize the whole Part as JSON into a single `Data` blob column. This is future-proof — if `genai.Part` gains new fields, no migration needed.

Use `encoding/json` — `genai.Part` supports standard JSON marshaling.

## Dependencies

```
google.golang.org/adk v0.5.0        # artifact.Service interface
google.golang.org/genai              # genai.Part type (transitive via adk)
gorm.io/gorm                         # ORM
github.com/glebarez/sqlite           # CGo-free SQLite driver for GORM
```

Use `github.com/glebarez/sqlite` — same driver Foundry uses. CGo-free, pure Go, works everywhere.

## Constructor

```go
func NewService(db *gorm.DB) (artifact.Service, error)
```

- Accept a `*gorm.DB` (caller controls the database file/connection)
- Run `db.AutoMigrate(&StorageArtifact{})` in the constructor
- Return the service ready to use

This lets the consumer share a DB connection (e.g., Foundry's `foundry-adk.db`) or use a dedicated file.

## Reference Implementations

Study these before writing code:

1. **ADK InMemory** (canonical behavior to match):
   - Located at: `~/go/pkg/mod/google.golang.org/adk@v0.5.0/artifact/inmemory.go`
   - 289 lines. Uses `rsc.io/omap` with ordered composite keys.
   - Key patterns: user-scope handling, version auto-increment, scan ranges, descending version order.

2. **achetronic/adk-utils-go Filesystem** (community Go reference):
   - GitHub: https://github.com/achetronic/adk-utils-go
   - Filesystem-based, JSON-serialized `genai.Part` per version file.
   - Same interface, same patterns, just filesystem I/O instead of ordered map.

3. **ADK GCS** (production reference):
   - Located at: `~/go/pkg/mod/google.golang.org/adk@v0.5.0/artifact/gcsartifact/service.go`
   - Blob path: `{appName}/{userID}/{sessionID}/{fileName}/{version}`
   - Shows content-type handling and parallel deletion.

## Testing

Write tests that verify behavior matches the InMemory implementation:

```go
func TestSaveAndLoad(t *testing.T)           // basic save then load
func TestAutoVersioning(t *testing.T)        // multiple saves increment version
func TestLoadLatest(t *testing.T)            // version=0 returns newest
func TestLoadSpecificVersion(t *testing.T)   // version=N returns that version
func TestDeleteSpecificVersion(t *testing.T) // deletes one version, others remain
func TestDeleteAllVersions(t *testing.T)     // version=0 deletes all
func TestDeleteNonExisting(t *testing.T)     // no error
func TestLoadNotFound(t *testing.T)          // returns fs.ErrNotExist
func TestVersionsNotFound(t *testing.T)      // returns fs.ErrNotExist
func TestList(t *testing.T)                  // lists filenames (deduplicated)
func TestUserScopedArtifacts(t *testing.T)   // "user:" prefix → shared across sessions
func TestListIncludesUserScoped(t *testing.T)// List returns both session + user artifacts
func TestBinaryArtifact(t *testing.T)        // InlineData with MIMEType + bytes
func TestFilenameValidation(t *testing.T)    // rejects / and \ in filenames
func TestConcurrentAccess(t *testing.T)      // parallel saves/loads don't corrupt
```

Use `t.TempDir()` for the SQLite database file in tests.

## Build & Development

```bash
go mod init github.com/dmora/adk-go-extras
go mod tidy
go build ./...
go test ./...
go vet ./...
```

## Design Principles

- **Match InMemory behavior exactly** — this is a drop-in replacement, not a reinterpretation
- **No extra features** — don't add metadata columns, search, or anything not in the interface
- **Minimal dependencies** — only GORM, the SQLite driver, and ADK
- **Let the caller own the DB** — accept `*gorm.DB`, don't create connections internally
- **JSON blob for Part** — future-proof, no migrations when genai.Part evolves

## Consumer: Foundry

The primary consumer is [Foundry](https://github.com/dmora/foundry), an industrial FUI terminal app that orchestrates autonomous software development workflows via Google ADK. Foundry uses this to persist artifacts (blueprints, specs, verdicts, patches, reports) flowing between pipeline stations.

Foundry already has `foundry-adk.db` (GORM + `glebarez/sqlite`) for ADK session data. The artifact service will share this database:

```go
// In Foundry's app setup:
import sqliteartifact "github.com/dmora/adk-go-extras/artifact/sqlite"

artifactService, err := sqliteartifact.NewService(adkDB) // same *gorm.DB
```

Then wire into the ADK runner:

```go
// In agent.go, when building the runner:
r, err := runner.New(runner.Config{
    ArtifactService: artifactService,
    // ... existing config ...
})
```
