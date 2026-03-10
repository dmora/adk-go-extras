package sqlite

import (
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/adk/artifact"
	"google.golang.org/genai"

	"github.com/dmora/adk-go-extras/internal/testutil"
	"github.com/dmora/adk-go-extras/internal/testutil/artifacttest"
)

func TestNewServiceRequiresDB(t *testing.T) {
	if _, err := NewService(nil); err == nil {
		t.Fatal("NewService(nil) = nil error, want error")
	}
}

func TestContract(t *testing.T) {
	artifacttest.RunContractTests(t, "SQLite", func(t *testing.T) (artifact.Service, error) {
		return NewService(testutil.NewTestDB(t))
	})
}

func TestNewServiceReopenPersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "artifacts.db")

	db := testutil.OpenSQLiteDB(t, dbPath)
	srv, err := NewService(db)
	if err != nil {
		t.Fatalf("NewService() failed: %v", err)
	}

	ctx := t.Context()
	part := genai.NewPartFromText("persist me")
	if _, err := srv.Save(ctx, &artifact.SaveRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "session",
		FileName:  "persist.txt",
		Part:      part,
	}); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() failed: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("sqlDB.Close() failed: %v", err)
	}

	reopenedDB := testutil.OpenSQLiteDB(t, dbPath)
	reopened, err := NewService(reopenedDB)
	if err != nil {
		t.Fatalf("NewService(reopen) failed: %v", err)
	}

	got, err := reopened.Load(ctx, &artifact.LoadRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "session",
		FileName:  "persist.txt",
	})
	if err != nil {
		t.Fatalf("Load() failed after reopen: %v", err)
	}
	if diff := cmp.Diff(got.Part, part); diff != "" {
		t.Fatalf("Load() mismatch after reopen (-got +want):\n%s", diff)
	}
}

func TestVersionsDescending(t *testing.T) {
	srv := mustNewTestService(t)
	ctx := t.Context()

	for i := 1; i <= 3; i++ {
		if _, err := srv.Save(ctx, &artifact.SaveRequest{
			AppName:   "app",
			UserID:    "user",
			SessionID: "session",
			FileName:  "ordered.txt",
			Part:      genai.NewPartFromText(fmt.Sprintf("v%d", i)),
		}); err != nil {
			t.Fatalf("Save(%d) failed: %v", i, err)
		}
	}

	got, err := srv.Versions(ctx, &artifact.VersionsRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "session",
		FileName:  "ordered.txt",
	})
	if err != nil {
		t.Fatalf("Versions() failed: %v", err)
	}

	want := []int64{3, 2, 1}
	if diff := cmp.Diff(got.Versions, want); diff != "" {
		t.Fatalf("Versions() mismatch (-got +want):\n%s", diff)
	}
}

func TestSaveIgnoresExplicitVersion(t *testing.T) {
	srv := mustNewTestService(t)
	ctx := t.Context()

	got, err := srv.Save(ctx, &artifact.SaveRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "session",
		FileName:  "versioned.txt",
		Version:   99,
		Part:      genai.NewPartFromText("hello"),
	})
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}
	if got.Version != 1 {
		t.Fatalf("Save() version = %d, want 1", got.Version)
	}
}

func TestListEmptySessionReturnsEmptySlice(t *testing.T) {
	srv := mustNewTestService(t)

	got, err := srv.List(t.Context(), &artifact.ListRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "session",
	})
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if got.FileNames == nil {
		t.Fatal("List() returned nil slice, want non-nil empty slice")
	}
	if len(got.FileNames) != 0 {
		t.Fatalf("List() returned %v, want empty slice", got.FileNames)
	}
}

func TestJSONRoundTripPreservesPart(t *testing.T) {
	srv := mustNewTestService(t)
	ctx := t.Context()

	original := genai.NewPartFromBytes([]byte("payload"), "application/octet-stream")
	original.InlineData.DisplayName = "payload.bin"
	original.Thought = true
	original.ThoughtSignature = []byte{1, 2, 3}

	if _, err := srv.Save(ctx, &artifact.SaveRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "session",
		FileName:  "blob.bin",
		Part:      original,
	}); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	got, err := srv.Load(ctx, &artifact.LoadRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "session",
		FileName:  "blob.bin",
	})
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if diff := cmp.Diff(got.Part, original); diff != "" {
		t.Fatalf("Load() mismatch (-got +want):\n%s", diff)
	}
}

