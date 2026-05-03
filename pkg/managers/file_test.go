package managers

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/itunified-io/linuxctl/pkg/config"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

const arEmptyB64 = "ITxhcmNoPgo=" // !<arch>\n

func TestFileManager_Plan_CreateMissingFile(t *testing.T) {
	mock := newFileMock()
	m := NewFileManager().WithSession(mock)
	files := []config.FileSpec{{
		Path:       "/usr/lib64/libpthread_nonshared.a",
		Mode:       "0644",
		Owner:      "root",
		Group:      "root",
		ContentB64: arEmptyB64,
	}}
	changes, err := m.Plan(context.Background(), Spec(files), nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Action != "create" {
		t.Errorf("Action = %q, want create", changes[0].Action)
	}
}

func TestFileManager_Plan_IdempotentSameBody(t *testing.T) {
	body := "!<arch>\n"
	mock := newFileMock().withFile("/x/y", body)
	m := NewFileManager().WithSession(mock)
	files := []config.FileSpec{{Path: "/x/y", ContentB64: b64(body)}}
	changes, err := m.Plan(context.Background(), Spec(files), nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes (same body), got %d", len(changes))
	}
}

func TestFileManager_Plan_DriftDetected_WhenBodyDiffers(t *testing.T) {
	mock := newFileMock().withFile("/x/y", "old content")
	m := NewFileManager().WithSession(mock)
	files := []config.FileSpec{{Path: "/x/y", ContentB64: b64("new content")}}
	changes, err := m.Plan(context.Background(), Spec(files), nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 1 || changes[0].Action != "update" {
		t.Fatalf("expected 1 update change, got %+v", changes)
	}
}

func TestFileManager_Plan_CreateOnly_PreservesExisting(t *testing.T) {
	mock := newFileMock().withFile("/x/y", "user wrote this")
	m := NewFileManager().WithSession(mock)
	files := []config.FileSpec{{
		Path:       "/x/y",
		ContentB64: b64("stub content"),
		CreateOnly: true,
	}}
	changes, err := m.Plan(context.Background(), Spec(files), nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("CreateOnly + existing file should yield 0 changes, got %d", len(changes))
	}
}

func TestFileManager_Plan_CreateOnly_StillCreatesIfMissing(t *testing.T) {
	mock := newFileMock()
	m := NewFileManager().WithSession(mock)
	files := []config.FileSpec{{
		Path:       "/x/y",
		ContentB64: arEmptyB64,
		CreateOnly: true,
	}}
	changes, err := m.Plan(context.Background(), Spec(files), nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(changes) != 1 || changes[0].Action != "create" {
		t.Errorf("CreateOnly + missing file should create, got %+v", changes)
	}
}

func TestFileManager_Plan_InvalidContentB64_Errors(t *testing.T) {
	mock := newFileMock()
	m := NewFileManager().WithSession(mock)
	files := []config.FileSpec{{Path: "/x/y", ContentB64: "!!! not base64 !!!"}}
	_, err := m.Plan(context.Background(), Spec(files), nil)
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestFileManager_Apply_WritesAndChowns(t *testing.T) {
	mock := newFileMock().on("chown", "", nil)
	m := NewFileManager().WithSession(mock)
	files := []config.FileSpec{{
		Path:       "/usr/lib64/libpthread_nonshared.a",
		Mode:       "0644",
		Owner:      "root",
		Group:      "root",
		ContentB64: arEmptyB64,
	}}
	changes, err := m.Plan(context.Background(), Spec(files), nil)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	res, err := m.Apply(context.Background(), changes, false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Applied) != 1 {
		t.Errorf("Applied = %d, want 1", len(res.Applied))
	}
	written, ok := mock.writes["/usr/lib64/libpthread_nonshared.a"]
	if !ok {
		t.Fatal("file was not written")
	}
	want := "!<arch>\n"
	if string(written) != want {
		t.Errorf("body = %q, want %q", string(written), want)
	}
	if !mock.ran("chown 'root:root' '/usr/lib64/libpthread_nonshared.a'") {
		t.Errorf("expected chown call, cmds = %v", mock.cmds)
	}
}

func TestFileManager_Apply_DryRun_NoWrites(t *testing.T) {
	mock := newFileMock()
	m := NewFileManager().WithSession(mock)
	files := []config.FileSpec{{Path: "/x/y", ContentB64: arEmptyB64}}
	changes, _ := m.Plan(context.Background(), Spec(files), nil)
	res, err := m.Apply(context.Background(), changes, true)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Skipped) != 1 {
		t.Errorf("Skipped = %d, want 1", len(res.Skipped))
	}
	if _, wrote := mock.writes["/x/y"]; wrote {
		t.Error("dry-run wrote a file")
	}
}

func TestFileManager_Verify_OK_WhenNoDrift(t *testing.T) {
	body := "!<arch>\n"
	mock := newFileMock().withFile("/x/y", body)
	m := NewFileManager().WithSession(mock)
	res, err := m.Verify(context.Background(), Spec([]config.FileSpec{{Path: "/x/y", ContentB64: b64(body)}}))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK {
		t.Errorf("Verify OK = false, want true; drift = %+v", res.Drift)
	}
}

func TestFileManager_Rollback_RemovesCreatedFile(t *testing.T) {
	mock := newFileMock().on("rm -f", "", nil)
	m := NewFileManager().WithSession(mock)
	files := []config.FileSpec{{Path: "/x/y", ContentB64: arEmptyB64}}
	changes, _ := m.Plan(context.Background(), Spec(files), nil)
	if err := m.Rollback(context.Background(), changes); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if !mock.ran("rm -f '/x/y'") {
		t.Errorf("expected rm -f call, cmds = %v", mock.cmds)
	}
}

func TestFileManager_Rollback_RestoresPreviousBody(t *testing.T) {
	mock := newFileMock().withFile("/x/y", "previous content")
	m := NewFileManager().WithSession(mock)
	files := []config.FileSpec{{Path: "/x/y", ContentB64: b64("new content")}}
	changes, _ := m.Plan(context.Background(), Spec(files), nil)
	if err := m.Rollback(context.Background(), changes); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	got := string(mock.writes["/x/y"])
	if got != "previous content" {
		t.Errorf("rollback body = %q, want %q", got, "previous content")
	}
}

func TestFileManager_RequiresSession(t *testing.T) {
	m := NewFileManager()
	_, err := m.Plan(context.Background(), Spec([]config.FileSpec{{Path: "/x", ContentB64: arEmptyB64}}), nil)
	if err != ErrSessionRequired {
		t.Errorf("err = %v, want ErrSessionRequired", err)
	}
}

func TestParseMode_Default(t *testing.T) {
	got, err := parseMode("", 0o600)
	if err != nil || got != 0o600 {
		t.Errorf("default mode: got=%o err=%v want=600", got, err)
	}
	got, err = parseMode("0644", 0)
	if err != nil || got != 0o644 {
		t.Errorf("0644: got=%o err=%v", got, err)
	}
	if _, err := parseMode("notoctal", 0); err == nil {
		t.Error("expected error for invalid mode")
	}
}
