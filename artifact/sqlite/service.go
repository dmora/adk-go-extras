// Package sqlite provides a SQLite-backed [artifact.Service] for Google ADK.
//
// This package implements the [artifact.Service] interface using GORM with a
// CGo-free SQLite driver, providing persistent local artifact storage for
// desktop apps, CLI tools, and local development.
package sqlite
