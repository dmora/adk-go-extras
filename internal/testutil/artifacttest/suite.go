package artifacttest

// This suite started as a copy of
// google.golang.org/adk/internal/artifact/tests/service_suite.go from
// google.golang.org/adk v0.5.0 (commit 7f482224b22dd3e4b0a5a78df90a085c04836cb1).
//
// Keep local edits reviewed against upstream when updating the targeted ADK
// version so artifact backend behavior stays aligned.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/genai"

	"google.golang.org/adk/artifact"
)

// RunContractTests executes a copied version of ADK's artifact service suite
// against a caller-provided factory.
func RunContractTests(t *testing.T, name string, factory func(t *testing.T) (artifact.Service, error)) {
	t.Run(fmt.Sprintf("Test%sArtifactService", name), func(t *testing.T) {
		ctx := t.Context()
		srv, err := factory(t)
		if err != nil {
			t.Fatalf("Failed to set up service: %v", err)
		}
		testArtifactService(ctx, t, srv, name)
	})

	t.Run(fmt.Sprintf("Test%sArtifactService_Empty", name), func(t *testing.T) {
		ctx := t.Context()
		srv, err := factory(t)
		if err != nil {
			t.Fatalf("Failed to set up service: %v", err)
		}
		testArtifactServiceEmpty(ctx, t, srv, name)
	})

	t.Run(fmt.Sprintf("Test%sArtifactService_UserScoped", name), func(t *testing.T) {
		ctx := t.Context()
		srv, err := factory(t)
		if err != nil {
			t.Fatalf("Failed to set up service: %v", err)
		}
		testArtifactServiceUserScoped(ctx, t, srv, name)
	})
}

//nolint:gocognit,gocyclo,cyclop,funlen // Kept aligned with upstream ADK test suite structure.
func testArtifactService(ctx context.Context, t *testing.T, srv artifact.Service, testSuffix string) {
	appName := "testapp"
	userID := "testuser"
	sessionID := "testsession"

	testData := []struct {
		fileName string
		version  int64
		part     *genai.Part
	}{
		{"file1", 1, genai.NewPartFromBytes([]byte("file v1"), "text/plain")},
		{"file1", 2, genai.NewPartFromBytes([]byte("file v2"), "text/plain")},
		{"file1", 3, genai.NewPartFromBytes([]byte("file v3"), "text/plain")},
		{"file2", 1, genai.NewPartFromBytes([]byte("file v3"), "text/plain")},
		{"file3", 1, genai.NewPartFromText("file v1")},
	}

	for i, data := range testData {
		got, err := srv.Save(ctx, &artifact.SaveRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			FileName:  data.fileName,
			Part:      data.part,
		})
		if err != nil || got.Version != data.version {
			t.Errorf("[%d] Save() = (%v, %v), want (%v, nil)", i, got.Version, err, data.version)
		}
	}

	t.Run(fmt.Sprintf("Load_%s", testSuffix), func(t *testing.T) {
		for _, tc := range []struct {
			version int64
			want    *genai.Part
		}{
			{0, genai.NewPartFromBytes([]byte("file v3"), "text/plain")},
			{1, genai.NewPartFromBytes([]byte("file v1"), "text/plain")},
			{2, genai.NewPartFromBytes([]byte("file v2"), "text/plain")},
		} {
			got, err := srv.Load(ctx, &artifact.LoadRequest{
				AppName:   appName,
				UserID:    userID,
				SessionID: sessionID,
				FileName:  "file1",
				Version:   tc.version,
			})
			if err != nil || !cmp.Equal(got.Part, tc.want) {
				t.Errorf("Load(%v) = (%v, %v), want (%v, nil)", tc.version, got.Part, err, tc.want)
			}
		}
	})

	t.Run(fmt.Sprintf("List_%s", testSuffix), func(t *testing.T) {
		resp, err := srv.List(ctx, &artifact.ListRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		if err != nil {
			t.Fatalf("List() failed: %v", err)
		}
		got := append([]string(nil), resp.FileNames...)
		slices.Sort(got)
		want := []string{"file1", "file2", "file3"}
		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("List() mismatch (-got +want):\n%s", diff)
		}
	})

	t.Run(fmt.Sprintf("Versions_%s", testSuffix), func(t *testing.T) {
		resp, err := srv.Versions(ctx, &artifact.VersionsRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			FileName:  "file1",
		})
		if err != nil {
			t.Fatalf("Versions() failed: %v", err)
		}
		got := append([]int64(nil), resp.Versions...)
		want := []int64{3, 2, 1}
		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("Versions('file1') mismatch (-got +want):\n%s", diff)
		}
	})

	if err := srv.Delete(ctx, &artifact.DeleteRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
		FileName:  "file1",
		Version:   3,
	}); err != nil {
		t.Fatalf("Delete(file1@v3) failed: %v", err)
	}

	t.Run(fmt.Sprintf("LoadAfterDeleteVersion3_%s", testSuffix), func(t *testing.T) {
		resp, err := srv.Load(ctx, &artifact.LoadRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			FileName:  "file1",
		})
		if err != nil {
			t.Fatalf("Load('file1') failed: %v", err)
		}
		want := genai.NewPartFromBytes([]byte("file v2"), "text/plain")
		if diff := cmp.Diff(resp.Part, want); diff != "" {
			t.Fatalf("Load('file1') mismatch (-got +want):\n%s", diff)
		}
	})

	if err := srv.Delete(ctx, &artifact.DeleteRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
		FileName:  "file1",
	}); err != nil {
		t.Fatalf("Delete(file1) failed: %v", err)
	}

	t.Run(fmt.Sprintf("LoadAfterDelete_%s", testSuffix), func(t *testing.T) {
		got, err := srv.Load(ctx, &artifact.LoadRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			FileName:  "file1",
		})
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("Load('file1') = (%v, %v), want error(%v)", got, err, fs.ErrNotExist)
		}
	})

	t.Run(fmt.Sprintf("ListAfterDelete_%s", testSuffix), func(t *testing.T) {
		resp, err := srv.List(ctx, &artifact.ListRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		if err != nil {
			t.Fatalf("List() failed: %v", err)
		}
		got := append([]string(nil), resp.FileNames...)
		slices.Sort(got)
		want := []string{"file2", "file3"}
		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("List() mismatch (-got +want):\n%s", diff)
		}
	})

	t.Run(fmt.Sprintf("VersionsAfterDelete_%s", testSuffix), func(t *testing.T) {
		got, err := srv.Versions(ctx, &artifact.VersionsRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			FileName:  "file1",
		})
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("Versions('file1') = (%v, %v), want error(%v)", got, err, fs.ErrNotExist)
		}
	})
}

