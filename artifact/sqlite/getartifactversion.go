package sqlite

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"google.golang.org/adk/artifact"
	"gorm.io/gorm"
)

// GetArtifactVersion implements [artifact.Service] and returns the metadata for
// a specific version of an artifact. If req.Version is zero the latest version
// is used. Mirrors the in-memory service: only Version and MimeType are
// populated on the returned ArtifactVersion.
func (s *service) GetArtifactVersion(ctx context.Context, req *artifact.GetArtifactVersionRequest) (*artifact.GetArtifactVersionResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("request validation failed: %w", err)
	}

	query := s.artifactQuery(ctx, req.AppName, req.UserID, scopedSessionID(req.FileName, req.SessionID), req.FileName)
	if req.Version > 0 {
		query = query.Where("version = ?", req.Version)
	} else {
		query = query.Order("version DESC")
	}

	var row storageArtifact
	if err := query.Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("artifact not found: %w", fs.ErrNotExist)
		}
		return nil, fmt.Errorf("get artifact version: %w", err)
	}

	mimeType := "text/plain"
	if part, err := decodePart(row.Payload); err == nil && part.InlineData != nil {
		mimeType = part.InlineData.MIMEType
	}

	return &artifact.GetArtifactVersionResponse{
		ArtifactVersion: &artifact.ArtifactVersion{Version: row.Version, MimeType: mimeType},
	}, nil
}
