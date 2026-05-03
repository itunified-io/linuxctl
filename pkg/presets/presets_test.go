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
		"net.core.rmem_default", "net.core.rmem_max",
		"net.core.wmem_default", "net.core.wmem_max",
		"net.ipv4.conf.all.rp_filter", "net.ipv4.conf.default.rp_filter",
		"vm.swappiness",
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
	// 2 users × 10 entries (nofile/nproc/stack/memlock × soft+hard = 8) +
	// 2 entries per user for `data` (soft+hard unlimited; MoS 1264284.1).
	if len(entries) != 20 {
		t.Errorf("oracle-19c limits: want 20 entries (2 users * 10), got %d", len(entries))
	}
	userCount := map[string]int{}
	for _, e := range entries {
		userCount[e.User]++
	}
	if userCount["grid"] != 10 || userCount["oracle"] != 10 {
		t.Errorf("want 10 entries per user, got %+v", userCount)
	}
}

// TestEmbeddedPresets_OracleSingle_HasGridUser is a regression gate for
// linuxctl#53. /lab-up Phase D.1 (oracle-asm-configure) and Phase D.2
// (oracle-grid-install) both need the grid user to own GRID_HOME and run
// asmca / runInstaller. The oracle-single preset (used by every
// oracle-single-* bundle) MUST ship with both oracle + grid users; with the
// correct ASM groups; and oracle MUST NOT be in asmadmin or asmoper (those
// are reserved for grid).
func TestEmbeddedPresets_OracleSingle_HasGridUser(t *testing.T) {
	p, err := ResolveCategory("users_groups", "oracle-single", nil)
	if err != nil {
		t.Fatal(err)
	}
	ug, err := UsersGroupsSpec(p)
	if err != nil {
		t.Fatal(err)
	}

	// Index users + groups for assertions.
	users := map[string]int{}
	for _, u := range ug.Users {
		users[u.Name] = u.UID
	}
	groups := map[string]int{}
	for _, g := range ug.Groups {
		groups[g.Name] = g.GID
	}

	// Grid user — required for ASM/Oracle Restart.
	gridUID, hasGrid := users["grid"]
	if !hasGrid {
		t.Fatal("oracle-single: missing grid user — Phase D.1/D.2 will fail")
	}
	if gridUID != 54322 {
		t.Errorf("oracle-single: grid uid want 54322, got %d", gridUID)
	}

	// Oracle user — must remain present.
	if _, ok := users["oracle"]; !ok {
		t.Fatal("oracle-single: missing oracle user")
	}

	// Required ASM groups (Oracle 19c standard GIDs).
	for name, wantGID := range map[string]int{
		"asmadmin": 54327,
		"asmdba":   54328,
		"asmoper":  54329,
	} {
		got, ok := groups[name]
		if !ok {
			t.Errorf("oracle-single: missing ASM group %q", name)
			continue
		}
		if got != wantGID {
			t.Errorf("oracle-single: %s gid want %d, got %d", name, wantGID, got)
		}
	}

	// Oracle user MUST NOT be in asmadmin / asmoper — those are grid-only.
	for _, u := range ug.Users {
		if u.Name != "oracle" {
			continue
		}
		for _, g := range u.Groups {
			if g == "asmadmin" || g == "asmoper" {
				t.Errorf("oracle-single: oracle user MUST NOT be in %q (reserved for grid)", g)
			}
		}
	}

	// Grid user MUST be in asmadmin/asmdba/asmoper/dba.
	for _, u := range ug.Users {
		if u.Name != "grid" {
			continue
		}
		want := map[string]bool{"asmadmin": false, "asmdba": false, "asmoper": false, "dba": false}
		for _, g := range u.Groups {
			if _, ok := want[g]; ok {
				want[g] = true
			}
		}
		for g, found := range want {
			if !found {
				t.Errorf("oracle-single: grid user missing required supplementary group %q", g)
			}
		}
	}
}

