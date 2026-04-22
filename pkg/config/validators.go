package config

import (
	"fmt"
)

// validateCrossFields enforces uniqueness + referential integrity on the
// Linux manifest after struct validation has passed.
func validateCrossFields(l *Linux) error {
	if err := validateUniqueDirectories(l.Directories); err != nil {
		return err
	}
	if err := validateUniqueMounts(l.Mounts); err != nil {
		return err
	}
	if err := validateUsersGroups(l.UsersGroups); err != nil {
		return err
	}
	if err := validateHostsEntries(l.HostsEntries); err != nil {
		return err
	}
	if err := validateServices(l.Services); err != nil {
		return err
	}
	return nil
}

func validateUniqueDirectories(ds []Directory) error {
	seen := map[string]struct{}{}
	for _, d := range ds {
		if _, ok := seen[d.Path]; ok {
			return fmt.Errorf("duplicate directory path: %s", d.Path)
		}
		seen[d.Path] = struct{}{}
	}
	return nil
}

func validateUniqueMounts(ms []Mount) error {
	seen := map[string]struct{}{}
	for _, m := range ms {
		if _, ok := seen[m.MountPoint]; ok {
			return fmt.Errorf("duplicate mount_point: %s", m.MountPoint)
		}
		seen[m.MountPoint] = struct{}{}
	}
	return nil
}

func validateUsersGroups(ug *UsersGroups) error {
	if ug == nil {
		return nil
	}
	groupNames := map[string]struct{}{}
	for _, g := range ug.Groups {
		if _, ok := groupNames[g.Name]; ok {
			return fmt.Errorf("duplicate group: %s", g.Name)
		}
		groupNames[g.Name] = struct{}{}
	}
	userNames := map[string]struct{}{}
	for _, u := range ug.Users {
		if _, ok := userNames[u.Name]; ok {
			return fmt.Errorf("duplicate user: %s", u.Name)
		}
		userNames[u.Name] = struct{}{}
		if u.GID != "" {
			if _, ok := groupNames[u.GID]; !ok {
				return fmt.Errorf("user %s references unknown group %q", u.Name, u.GID)
			}
		}
		for _, g := range u.Groups {
			if _, ok := groupNames[g]; !ok {
				return fmt.Errorf("user %s references unknown group %q", u.Name, g)
			}
		}
	}
	return nil
}

func validateHostsEntries(hs []HostEntry) error {
	seen := map[string]struct{}{}
	for _, h := range hs {
		if _, ok := seen[h.IP]; ok {
			return fmt.Errorf("duplicate hosts entry ip: %s", h.IP)
		}
		seen[h.IP] = struct{}{}
	}
	return nil
}

func validateServices(ss []ServiceState) error {
	seen := map[string]struct{}{}
	for _, s := range ss {
		if _, ok := seen[s.Name]; ok {
			return fmt.Errorf("duplicate service: %s", s.Name)
		}
		seen[s.Name] = struct{}{}
	}
	return nil
}