//nolint:gocognit,gocyclo,cyclop,funlen // Kept aligned with upstream ADK test suite structure.
func testArtifactServiceUserScoped(ctx context.Context, t *testing.T, srv artifact.Service, testSuffix string) {
	appName := "testapp"
	userID := "testuser"
	sessionID := "testsession"

	testData := []struct {
		fileName string
		version  int64
		part     *genai.Part
	}{
		{"user:file1", 1, genai.NewPartFromBytes([]byte("file v1"), "text/plain")},
		{"user:file1", 2, genai.NewPartFromBytes([]byte("file v2"), "text/plain")},
		{"user:file1", 3, genai.NewPartFromBytes([]byte("file v3"), "text/plain")},
		{"file2", 1, genai.NewPartFromBytes([]byte("file v3"), "text/plain")},
		{"user:file3", 1, genai.NewPartFromText("file v1")},
	}

	for i, data := range testData {
		got, err := srv.Save(ctx, &artifact.SaveRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			FileName:  data.fileName,
			Part:      data.part,
		})
		if err != nil || got.Version != data.version {
			t.Errorf("[%d] Save() = (%v, %v), want (%v, nil)", i, got.Version, err, data.version)
		}
	}

	t.Run(fmt.Sprintf("Load_%s", testSuffix), func(t *testing.T) {
		for _, tc := range []struct {
			version int64
			want    *genai.Part
		}{
			{0, genai.NewPartFromBytes([]byte("file v3"), "text/plain")},
			{1, genai.NewPartFromBytes([]byte("file v1"), "text/plain")},
			{2, genai.NewPartFromBytes([]byte("file v2"), "text/plain")},
		} {
			got, err := srv.Load(ctx, &artifact.LoadRequest{
				AppName:   appName,
				UserID:    userID,
				SessionID: "'user' should be used instead",
				FileName:  "user:file1",
				Version:   tc.version,
			})
			if err != nil || !cmp.Equal(got.Part, tc.want) {
				t.Errorf("Load(%v) = (%v, %v), want (%v, nil)", tc.version, got.Part, err, tc.want)
			}
		}
	})

	t.Run(fmt.Sprintf("List_%s", testSuffix), func(t *testing.T) {
		resp, err := srv.List(ctx, &artifact.ListRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		if err != nil {
			t.Fatalf("List() failed: %v", err)
		}
		got := append([]string(nil), resp.FileNames...)
		want := []string{"file2", "user:file1", "user:file3"}
		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("List() mismatch (-got +want):\n%s", diff)
		}
	})

	t.Run(fmt.Sprintf("Versions_%s", testSuffix), func(t *testing.T) {
		resp, err := srv.Versions(ctx, &artifact.VersionsRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			FileName:  "user:file1",
		})
		if err != nil {
			t.Fatalf("Versions() failed: %v", err)
		}
		got := append([]int64(nil), resp.Versions...)
		want := []int64{3, 2, 1}
		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("Versions('user:file1') mismatch (-got +want):\n%s", diff)
		}
	})

	if err := srv.Delete(ctx, &artifact.DeleteRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
		FileName:  "user:file1",
		Version:   3,
	}); err != nil {
		t.Fatalf("Delete(user:file1@v3) failed: %v", err)
	}

	t.Run(fmt.Sprintf("LoadAfterDeleteVersion3_%s", testSuffix), func(t *testing.T) {
		resp, err := srv.Load(ctx, &artifact.LoadRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			FileName:  "user:file1",
		})
		if err != nil {
			t.Fatalf("Load('user:file1') failed: %v", err)
		}
		want := genai.NewPartFromBytes([]byte("file v2"), "text/plain")
		if diff := cmp.Diff(resp.Part, want); diff != "" {
			t.Fatalf("Load('user:file1') mismatch (-got +want):\n%s", diff)
		}
	})

	if err := srv.Delete(ctx, &artifact.DeleteRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessionID,
		FileName:  "user:file1",
	}); err != nil {
		t.Fatalf("Delete(user:file1) failed: %v", err)
	}

	t.Run(fmt.Sprintf("LoadAfterDelete_%s", testSuffix), func(t *testing.T) {
		got, err := srv.Load(ctx, &artifact.LoadRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			FileName:  "user:file1",
		})
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("Load('user:file1') = (%v, %v), want error(%v)", got, err, fs.ErrNotExist)
		}
	})

	t.Run(fmt.Sprintf("ListAfterDelete_%s", testSuffix), func(t *testing.T) {
		resp, err := srv.List(ctx, &artifact.ListRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		if err != nil {
			t.Fatalf("List() failed: %v", err)
		}
		got := append([]string(nil), resp.FileNames...)
		slices.Sort(got)
		want := []string{"file2", "user:file3"}
		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("List() mismatch (-got +want):\n%s", diff)
		}
	})

	t.Run(fmt.Sprintf("VersionsAfterDelete_%s", testSuffix), func(t *testing.T) {
		got, err := srv.Versions(ctx, &artifact.VersionsRequest{
			AppName:   appName,
			UserID:    userID,
			SessionID: sessionID,
			FileName:  "user:file1",
		})
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("Versions('user:file1') = (%v, %v), want error(%v)", got, err, fs.ErrNotExist)
		}
	})
}