func TestNamespaceIsolation(t *testing.T) {
	srv := mustNewTestService(t)
	ctx := t.Context()

	for _, tc := range []struct {
		appName   string
		userID    string
		fileName  string
		partText  string
		sessionID string
	}{
		{appName: "app-a", userID: "user-a", sessionID: "session", fileName: "shared.txt", partText: "tenant-a"},
		{appName: "app-a", userID: "user-a", sessionID: "session", fileName: "only-a.txt", partText: "only-a"},
		{appName: "app-b", userID: "user-b", sessionID: "session", fileName: "shared.txt", partText: "tenant-b"},
		{appName: "app-b", userID: "user-b", sessionID: "session", fileName: "only-b.txt", partText: "only-b"},
	} {
		if _, err := srv.Save(ctx, &artifact.SaveRequest{
			AppName:   tc.appName,
			UserID:    tc.userID,
			SessionID: tc.sessionID,
			FileName:  tc.fileName,
			Part:      genai.NewPartFromText(tc.partText),
		}); err != nil {
			t.Fatalf("Save(%s/%s/%s) failed: %v", tc.appName, tc.userID, tc.fileName, err)
		}
	}

	gotList, err := srv.List(ctx, &artifact.ListRequest{
		AppName:   "app-a",
		UserID:    "user-a",
		SessionID: "session",
	})
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	wantList := []string{"only-a.txt", "shared.txt"}
	if diff := cmp.Diff(gotList.FileNames, wantList); diff != "" {
		t.Fatalf("List() mismatch (-got +want):\n%s", diff)
	}

	gotLoad, err := srv.Load(ctx, &artifact.LoadRequest{
		AppName:   "app-a",
		UserID:    "user-a",
		SessionID: "session",
		FileName:  "shared.txt",
	})
	if err != nil {
		t.Fatalf("Load(app-a/shared.txt) failed: %v", err)
	}
	wantPart := genai.NewPartFromText("tenant-a")
	if diff := cmp.Diff(gotLoad.Part, wantPart); diff != "" {
		t.Fatalf("Load(app-a/shared.txt) mismatch (-got +want):\n%s", diff)
	}
}

func TestFilenameValidation(t *testing.T) {
	srv := mustNewTestService(t)
	ctx := t.Context()

	for _, name := range []string{"path/sep.txt", "back\\slash.txt"} {
		_, err := srv.Save(ctx, &artifact.SaveRequest{
			AppName:   "app",
			UserID:    "user",
			SessionID: "session",
			FileName:  name,
			Part:      genai.NewPartFromText("data"),
		})
		if err == nil {
			t.Errorf("Save(%q) = nil error, want validation error for path separator", name)
		}
	}
}

func TestSaveAfterPartialDeletion(t *testing.T) {
	srv := mustNewTestService(t)
	ctx := t.Context()

	for i := 1; i <= 3; i++ {
		if _, err := srv.Save(ctx, &artifact.SaveRequest{
			AppName:   "app",
			UserID:    "user",
			SessionID: "session",
			FileName:  "file.txt",
			Part:      genai.NewPartFromText(fmt.Sprintf("v%d", i)),
		}); err != nil {
			t.Fatalf("Save(v%d) failed: %v", i, err)
		}
	}

	if err := srv.Delete(ctx, &artifact.DeleteRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "session",
		FileName:  "file.txt",
		Version:   3,
	}); err != nil {
		t.Fatalf("Delete(v3) failed: %v", err)
	}

	// After deleting v3, MAX(version) is 2, so the next version is 3 — matching
	// ADK's in-memory behavior of max(existing)+1, not a monotonic counter.
	got, err := srv.Save(ctx, &artifact.SaveRequest{
		AppName:   "app",
		UserID:    "user",
		SessionID: "session",
		FileName:  "file.txt",
		Part:      genai.NewPartFromText("v3-again"),
	})
	if err != nil {
		t.Fatalf("Save(after delete) failed: %v", err)
	}
	if got.Version != 3 {
		t.Fatalf("Save(after delete) version = %d, want 3", got.Version)
	}
}

func TestConcurrentSavesProduceContiguousVersions(t *testing.T) {
	srv := mustNewTestService(t)
	ctx := t.Context()

	const writers = 12
	versions := make([]int64, 0, writers)
	errs := make(chan error, writers)
	results := make(chan int64, writers)

	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := srv.Save(ctx, &artifact.SaveRequest{
				AppName:   "app",
				UserID:    "user",
				SessionID: "session",
				FileName:  "shared.txt",
				Part:      genai.NewPartFromText(fmt.Sprintf("payload-%d", i)),
			})
			if err != nil {
				errs <- err
				return
			}
			results <- resp.Version
		}()
	}

	wg.Wait()
	close(errs)
	close(results)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Save() failed: %v", err)
		}
	}

	for version := range results {
		versions = append(versions, version)
	}
	slices.Sort(versions)

	want := make([]int64, writers)
	for i := range want {
		want[i] = int64(i + 1)
	}
	if diff := cmp.Diff(versions, want); diff != "" {
		t.Fatalf("concurrent versions mismatch (-got +want):\n%s", diff)
	}
}

func mustNewTestService(t *testing.T) artifact.Service {
	t.Helper()
	srv, err := NewService(testutil.NewTestDB(t))
	if err != nil {
		t.Fatalf("NewService() failed: %v", err)
	}
	return srv
}
