package license

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLicense_TierConstants(t *testing.T) {
	if TierCommunity == "" || TierBusiness == "" || TierEnterprise == "" {
		t.Fatal("tier constants must be non-empty")
	}
	if TierCommunity == TierBusiness || TierBusiness == TierEnterprise || TierCommunity == TierEnterprise {
		t.Fatal("tier constants must be distinct")
	}
	if string(TierCommunity) != "community" {
		t.Errorf("TierCommunity = %q", TierCommunity)
	}
	if string(TierBusiness) != "business" {
		t.Errorf("TierBusiness = %q", TierBusiness)
	}
	if string(TierEnterprise) != "enterprise" {
		t.Errorf("TierEnterprise = %q", TierEnterprise)
	}
}

func TestToolCatalog_TotalSize(t *testing.T) {
	// 3 config + 7 env + 13 per-manager + 4 apply + 1 diff + 3 license
	// + 5 business + 8 enterprise = 44
	if len(ToolCatalog) != 44 {
		t.Errorf("expected 44 tools in catalog, got %d", len(ToolCatalog))
	}
}

func TestToolCatalog_TierCounts(t *testing.T) {
	counts := map[Tier]int{}
	for _, tl := range ToolCatalog {
		counts[tl.Tier]++
	}
	if counts[TierCommunity] != 31 {
		t.Errorf("community tools = %d, want 31", counts[TierCommunity])
	}
	if counts[TierBusiness] != 5 {
		t.Errorf("business tools = %d, want 5", counts[TierBusiness])
	}
	if counts[TierEnterprise] != 8 {
		t.Errorf("enterprise tools = %d, want 8", counts[TierEnterprise])
	}
}

func TestToolCatalog_AllNamesPrefixedAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, tl := range ToolCatalog {
		if !strings.HasPrefix(tl.Name, "linuxctl_") {
			t.Errorf("tool %q missing linuxctl_ prefix", tl.Name)
		}
		if seen[tl.Name] {
			t.Errorf("duplicate tool name %q", tl.Name)
		}
		seen[tl.Name] = true
		if tl.Tier == "" {
			t.Errorf("tool %q has empty tier", tl.Name)
		}
	}
}

func TestToolCatalog_KeyEntriesPresent(t *testing.T) {
	want := map[string]Tier{
		"linuxctl_config_validate":      TierCommunity,
		"linuxctl_env_list":             TierCommunity,
		"linuxctl_disk_plan":            TierCommunity,
		"linuxctl_apply_rollback":       TierCommunity,
		"linuxctl_diff":                 TierCommunity,
		"linuxctl_license_status":       TierCommunity,
		"linuxctl_fleet_apply":          TierBusiness,
		"linuxctl_preset_apply":         TierBusiness,
		"linuxctl_drift_auto_remediate": TierBusiness,
		"linuxctl_state_sync_central":   TierEnterprise,
		"linuxctl_rbac_role_add":        TierEnterprise,
		"linuxctl_airgap_bundle":        TierEnterprise,
	}
	byName := map[string]Tier{}
	for _, tl := range ToolCatalog {
		byName[tl.Name] = tl.Tier
	}
	for name, tier := range want {
		got, ok := byName[name]
		if !ok {
			t.Errorf("tool %q missing from catalog", name)
			continue
		}
		if got != tier {
			t.Errorf("tool %q tier = %s, want %s", name, got, tier)
		}
	}
}

func TestCheck_CommunityAllows(t *testing.T) {
	// explicit community
	if err := Check(context.Background(), Caps{Tier: TierCommunity, Op: "disk.plan"}); err != nil {
		t.Fatalf("community check failed: %v", err)
	}
	// empty tier is treated as community
	if err := Check(context.Background(), Caps{Tier: "", Op: "any"}); err != nil {
		t.Fatalf("empty tier should allow: %v", err)
	}
}

func TestCheck_BusinessRefused(t *testing.T) {
	err := Check(context.Background(), Caps{Tier: TierBusiness, Op: "fleet.apply"})
	if err == nil {
		t.Fatal("business check should fail in scaffold build")
	}
	var te ErrTierNotActive
	if !errors.As(err, &te) {
		t.Fatalf("expected ErrTierNotActive, got %T: %v", err, err)
	}
	if te.Tier != TierBusiness {
		t.Errorf("tier = %s, want %s", te.Tier, TierBusiness)
	}
	if te.Op != "fleet.apply" {
		t.Errorf("op = %s", te.Op)
	}
}

func TestCheck_EnterpriseRefused(t *testing.T) {
	err := Check(context.Background(), Caps{Tier: TierEnterprise, Op: "audit.export"})
	if err == nil {
		t.Fatal("enterprise check should fail in scaffold build")
	}
	var te ErrTierNotActive
	if !errors.As(err, &te) {
		t.Fatalf("expected ErrTierNotActive, got %T", err)
	}
	if te.Tier != TierEnterprise {
		t.Errorf("tier = %s", te.Tier)
	}
}

func TestCheck_UnknownTierRefused(t *testing.T) {
	err := Check(context.Background(), Caps{Tier: "ultra", Op: "x"})
	if err == nil {
		t.Fatal("unknown tier should fail")
	}
}

func TestErrTierNotActive_Error(t *testing.T) {
	e := ErrTierNotActive{Tier: TierEnterprise, Op: "rbac.bind.add"}
	msg := e.Error()
	if !strings.Contains(msg, "enterprise") {
		t.Errorf("error missing tier: %s", msg)
	}
	if !strings.Contains(msg, "rbac.bind.add") {
		t.Errorf("error missing op: %s", msg)
	}
	if !strings.Contains(msg, "scaffold") {
		t.Errorf("error missing scaffold marker: %s", msg)
	}
}

func TestCheck_RespectsContext(t *testing.T) {
	// ctx is currently unused but must not panic even when canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Check(ctx, Caps{Tier: TierCommunity, Op: "x"}); err != nil {
		t.Fatalf("canceled ctx should still allow community: %v", err)
	}
}