func testArtifactServiceEmpty(ctx context.Context, t *testing.T, srv artifact.Service, testSuffix string) {
	t.Run(fmt.Sprintf("Load_%s", testSuffix), func(t *testing.T) {
		got, err := srv.Load(ctx, &artifact.LoadRequest{
			AppName: "app", UserID: "user", SessionID: "session", FileName: "file",
		})
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("Load() = (%v, %v), want error(%v)", got, err, fs.ErrNotExist)
		}
	})

	t.Run(fmt.Sprintf("List_%s", testSuffix), func(t *testing.T) {
		if _, err := srv.List(ctx, &artifact.ListRequest{
			AppName: "app", UserID: "user", SessionID: "session",
		}); err != nil {
			t.Fatalf("List() failed: %v", err)
		}
	})

	t.Run(fmt.Sprintf("Delete_%s", testSuffix), func(t *testing.T) {
		if err := srv.Delete(ctx, &artifact.DeleteRequest{
			AppName: "app", UserID: "user", SessionID: "session", FileName: "file1",
		}); err != nil {
			t.Fatalf("Delete() failed: %v", err)
		}
	})

	t.Run(fmt.Sprintf("Versions_%s", testSuffix), func(t *testing.T) {
		got, err := srv.Versions(ctx, &artifact.VersionsRequest{
			AppName: "app", UserID: "user", SessionID: "session", FileName: "file1",
		})
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("Versions() = (%v, %v), want error(%v)", got, err, fs.ErrNotExist)
		}
	})
}
