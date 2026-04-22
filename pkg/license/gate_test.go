package license

import (
	"context"
	"testing"
)

func TestLicense_TierConstants(t *testing.T) {
	if TierCommunity == "" || TierBusiness == "" || TierEnterprise == "" {
		t.Fatal("tier constants must be non-empty")
	}
	if TierCommunity == TierBusiness || TierBusiness == TierEnterprise {
		t.Fatal("tier constants must be distinct")
	}
}

func TestToolCatalog_NotEmpty(t *testing.T) {
	if len(ToolCatalog) < 35 {
		t.Fatalf("expected >=35 tools in catalog, got %d", len(ToolCatalog))
	}
	community := 0
	for _, tl := range ToolCatalog {
		if tl.Tier == TierCommunity {
			community++
		}
	}
	if community < 30 {
		t.Errorf("expected >=30 community tools, got %d", community)
	}
}

func TestLicense_CheckCommunity(t *testing.T) {
	if err := Check(context.Background(), Caps{Tier: TierCommunity, Op: "disk.plan"}); err != nil {
		t.Fatalf("community check failed: %v", err)
	}
	if err := Check(context.Background(), Caps{Tier: TierBusiness, Op: "fleet.apply"}); err == nil {
		t.Fatal("business check should fail in scaffold build")
	}
}
