package presets

import (
	"sort"

	"github.com/itunified-io/linuxctl/pkg/config"
)

// MergeDirectories merges explicit entries with a preset's entries, deduping
// by path. Explicit entries win on path collision. Output is sorted by path
// for deterministic comparison.
func MergeDirectories(explicit []config.Directory, preset []config.Directory) []config.Directory {
	byPath := map[string]config.Directory{}
	for _, p := range preset {
		byPath[p.Path] = p
	}
	for _, e := range explicit {
		byPath[e.Path] = e
	}
	out := make([]config.Directory, 0, len(byPath))
	for _, v := range byPath {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// MergeUsersGroups merges explicit groups + users with a preset's, deduping
// by name. For users, the group list is unioned (preset groups + explicit
// groups); other user fields are overridden by the explicit entry when set.
func MergeUsersGroups(explicit config.UsersGroups, preset config.UsersGroups) config.UsersGroups {
	// Groups: dedup by name, explicit wins.
	gByName := map[string]config.Group{}
	for _, g := range preset.Groups {
		gByName[g.Name] = g
	}
	for _, g := range explicit.Groups {
		gByName[g.Name] = g
	}
	groups := make([]config.Group, 0, len(gByName))
	for _, g := range gByName {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })

	// Users: explicit wins on all fields, except Groups which is unioned.
	uByName := map[string]config.User{}
	presetByName := map[string]config.User{}
	for _, u := range preset.Users {
		uByName[u.Name] = u
		presetByName[u.Name] = u
	}
	for _, u := range explicit.Users {
		if prev, had := presetByName[u.Name]; had {
			merged := u
			merged.Groups = unionStrings(prev.Groups, u.Groups)
			// Fill holes in explicit from preset.
			if merged.UID == 0 {
				merged.UID = prev.UID
			}
			if merged.GID == "" {
				merged.GID = prev.GID
			}
			if merged.Home == "" {
				merged.Home = prev.Home
			}
			if merged.Shell == "" {
				merged.Shell = prev.Shell
			}
			if len(merged.SSHKeys) == 0 {
				merged.SSHKeys = prev.SSHKeys
			}
			if merged.Password == "" {
				merged.Password = prev.Password
			}
			uByName[u.Name] = merged
		} else {
			uByName[u.Name] = u
		}
	}
	users := make([]config.User, 0, len(uByName))
	for _, u := range uByName {
		users = append(users, u)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })

	return config.UsersGroups{Groups: groups, Users: users}
}

// MergePackages merges explicit + preset package lists. Install and Remove
// sets are unioned; identical names survive only once. If a package appears
// in both Install and Remove (conflict), the explicit side wins.
func MergePackages(explicit config.Packages, preset config.Packages) config.Packages {
	install := unionStrings(preset.Install, explicit.Install)
	remove := unionStrings(preset.Remove, explicit.Remove)

	// Explicit wins on Install/Remove collision.
	explicitInstall := map[string]bool{}
	explicitRemove := map[string]bool{}
	for _, n := range explicit.Install {
		explicitInstall[n] = true
	}
	for _, n := range explicit.Remove {
		explicitRemove[n] = true
	}

	inInstall := map[string]bool{}
	for _, n := range install {
		inInstall[n] = true
	}
	// Remove any name that appears in both install+remove: explicit wins.
	cleanInstall := make([]string, 0, len(install))
	for _, n := range install {
		if inRemove(remove, n) {
			if explicitInstall[n] {
				cleanInstall = append(cleanInstall, n)
			}
			continue
		}
		cleanInstall = append(cleanInstall, n)
	}
	cleanRemove := make([]string, 0, len(remove))
	for _, n := range remove {
		if inInstall[n] {
			if explicitRemove[n] {
				cleanRemove = append(cleanRemove, n)
			}
			continue
		}
		cleanRemove = append(cleanRemove, n)
	}

	// Enabled / DisabledServices: union.
	enabled := unionStrings(preset.EnabledServices, explicit.EnabledServices)
	disabled := unionStrings(preset.DisabledServices, explicit.DisabledServices)

	sort.Strings(cleanInstall)
	sort.Strings(cleanRemove)
	sort.Strings(enabled)
	sort.Strings(disabled)

	return config.Packages{
		Install:          cleanInstall,
		Remove:           cleanRemove,
		EnabledServices:  enabled,
		DisabledServices: disabled,
	}
}

// MergeSysctl merges explicit + preset sysctl entries, deduping by key.
// Explicit wins on key collision. Output is sorted by key.
func MergeSysctl(explicit []config.SysctlEntry, preset []config.SysctlEntry) []config.SysctlEntry {
	byKey := map[string]string{}
	for _, p := range preset {
		byKey[p.Key] = p.Value
	}
	for _, e := range explicit {
		byKey[e.Key] = e.Value
	}
	out := make([]config.SysctlEntry, 0, len(byKey))
	for k, v := range byKey {
		out = append(out, config.SysctlEntry{Key: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// MergeLimits merges explicit + preset limit entries, deduping by
// (user, type, item). Explicit wins.
func MergeLimits(explicit []config.LimitEntry, preset []config.LimitEntry) []config.LimitEntry {
	key := func(l config.LimitEntry) string { return l.User + "|" + l.Type + "|" + l.Item }
	byKey := map[string]config.LimitEntry{}
	for _, p := range preset {
		byKey[key(p)] = p
	}
	for _, e := range explicit {
		byKey[key(e)] = e
	}
	out := make([]config.LimitEntry, 0, len(byKey))
	for _, v := range byKey {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].User != out[j].User {
			return out[i].User < out[j].User
		}
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Item < out[j].Item
	})
	return out
}

// unionStrings returns the stable-order union of two string slices, dedup.
// Order: a-items first in their input order, then a-missing b-items in their
// order.
func unionStrings(a, b []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func inRemove(remove []string, name string) bool {
	for _, r := range remove {
		if r == name {
			return true
		}
	}
	return false
}
