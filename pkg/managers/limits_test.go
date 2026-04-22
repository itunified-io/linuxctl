package managers

import (
	"context"
	"strings"
	"testing"

	"github.com/itunified-io/linuxctl/pkg/config"
)

func TestLimits_PresetExpansion(t *testing.T) {
	p := presetLimits("oracle-19c")
	if len(p) != 16 {
		t.Fatalf("oracle-19c should yield 16 entries (grid+oracle * 8), got %d", len(p))
	}
	users := map[string]int{}
	for _, e := range p {
		users[e.User]++
	}
	if users["grid"] != 8 || users["oracle"] != 8 {
		t.Errorf("expected 8 entries per user, got %+v", users)
	}
	if got := presetLimits("pg-16"); got != nil {
		t.Errorf("pg-16 should stub to nil, got %d", len(got))
	}
	if got := presetLimits(""); got != nil {
		t.Error("empty preset should yield nil")
	}
	if got := presetLimits("unknown"); got != nil {
		t.Error("unknown preset should yield nil")
	}
}

func TestLimits_MergePresetExplicit(t *testing.T) {
	preset := []config.LimitEntry{
		{User: "oracle", Type: "soft", Item: "nofile", Value: "1024"},
		{User: "oracle", Type: "hard", Item: "nofile", Value: "65536"},
	}
	explicit := []config.LimitEntry{
		{User: "oracle", Type: "soft", Item: "nofile", Value: "4096"}, // override
		{User: "app", Type: "hard", Item: "nproc", Value: "8192"},     // new
	}
	merged := mergeLimits(explicit, preset)
	if len(merged) != 3 {
		t.Fatalf("want 3 merged entries, got %d: %+v", len(merged), merged)
	}
	for _, e := range merged {
		if e.User == "oracle" && e.Type == "soft" && e.Item == "nofile" && e.Value != "4096" {
			t.Errorf("explicit override failed: got value %q", e.Value)
		}
	}
}

func TestLimits_Render(t *testing.T) {
	body := renderLimits([]config.LimitEntry{
		{User: "oracle", Type: "soft", Item: "nofile", Value: "1024"},
	})
	if !strings.Contains(body, "oracle soft nofile 1024") {
		t.Errorf("rendered body wrong: %q", body)
	}
	if !strings.HasPrefix(body, "# Managed by linuxctl") {
		t.Errorf("body missing header: %q", body)
	}
}

