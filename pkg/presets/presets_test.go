package presets

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

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

// ---- Golden tests over embedded YAML -------------------------------------

func TestEmbeddedPresets_AllValid(t *testing.T) {
	var count int
	var bundles []string
	err := fs.WalkDir(embeddedFS, "data", func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() || !strings.HasSuffix(p, ".yaml") {
			return nil
		}
		count++
		b, rerr := embeddedFS.ReadFile(p)
		if rerr != nil {
			t.Errorf("%s: read: %v", p, rerr)
			return nil
		}
		var pr Preset
		if derr := yaml.Unmarshal(b, &pr); derr != nil {
			t.Errorf("%s: parse: %v", p, derr)
			return nil
		}
		if pr.Metadata.Name == "" {
			t.Errorf("%s: missing metadata.name", p)
		}
		if pr.Metadata.Version == "" {
			t.Errorf("%s: missing metadata.version", p)
		}
		if pr.Kind != "Preset" && pr.Kind != "Bundle" {
			t.Errorf("%s: unexpected kind %q", p, pr.Kind)
		}
		expectedCategory := path.Base(path.Dir(p))
		if string(pr.Metadata.Category) != expectedCategory {
			t.Errorf("%s: category %q does not match parent dir %q", p, pr.Metadata.Category, expectedCategory)
		}
		fn := strings.TrimSuffix(path.Base(p), ".yaml")
		if fn != pr.Metadata.Name {
			t.Errorf("%s: filename %q does not match metadata.name %q", p, fn, pr.Metadata.Name)
		}
		if pr.Kind == "Bundle" {
			bundles = append(bundles, pr.Metadata.Name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count < 15 {
		t.Errorf("expected 15+ shipped presets, got %d", count)
	}
	// Bundles reference real presets.
	for _, bname := range bundles {
		children, err := BundleExpand(bname, nil)
		if err != nil {
			t.Errorf("bundle %q expand: %v", bname, err)
			continue
		}
		for cat, child := range children {
			if _, err := ResolveCategory(cat, child, nil); err != nil {
				t.Errorf("bundle %q: %s preset %q not resolvable: %v", bname, cat, child, err)
			}
		}
	}
}

func TestEmbeddedPresets_Oracle19cSysctlByteForByte(t *testing.T) {
	// Regression gate: migrated YAML preset must produce identical entries
	// to the hardcoded Go data it replaces.
	p, err := ResolveCategory("sysctl", "oracle-19c", func() Tier { return TierCommunity })
	if err != nil {
		t.Fatal(err)
	}
	entries, err := SysctlSpec(p)
	if err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{
		"fs.aio-max-nr", "fs.file-max", "kernel.panic_on_oops",
		"kernel.sem", "kernel.shmall", "kernel.shmmax", "kernel.shmmni",
		"net.core.rmem_max", "net.core.wmem_max", "vm.swappiness",
	}
	if len(entries) != len(wantKeys) {
		t.Errorf("oracle-19c sysctl preset: want %d entries, got %d", len(wantKeys), len(entries))
	}
	byKey := map[string]string{}
	for _, e := range entries {
		byKey[e.Key] = e.Value
	}
	want := map[string]string{
		"fs.aio-max-nr":        "1048576",
		"fs.file-max":          "6815744",
		"kernel.panic_on_oops": "1",
		"kernel.sem":           "250 32000 100 128",
		"kernel.shmall":        "1073741824",
		"kernel.shmmax":        "4398046511104",
		"kernel.shmmni":        "4096",
		"net.core.rmem_max":    "4194304",
		"net.core.wmem_max":    "1048576",
		"vm.swappiness":        "10",
	}
	for k, v := range want {
		if byKey[k] != v {
			t.Errorf("sysctl preset key %s: want %q, got %q", k, v, byKey[k])
		}
	}
}

func TestEmbeddedPresets_Oracle19cLimitsByteForByte(t *testing.T) {
	p, err := ResolveCategory("limits", "oracle-19c", nil)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := LimitsSpec(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 16 {
		t.Errorf("oracle-19c limits: want 16 entries (2 users * 8), got %d", len(entries))
	}
	userCount := map[string]int{}
	for _, e := range entries {
		userCount[e.User]++
	}
	if userCount["grid"] != 8 || userCount["oracle"] != 8 {
		t.Errorf("want 8 entries per user, got %+v", userCount)
	}
}

func TestResolve_AmbiguousName(t *testing.T) {
	// oracle-19c exists in packages, sysctl, limits → ambiguous.
	if _, err := Resolve("oracle-19c", nil); err == nil {
		t.Error("expected ambiguous error for oracle-19c")
	}
}

func TestList_CommunityTier(t *testing.T) {
	metas := List(func() Tier { return TierCommunity })
	if len(metas) < 15 {
		t.Errorf("expected 15+ presets at community tier, got %d", len(metas))
	}
}

func TestBundleExpand_OracleRac19c(t *testing.T) {
	children, err := BundleExpand("oracle-rac-19c", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"directories":  "oracle-ofa-19c",
		"users_groups": "oracle-rac",
		"packages":     "oracle-19c",
		"sysctl":       "oracle-19c",
		"limits":       "oracle-19c",
	}
	for k, v := range want {
		if children[k] != v {
			t.Errorf("bundle child[%s]: want %q, got %q", k, v, children[k])
		}
	}
}

func TestBundleExpand_NotABundle(t *testing.T) {
	if _, err := BundleExpand("oracle-ofa-19c", nil); err == nil {
		t.Error("expected error for non-bundle name")
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
