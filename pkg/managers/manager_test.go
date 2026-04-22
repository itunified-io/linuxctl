package managers

import "testing"

// Compile-time check: every stub must implement Manager.
var (
	_ Manager = (*DiskManager)(nil)
	_ Manager = (*UserManager)(nil)
	_ Manager = (*PackageManager)(nil)
	_ Manager = (*ServiceManager)(nil)
	_ Manager = (*MountManager)(nil)
	_ Manager = (*SysctlManager)(nil)
	_ Manager = (*LimitsManager)(nil)
	_ Manager = (*FirewallManager)(nil)
	_ Manager = (*HostsManager)(nil)
	_ Manager = (*NetworkManager)(nil)
	_ Manager = (*SSHAuthManager)(nil)
	_ Manager = (*SELinuxManager)(nil)
	_ Manager = (*DirManager)(nil)
)

func TestManager_InterfaceCompliance(t *testing.T) {
	mgrs := []Manager{
		NewDiskManager(),
		NewUserManager(),
		NewPackageManager(),
		NewServiceManager(),
		NewMountManager(),
		NewSysctlManager(),
		NewLimitsManager(),
		NewFirewallManager(),
		NewHostsManager(),
		NewNetworkManager(),
		NewSSHAuthManager(),
		NewSELinuxManager(),
		NewDirManager(),
	}
	if got := len(mgrs); got != 13 {
		t.Fatalf("expected 13 managers, got %d", got)
	}
	seen := map[string]bool{}
	for _, m := range mgrs {
		n := m.Name()
		if n == "" {
			t.Errorf("%T has empty Name()", m)
		}
		if seen[n] {
			t.Errorf("duplicate manager name %q", n)
		}
		seen[n] = true
	}
}