// TestEmbeddedPresets_Oracle19c_OL9BuildDeps is a regression gate for
// linuxctl#57. /lab-up Phase D.2 (oracle-grid-install) on OL9 fails with
// "gcc: command not found" because oracle-database-preinstall-19c does NOT
// pull a working C toolchain on OL9. The oracle-19c packages preset MUST
// install gcc + gcc-c++ + make alongside the preinstall metapackage.
//
// cvuqdisk is intentionally NOT in this preset — it ships inside the Grid
// Home zip at $GRID_HOME/cv/rpm/ and is installed by the
// /oracle-grid-install skill at extraction time.
func TestEmbeddedPresets_Oracle19c_OL9BuildDeps(t *testing.T) {
	p, err := ResolveCategory("packages", "oracle-19c", nil)
	if err != nil {
		t.Fatal(err)
	}
	pp, err := PackagesSpec(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"oracle-database-preinstall-19c", // baseline
		"kmod-oracleasm",                 // baseline
		"cifs-utils",                     // baseline
		"gcc",                            // linuxctl#57
		"gcc-c++",                        // linuxctl#57
		"make",                           // linuxctl#57
		"binutils",                       // linuxctl#57
	} {
		if !containsStr(pp.Install, want) {
			t.Errorf("oracle-19c packages preset: missing required package %q (install=%v)", want, pp.Install)
		}
	}
	// cvuqdisk MUST NOT be in this preset — it's a Grid-extraction-time step.
	if containsStr(pp.Install, "cvuqdisk") {
		t.Error("oracle-19c packages preset: cvuqdisk MUST NOT be here (it ships inside Grid zip; install via /oracle-grid-install Step 0a)")
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

// TestBundleInlineExpand_OracleSingle19c verifies that the OL9 Oracle 19c
// inline-capability shims (linuxctl#57 v2) are surfaced by the registry:
//   - ol9_codeready_builder must be in repos_enable
//   - /usr/lib64/libpthread_nonshared.a stub must be in files (CreateOnly)
func TestBundleInlineExpand_OracleSingle19c(t *testing.T) {
	repos, files, err := BundleInlineExpand("oracle-single-19c", nil)
	if err != nil {
		t.Fatal(err)
	}
	foundCRB := false
	for _, r := range repos {
		if r == "ol9_codeready_builder" {
			foundCRB = true
		}
	}
	if !foundCRB {
		t.Errorf("oracle-single-19c missing ol9_codeready_builder in repos_enable; got %v", repos)
	}
	var stub *config.FileSpec
	for i, f := range files {
		if f.Path == "/usr/lib64/libpthread_nonshared.a" {
			stub = &files[i]
		}
	}
	if stub == nil {
		t.Fatalf("oracle-single-19c missing libpthread_nonshared.a stub; got files=%v", files)
	}
	if !stub.CreateOnly {
		t.Errorf("libpthread stub should be CreateOnly")
	}
	if stub.Mode != "0644" || stub.Owner != "root" || stub.Group != "root" {
		t.Errorf("libpthread stub metadata: mode=%q owner=%q group=%q", stub.Mode, stub.Owner, stub.Group)
	}
	if stub.ContentB64 == "" {
		t.Errorf("libpthread stub has no content_b64")
	}
}

func TestBundleInlineExpand_NoInlineYieldsEmpty(t *testing.T) {
	// oracle-rac-19c does not declare inline capabilities — both lists empty.
	repos, files, err := BundleInlineExpand("oracle-rac-19c", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 || len(files) != 0 {
		t.Errorf("oracle-rac-19c should have no inline caps, got repos=%v files=%v", repos, files)
	}
}

func TestBundleParse_RepoAndFiles(t *testing.T) {
	// Round-trip a Bundle YAML through schema decode to confirm
	// repos_enable + files survive parsing.
	src := `
apiVersion: linuxctl.itunified.io/preset/v1
kind: Bundle
metadata:
  name: test-inline
  category: bundles
spec:
  bundle:
    packages_preset: oracle-19c
    repos_enable:
      - r1
      - r2
    files:
      - path: /a
        mode: "0600"
        owner: root
        group: root
        content_b64: aGVsbG8=
        create_only: true
`
	var pr Preset
	if err := yaml.Unmarshal([]byte(src), &pr); err != nil {
		t.Fatal(err)
	}
	b, err := decodeBundle(&pr)
	if err != nil {
		t.Fatal(err)
	}
	if b.PackagesPreset != "oracle-19c" {
		t.Errorf("PackagesPreset = %q", b.PackagesPreset)
	}
	if len(b.ReposEnable) != 2 || b.ReposEnable[0] != "r1" {
		t.Errorf("ReposEnable = %v", b.ReposEnable)
	}
	if len(b.Files) != 1 || b.Files[0].Path != "/a" || !b.Files[0].CreateOnly {
		t.Errorf("Files = %+v", b.Files)
	}
}

func TestSpecDecoders(t *testing.T) {
	// Directories
	p, err := ResolveCategory("directories", "oracle-ofa-19c", nil)
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := DirectoriesSpec(p)
	if err != nil || len(dirs) == 0 {
		t.Errorf("DirectoriesSpec: %v len=%d", err, len(dirs))
	}

	// UsersGroups
	p, _ = ResolveCategory("users_groups", "oracle-rac", nil)
	ug, err := UsersGroupsSpec(p)
	if err != nil || ug == nil || len(ug.Groups) == 0 {
		t.Errorf("UsersGroupsSpec failed: err=%v ug=%+v", err, ug)
	}

	// Packages
	p, _ = ResolveCategory("packages", "oracle-19c", nil)
	pp, err := PackagesSpec(p)
	if err != nil || pp == nil || len(pp.Install) == 0 {
		t.Errorf("PackagesSpec failed: err=%v pp=%+v", err, pp)
	}
}

func TestResolveCategory_NotFound(t *testing.T) {
	if _, err := ResolveCategory("bundles", "nope-xx", nil); err == nil {
		t.Error("expected error")
	}
	if _, err := ResolveCategory("", "", nil); err == nil {
		t.Error("empty name should error")
	}
}

func TestList_BusinessTierIncludesCommunity(t *testing.T) {
	// All shipped presets are community tier — business tier should see the
	// same count (or more in the future).
	c := List(func() Tier { return TierCommunity })
	b := List(func() Tier { return TierBusiness })
	if len(b) < len(c) {
		t.Errorf("business should see >= community count (c=%d b=%d)", len(c), len(b))
	}
}

func TestMergeLimits_FullSort(t *testing.T) {
	entries := []config.LimitEntry{
		{User: "b", Type: "hard", Item: "nofile", Value: "2"},
		{User: "a", Type: "soft", Item: "nproc", Value: "1"},
		{User: "a", Type: "hard", Item: "nofile", Value: "1"},
		{User: "a", Type: "soft", Item: "nofile", Value: "1"},
	}
	out := MergeLimits(entries, nil)
	if len(out) != 4 {
		t.Fatalf("want 4, got %d", len(out))
	}
	// a/hard/nofile, a/soft/nofile, a/soft/nproc, b/hard/nofile
	if out[0].User != "a" || out[0].Type != "hard" {
		t.Errorf("bad order: %+v", out)
	}
	if out[3].User != "b" {
		t.Errorf("bad order: %+v", out)
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
