// Package sqlite provides a SQLite-backed [artifact.Service] for Google ADK.
//
// This package implements the [artifact.Service] interface using GORM with a
// CGo-free SQLite driver, providing persistent local artifact storage for
// desktop apps, CLI tools, and local development.
package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"google.golang.org/adk/artifact"
	"google.golang.org/genai"
	"gorm.io/gorm"
)

const (
	artifactTableName   = "adk_extra_artifacts"
	userScopedSessionID = "user"
)

// Raw SQL is used here so version allocation stays atomic: one statement both
// computes the next version and inserts the row, avoiding a select-then-insert
// race under concurrent writers.
var insertArtifactSQL = fmt.Sprintf(`
INSERT INTO %s (
	app_name,
	user_id,
	session_id,
	file_name,
	version,
	payload
)
SELECT ?, ?, ?, ?, COALESCE(MAX(version), 0) + 1, ?
FROM %s
WHERE app_name = ? AND user_id = ? AND session_id = ? AND file_name = ?
RETURNING version
`, artifactTableName, artifactTableName)

// NewService creates a SQLite-backed artifact service using the caller-owned
// GORM database handle.
//
// The constructor auto-migrates the backing table. Callers remain responsible
// for database lifecycle, PRAGMA configuration, and logger configuration.
//
// For concurrent write-heavy usage in a single process, share one *gorm.DB,
// limit it to a single SQLite connection, and enable WAL mode before passing
// it to this constructor:
//
//	sqlDB, err := db.DB()
//	if err != nil {
//		return nil, err
//	}
//	sqlDB.SetMaxOpenConns(1)
//	sqlDB.SetMaxIdleConns(1)
//	db.Exec("PRAGMA journal_mode=WAL")
//
// To avoid logging artifact contents through SQL bind parameters, prefer a
// quiet or parameterized GORM logger on the provided DB.
func NewService(db *gorm.DB) (artifact.Service, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	if err := db.AutoMigrate(&storageArtifact{}); err != nil {
		return nil, fmt.Errorf("auto migrate sqlite artifact table: %w", err)
	}
	return &service{db: db}, nil
}

type service struct {
	db *gorm.DB
}

type storageArtifact struct {
	AppName   string `gorm:"column:app_name;primaryKey;not null"`
	UserID    string `gorm:"column:user_id;primaryKey;not null"`
	SessionID string `gorm:"column:session_id;primaryKey;not null"`
	FileName  string `gorm:"column:file_name;primaryKey;not null"`
	Version   int64  `gorm:"column:version;primaryKey;not null"`
	Payload   []byte `gorm:"column:payload;type:blob;not null"`
}

func (storageArtifact) TableName() string {
	return artifactTableName
}

type insertedVersion struct {
	Version int64
}

func (s *service) Save(ctx context.Context, req *artifact.SaveRequest) (*artifact.SaveResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	payload, err := json.Marshal(req.Part)
	if err != nil {
		return nil, fmt.Errorf("marshal artifact part: %w", err)
	}

	sessionID := scopedSessionID(req.FileName, req.SessionID)

	// Match ADK's in-memory service: explicit req.Version is ignored and a new
	// monotonically increasing version is always allocated.
	var inserted insertedVersion
	if err := s.db.WithContext(ctx).Raw(
		insertArtifactSQL,
		req.AppName,
		req.UserID,
		sessionID,
		req.FileName,
		payload,
		req.AppName,
		req.UserID,
		sessionID,
		req.FileName,
	).Scan(&inserted).Error; err != nil {
		return nil, fmt.Errorf("save artifact: %w", err)
	}
	if inserted.Version == 0 {
		return nil, fmt.Errorf(
			"save artifact %q for app %q user %q session %q: failed to allocate version",
			req.FileName,
			req.AppName,
			req.UserID,
			sessionID,
		)
	}

	return &artifact.SaveResponse{Version: inserted.Version}, nil
}

func (s *service) Load(ctx context.Context, req *artifact.LoadRequest) (*artifact.LoadResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	var row storageArtifact
	query := s.artifactQuery(ctx, req.AppName, req.UserID, scopedSessionID(req.FileName, req.SessionID), req.FileName)
	if req.Version > 0 {
		query = query.Where("version = ?", req.Version)
	} else {
		query = query.Order("version DESC")
	}

	if err := query.Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
		}
		return nil, fmt.Errorf("load artifact: %w", err)
	}

	part, err := decodePart(row.Payload)
	if err != nil {
		return nil, fmt.Errorf("decode artifact payload: %w", err)
	}
	return &artifact.LoadResponse{Part: part}, nil
}

func (s *service) Delete(ctx context.Context, req *artifact.DeleteRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("request validation failed: %w", err)
	}

	query := s.artifactQuery(ctx, req.AppName, req.UserID, scopedSessionID(req.FileName, req.SessionID), req.FileName)
	if req.Version > 0 {
		query = query.Where("version = ?", req.Version)
	}

	if err := query.Delete(&storageArtifact{}).Error; err != nil {
		return fmt.Errorf("delete artifact: %w", err)
	}
	return nil
}

func (s *service) List(ctx context.Context, req *artifact.ListRequest) (*artifact.ListResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	var fileNames []string
	if err := s.db.WithContext(ctx).
		Model(&storageArtifact{}).
		Distinct("file_name").
		Where(
			"app_name = ? AND user_id = ? AND session_id IN ?",
			req.AppName,
			req.UserID,
			[]string{req.SessionID, userScopedSessionID},
		).
		Order("file_name ASC").
		Pluck("file_name", &fileNames).Error; err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}

	if fileNames == nil {
		fileNames = []string{}
	}
	sort.Strings(fileNames)
	return &artifact.ListResponse{FileNames: fileNames}, nil
}

func (s *service) Versions(ctx context.Context, req *artifact.VersionsRequest) (*artifact.VersionsResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	var versions []int64
	if err := s.artifactQuery(ctx, req.AppName, req.UserID, scopedSessionID(req.FileName, req.SessionID), req.FileName).
		Order("version DESC").
		Pluck("version", &versions).Error; err != nil {
		return nil, fmt.Errorf("list artifact versions: %w", err)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
	}
	return &artifact.VersionsResponse{Versions: versions}, nil
}

func (s *service) artifactQuery(ctx context.Context, appName, userID, sessionID, fileName string) *gorm.DB {
	return s.db.WithContext(ctx).
		Model(&storageArtifact{}).
		Where(
			"app_name = ? AND user_id = ? AND session_id = ? AND file_name = ?",
			appName,
			userID,
			sessionID,
			fileName,
		)
}

func scopedSessionID(fileName, sessionID string) string {
	if strings.HasPrefix(fileName, "user:") {
		return userScopedSessionID
	}
	return sessionID
}

func decodePart(payload []byte) (*genai.Part, error) {
	var part genai.Part
	if err := json.Unmarshal(payload, &part); err != nil {
		return nil, err
	}
	return &part, nil
}

var _ artifact.Service = (*service)(nil)