func TestLimits_Plan_NoDrift(t *testing.T) {
	body := renderLimits([]config.LimitEntry{
		{User: "oracle", Type: "soft", Item: "nofile", Value: "1024"},
	})
	ms := newFileMock().withFile(LimitsManagedPath, body)
	l := NewLimitsManager().WithSession(ms)
	changes, err := l.Plan(context.Background(), &config.Linux{
		Limits: []config.LimitEntry{{User: "oracle", Type: "soft", Item: "nofile", Value: "1024"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Errorf("want no drift, got %+v", changes)
	}
}

func TestLimits_Plan_FileMissing(t *testing.T) {
	ms := newFileMock()
	l := NewLimitsManager().WithSession(ms)
	changes, err := l.Plan(context.Background(), &config.Linux{
		Limits: []config.LimitEntry{{User: "oracle", Type: "soft", Item: "nofile", Value: "1024"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Action != "update" {
		t.Fatalf("want 1 update, got %+v", changes)
	}
}

func TestLimits_Plan_DriftDetected(t *testing.T) {
	ms := newFileMock().withFile(LimitsManagedPath, "oracle soft nofile 512\n")
	l := NewLimitsManager().WithSession(ms)
	changes, err := l.Plan(context.Background(), &config.Linux{
		Limits: []config.LimitEntry{{User: "oracle", Type: "soft", Item: "nofile", Value: "1024"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("want 1 change, got %+v", changes)
	}
	a := changes[0].After.(limitsApply)
	if !strings.Contains(a.Body, "oracle soft nofile 1024") {
		t.Errorf("body missing desired: %q", a.Body)
	}
}

func TestLimits_Apply_WritesFile(t *testing.T) {
	ms := newFileMock()
	l := NewLimitsManager().WithSession(ms)
	entries := []config.LimitEntry{{User: "oracle", Type: "soft", Item: "nofile", Value: "1024"}}
	body := renderLimits(entries)
	changes := []Change{{Action: "update",
		Before: limitsSnap{},
		After:  limitsApply{Body: body, Entries: entries}}}
	res, err := l.Apply(context.Background(), changes, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Applied) != 1 || len(res.Failed) != 0 {
		t.Fatalf("want 1 applied, got %+v", res)
	}
	if string(ms.writes[LimitsManagedPath]) != body {
		t.Errorf("written body mismatch: %q", ms.writes[LimitsManagedPath])
	}
}

func TestLimits_Apply_DryRun(t *testing.T) {
	ms := newFileMock()
	l := NewLimitsManager().WithSession(ms)
	res, _ := l.Apply(context.Background(), []Change{{Action: "update", After: limitsApply{}}}, true)
	if len(res.Skipped) != 1 {
		t.Fatalf("want 1 skipped, got %+v", res)
	}
	if len(ms.writes) != 0 {
		t.Error("dry-run should not write")
	}
}

func TestLimits_Apply_NoSession(t *testing.T) {
	l := NewLimitsManager()
	_, err := l.Apply(context.Background(), []Change{{Action: "update", After: limitsApply{}}}, false)
	if err == nil {
		t.Error("want session-required error")
	}
}

func TestLimits_Rollback_Restores(t *testing.T) {
	ms := newFileMock().withFile(LimitsManagedPath, "new\n")
	l := NewLimitsManager().WithSession(ms)
	changes := []Change{{Action: "update",
		Before: limitsSnap{Body: "old entry 1\n"},
		After:  limitsApply{Body: "new\n"}}}
	if err := l.Rollback(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	if string(ms.writes[LimitsManagedPath]) != "old entry 1\n" {
		t.Errorf("rollback didn't restore: %q", ms.writes[LimitsManagedPath])
	}
}

func TestLimits_Rollback_NoPrevRemoves(t *testing.T) {
	ms := newFileMock()
	l := NewLimitsManager().WithSession(ms)
	changes := []Change{{Action: "update",
		Before: limitsSnap{Body: ""},
		After:  limitsApply{Body: "x\n"}}}
	if err := l.Rollback(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	if !ms.ran("rm -f '" + LimitsManagedPath + "'") {
		t.Errorf("expected rm -f; cmds=%v", ms.cmds)
	}
}

func TestLimits_Verify(t *testing.T) {
	body := renderLimits([]config.LimitEntry{{User: "oracle", Type: "soft", Item: "nofile", Value: "1024"}})
	ms := newFileMock().withFile(LimitsManagedPath, body)
	l := NewLimitsManager().WithSession(ms)
	vr, err := l.Verify(context.Background(), &config.Linux{
		Limits: []config.LimitEntry{{User: "oracle", Type: "soft", Item: "nofile", Value: "1024"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !vr.OK {
		t.Errorf("want OK; drift=%+v", vr.Drift)
	}
}

func TestLimits_Plan_NoSession(t *testing.T) {
	l := NewLimitsManager()
	_, err := l.Plan(context.Background(), &config.Linux{
		Limits: []config.LimitEntry{{User: "x", Type: "soft", Item: "y", Value: "1"}},
	}, nil)
	if err == nil {
		t.Error("want session-required")
	}
}

func TestLimits_CastLinuxForLimits_Variants(t *testing.T) {
	if _, err := castLinuxForLimits(nil); err != nil {
		t.Error(err)
	}
	if _, err := castLinuxForLimits([]config.LimitEntry{{User: "x", Type: "soft", Item: "y", Value: "1"}}); err != nil {
		t.Error(err)
	}
	if _, err := castLinuxForLimits(config.Linux{}); err != nil {
		t.Error(err)
	}
	if _, err := castLinuxForLimits("bad"); err == nil {
		t.Error("want error for bad type")
	}
}

func TestLimits_Apply_WrongAfterType(t *testing.T) {
	ms := newFileMock()
	l := NewLimitsManager().WithSession(ms)
	res, _ := l.Apply(context.Background(), []Change{{Action: "update", After: "bad"}}, false)
	if len(res.Failed) != 1 {
		t.Error("want 1 failed")
	}
}

func TestLimits_Rollback_NoSession(t *testing.T) {
	l := NewLimitsManager()
	if err := l.Rollback(context.Background(), []Change{{}}); err == nil {
		t.Error("want session-required")
	}
}

func TestLimits_Rollback_BadBeforeSkipped(t *testing.T) {
	ms := newFileMock()
	l := NewLimitsManager().WithSession(ms)
	changes := []Change{{Action: "update", Before: "not a snap"}}
	if err := l.Rollback(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	if len(ms.writes) != 0 {
		t.Error("should not write")
	}
}

func TestLimits_PresetOnly_DriftWhenMissing(t *testing.T) {
	ms := newFileMock()
	l := NewLimitsManager().WithSession(ms)
	changes, err := l.Plan(context.Background(), &config.Linux{LimitsPreset: "oracle-19c"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("want 1 change from preset, got %+v", changes)
	}
	a := changes[0].After.(limitsApply)
	if len(a.Entries) != 16 {
		t.Errorf("want 16 preset entries, got %d", len(a.Entries))
	}
}
