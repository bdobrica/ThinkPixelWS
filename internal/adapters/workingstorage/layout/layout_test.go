package layout

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain"
	"github.com/google/uuid"
)

func fixture(t *testing.T) (uuid.UUID, uuid.UUID, []domain.WorkspaceComponent) {
	t.Helper()
	tenant, workspace := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	components := make([]domain.WorkspaceComponent, 0, 4)
	for i, kind := range []domain.WorkspaceComponentKind{domain.WorkspaceComponentRepository, domain.WorkspaceComponentDirectory, domain.WorkspaceComponentDocumentCollection, domain.WorkspaceComponentArtifactCollection} {
		c, err := (domain.NewWorkspaceComponent{TenantID: tenant, WorkspaceID: workspace, ID: uuid.Must(uuid.NewV7()), Name: []string{"repo", "scratch", "docs", "artifacts"}[i], Kind: kind}).WorkspaceComponent(time.Now())
		if err != nil {
			t.Fatal(err)
		}
		components = append(components, c)
	}
	return tenant, workspace, components
}

func openRoot(t *testing.T, path string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return root
}

func TestPrepareLayoutSurvivesReopen(t *testing.T) {
	tenant, workspace, components := fixture(t)
	volume := t.TempDir()
	root := openRoot(t, volume)
	original := append([]domain.WorkspaceComponent(nil), components...)
	got, err := Prepare(context.Background(), root, tenant, workspace, components)
	if err != nil {
		t.Fatal(err)
	}
	want := []Component{
		{components[3].ID, "artifacts", "/workspace/artifacts"},
		{components[2].ID, "docs", "/workspace/docs"},
		{components[0].ID, "repo", "/workspace/repo"},
		{components[1].ID, "scratch", "/workspace/scratch"},
	}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(components, original) {
		t.Fatalf("layout = %#v, inputs = %#v", got, components)
	}
	for _, c := range got {
		if err := root.WriteFile(c.RelativePath+"/content", []byte(c.RelativePath), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := root.Mkdir("unselected", 0700); err != nil {
		t.Fatal(err)
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	root = openRoot(t, volume)
	components[0], components[3] = components[3], components[0]
	retry, err := Prepare(context.Background(), root, tenant, workspace, components)
	if err != nil || !reflect.DeepEqual(got, retry) {
		t.Fatalf("retry = %#v, %v", retry, err)
	}
	for _, c := range retry {
		content, err := root.ReadFile(c.RelativePath + "/content")
		if err != nil || string(content) != c.RelativePath {
			t.Fatalf("lost content for %s: %q, %v", c.RelativePath, content, err)
		}
	}
	if _, err := root.Stat("unselected"); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidRecordsDoNotCreateDirectories(t *testing.T) {
	for name, mutate := range map[string]func([]domain.WorkspaceComponent){
		"tenant":         func(c []domain.WorkspaceComponent) { c[3].TenantID = uuid.Must(uuid.NewV7()) },
		"workspace":      func(c []domain.WorkspaceComponent) { c[3].WorkspaceID = uuid.Must(uuid.NewV7()) },
		"duplicate ID":   func(c []domain.WorkspaceComponent) { c[3].ID = c[0].ID },
		"duplicate name": func(c []domain.WorkspaceComponent) { c[3].Name, c[3].CanonicalPath = c[0].Name, c[0].CanonicalPath },
		"traversal": func(c []domain.WorkspaceComponent) {
			c[3].Name, c[3].CanonicalPath = "../escape", "/workspace/../escape"
		},
		"absolute":           func(c []domain.WorkspaceComponent) { c[3].Name = "/escape" },
		"canonical mismatch": func(c []domain.WorkspaceComponent) { c[3].CanonicalPath = "/workspace/repo" },
	} {
		t.Run(name, func(t *testing.T) {
			tenant, workspace, components := fixture(t)
			mutate(components)
			volume := t.TempDir()
			if _, err := Prepare(context.Background(), openRoot(t, volume), tenant, workspace, components); err == nil {
				t.Fatal("accepted invalid records")
			}
			entries, err := os.ReadDir(volume)
			if err != nil || len(entries) != 0 {
				t.Fatalf("unexpected writes: %v, %v", entries, err)
			}
		})
	}
}

func TestPathConflictsDoNotModifyVolume(t *testing.T) {
	for _, kind := range []string{"file", "external symlink", "internal symlink", "dangling symlink"} {
		t.Run(kind, func(t *testing.T) {
			tenant, workspace, components := fixture(t)
			volume, outside := t.TempDir(), t.TempDir()
			root := openRoot(t, volume)
			var err error
			switch kind {
			case "file":
				err = root.WriteFile("scratch", []byte("keep"), 0600)
			case "external symlink":
				err = os.Symlink(outside, filepath.Join(volume, "scratch"))
			case "internal symlink":
				if err := root.Mkdir("repo", 0755); err != nil {
					t.Fatal(err)
				}
				err = root.Symlink("repo", "scratch")
			case "dangling symlink":
				err = root.Symlink("absent", "scratch")
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := root.Lstat("scratch")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Prepare(context.Background(), root, tenant, workspace, components); err == nil {
				t.Fatal("accepted conflicting path")
			}
			if _, err := root.Lstat("artifacts"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("created directory before conflict: %v", err)
			}
			after, err := root.Lstat("scratch")
			if err != nil || !os.SameFile(before, after) {
				t.Fatalf("replaced conflict: %v", err)
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatalf("escaped volume: %v, %v", entries, err)
			}
		})
	}
}

func TestEmptyAndCanceledLayout(t *testing.T) {
	tenant, workspace, components := fixture(t)
	root := openRoot(t, t.TempDir())
	got, err := Prepare(context.Background(), root, tenant, workspace, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty layout: %v, %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Prepare(ctx, root, tenant, workspace, components); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled prepare: %v", err)
	}
	if _, err := root.Lstat("artifacts"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled prepare wrote: %v", err)
	}
	if _, err := Prepare(context.Background(), nil, tenant, workspace, components); err == nil {
		t.Fatal("accepted nil root")
	}
}
