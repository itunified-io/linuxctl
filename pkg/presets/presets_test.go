package presets

import (
	"testing"

	"github.com/itunified-io/linuxctl/pkg/config"
)

func TestMergeDirectories_ExplicitWins(t *testing.T) {
	preset := []config.Directory{
		{Path: "/u01/app/oracle", Owner: "oracle", Group: "oinstall", Mode: "0775"},
		{Path: "/smb/software", Owner: "root", Group: "root", Mode: "0755"},
	}
	explicit := []config.Directory{
		{Path: "/smb/software", Owner: "root", Group: "root", Mode: "0700"}, // override
		{Path: "/opt/custom", Owner: "root", Group: "root", Mode: "0755"},   // new
	}
	out := MergeDirectories(explicit, preset)
	if len(out) != 3 {
		t.Fatalf("want 3 dirs, got %d: %+v", len(out), out)
	}
	// Sorted by path
	if out[0].Path != "/opt/custom" || out[1].Path != "/smb/software" || out[2].Path != "/u01/app/oracle" {
		t.Errorf("unexpected ordering: %+v", out)
	}
	// Override
	for _, d := range out {
		if d.Path == "/smb/software" && d.Mode != "0700" {
			t.Errorf("explicit did not win: %+v", d)
		}
	}
}

func TestMergeDirectories_Empty(t *testing.T) {
	if out := MergeDirectories(nil, nil); len(out) != 0 {
		t.Errorf("want empty, got %+v", out)
	}
}

func TestMergeUsersGroups_UnionGroupsExplicitWins(t *testing.T) {
	preset := config.UsersGroups{
		Groups: []config.Group{{Name: "oinstall", GID: 54321}, {Name: "dba", GID: 54322}},
		Users: []config.User{
			{Name: "oracle", UID: 54321, GID: "oinstall", Groups: []string{"dba"}},
		},
	}
	explicit := config.UsersGroups{
		Groups: []config.Group{{Name: "wheel", GID: 10}},
		Users: []config.User{
			{Name: "oracle", Groups: []string{"extra"}, Shell: "/bin/zsh"},
			{Name: "alice", Shell: "/bin/bash"},
		},
	}
	out := MergeUsersGroups(explicit, preset)
	if len(out.Groups) != 3 {
		t.Errorf("want 3 groups, got %d: %+v", len(out.Groups), out.Groups)
	}
	if len(out.Users) != 2 {
		t.Fatalf("want 2 users, got %+v", out.Users)
	}
	var oracle config.User
	for _, u := range out.Users {
		if u.Name == "oracle" {
			oracle = u
		}
	}
	if oracle.UID != 54321 {
		t.Errorf("oracle UID lost from preset: %+v", oracle)
	}
	if oracle.Shell != "/bin/zsh" {
		t.Errorf("explicit shell did not win: %+v", oracle)
	}
	if !containsStr(oracle.Groups, "dba") || !containsStr(oracle.Groups, "extra") {
		t.Errorf("groups union failed: %+v", oracle.Groups)
	}
}

func TestMergeUsersGroups_GroupOverride(t *testing.T) {
	preset := config.UsersGroups{Groups: []config.Group{{Name: "oinstall", GID: 1000}}}
	explicit := config.UsersGroups{Groups: []config.Group{{Name: "oinstall", GID: 54321}}}
	out := MergeUsersGroups(explicit, preset)
	if len(out.Groups) != 1 || out.Groups[0].GID != 54321 {
		t.Errorf("explicit should override GID: %+v", out.Groups)
	}
}

func TestMergePackages_UnionAndConflict(t *testing.T) {
	preset := config.Packages{
		Install: []string{"cifs-utils", "nfs-utils"},
		Remove:  []string{"telnet"},
	}
	explicit := config.Packages{
		Install: []string{"htop", "nfs-utils"}, // dedup
		Remove:  []string{"cifs-utils"},        // explicit remove wins over preset install
	}
	out := MergePackages(explicit, preset)
	if containsStr(out.Install, "cifs-utils") {
		t.Errorf("explicit remove should win over preset install: %+v", out)
	}
	if !containsStr(out.Remove, "cifs-utils") {
		t.Errorf("cifs-utils should be in Remove: %+v", out)
	}
	if !containsStr(out.Install, "htop") || !containsStr(out.Install, "nfs-utils") {
		t.Errorf("install union failed: %+v", out.Install)
	}
	if !containsStr(out.Remove, "telnet") {
		t.Errorf("preset remove lost: %+v", out.Remove)
	}
}

func TestMergeSysctl_ExplicitWins(t *testing.T) {
	preset := []config.SysctlEntry{{Key: "vm.swappiness", Value: "10"}}
	explicit := []config.SysctlEntry{{Key: "vm.swappiness", Value: "1"}}
	out := MergeSysctl(explicit, preset)
	if len(out) != 1 || out[0].Value != "1" {
		t.Errorf("explicit did not win: %+v", out)
	}
}

func TestMergeLimits_ExplicitWins(t *testing.T) {
	preset := []config.LimitEntry{{User: "oracle", Type: "soft", Item: "nofile", Value: "1024"}}
	explicit := []config.LimitEntry{{User: "oracle", Type: "soft", Item: "nofile", Value: "4096"}}
	out := MergeLimits(explicit, preset)
	if len(out) != 1 || out[0].Value != "4096" {
		t.Errorf("explicit did not win: %+v", out)
	}
}

func TestTierRank_And_Resolve(t *testing.T) {
	if tierRank(TierCommunity) >= tierRank(TierBusiness) {
		t.Error("community should rank below business")
	}
	if tierRank(TierBusiness) >= tierRank(TierEnterprise) {
		t.Error("business should rank below enterprise")
	}
	if tierRank(Tier("garbage")) != 1 {
		t.Error("unknown tier should default to community rank")
	}
	if resolveTier(nil) != TierCommunity {
		t.Error("nil fn should default to community")
	}
	if resolveTier(func() Tier { return "" }) != TierCommunity {
		t.Error("empty tier should default to community")
	}
	if resolveTier(func() Tier { return TierBusiness }) != TierBusiness {
		t.Error("tierFn should be honored")
	}
}

func TestResolve_EmptyName(t *testing.T) {
	if _, err := Resolve("", nil); err == nil {
		t.Error("expected error on empty name")
	}
}

func TestResolve_UnknownName(t *testing.T) {
	if _, err := Resolve("does-not-exist-xxx", nil); err == nil {
		t.Error("expected error on unknown preset")
	}
}

func TestUnionStrings(t *testing.T) {
	a := []string{"a", "b"}
	b := []string{"b", "c"}
	out := unionStrings(a, b)
	if len(out) != 3 || out[0] != "a" || out[1] != "b" || out[2] != "c" {
		t.Errorf("union failed: %+v", out)
	}
}

func containsStr(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}
