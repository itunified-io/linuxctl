package config

// Linux is the top-level linux.yaml manifest — the typed Linux layer consumed
// by the 13 managers.
type Linux struct {
	Kind         string         `yaml:"kind" validate:"required,eq=Linux"`
	APIVersion   string         `yaml:"apiVersion,omitempty"`
	Hosts        []HostSpec     `yaml:"hosts,omitempty"`
	DiskLayout   *DiskLayout    `yaml:"disk_layout,omitempty"`
	UsersGroups  *UsersGroups   `yaml:"users_groups,omitempty"`
	Directories  []Directory    `yaml:"directories,omitempty" validate:"dive"`
	Mounts       []Mount        `yaml:"mounts,omitempty" validate:"dive"`
	Packages     *Packages      `yaml:"packages,omitempty"`
	Sysctl       []SysctlEntry  `yaml:"sysctl,omitempty" validate:"dive"`
	SysctlPreset string         `yaml:"sysctl_preset,omitempty"`
	Limits       []LimitEntry   `yaml:"limits,omitempty" validate:"dive"`
	LimitsPreset string         `yaml:"limits_preset,omitempty"`
	// DirectoriesPreset, UsersGroupsPreset, PackagesPreset, BundlePreset:
	// plan 033 conventions library. Resolved at config-load time (bundle)
	// and per-manager Plan() (individual *_preset fields).
	DirectoriesPreset string `yaml:"directories_preset,omitempty"`
	UsersGroupsPreset string `yaml:"users_groups_preset,omitempty"`
	PackagesPreset    string `yaml:"packages_preset,omitempty"`
	BundlePreset      string `yaml:"bundle_preset,omitempty"`
	Firewall     *Firewall      `yaml:"firewall,omitempty"`
	HostsEntries []HostEntry    `yaml:"hosts_entries,omitempty" validate:"dive"`
	Services     []ServiceState `yaml:"services,omitempty" validate:"dive"`
	SSHConfig    *SSHConfig     `yaml:"ssh,omitempty"`
	SELinux      *SELinuxConfig `yaml:"selinux,omitempty"`
}

// HostSpec is retained for backward compatibility with the scaffold layout —
// a manifest may pin per-host selectors and carry a host-scoped Spec override.
type HostSpec struct {
	Selector Selector `yaml:"selector"`
	Spec     Spec     `yaml:"spec"`
}

// Selector joins against cluster.yaml roles / distros.
type Selector struct {
	Role   []string `yaml:"role,omitempty"`
	Distro []string `yaml:"distro,omitempty"`
}

// Spec is the host-scoped desired state. Alias over Linux so both the
// top-level manifest and per-host overrides share one schema.
type Spec = Linux

// DiskLayout describes root + additional-disk storage.
type DiskLayout struct {
	Root       *RootDisk        `yaml:"root,omitempty"`
	Additional []AdditionalDisk `yaml:"additional,omitempty" validate:"dive"`
}

// RootDisk describes the root disk / root VG.
type RootDisk struct {
	Device         string          `yaml:"device" validate:"required"`
	VGName         string          `yaml:"vg_name,omitempty"`
	LogicalVolumes []LogicalVolume `yaml:"logical_volumes,omitempty" validate:"dive"`
}

// AdditionalDisk is a non-root disk resolved either by device path or by role
// tag (which linuxctl looks up in hypervisor.disks).
type AdditionalDisk struct {
	Device         string          `yaml:"device,omitempty"`
	Role           string          `yaml:"role,omitempty"`
	Tag            string          `yaml:"tag,omitempty"`
	VGName         string          `yaml:"vg_name" validate:"required"`
	LogicalVolumes []LogicalVolume `yaml:"logical_volumes" validate:"min=1,dive"`
}

// LogicalVolume describes a single LV carved from the parent VG.
type LogicalVolume struct {
	Name       string `yaml:"name" validate:"required"`
	MountPoint string `yaml:"mount_point" validate:"required,startswith=/"`
	Size       string `yaml:"size" validate:"required"`
	FS         string `yaml:"fs" validate:"required,oneof=xfs ext4 btrfs"`
}

// UsersGroups declares managed groups + users.
type UsersGroups struct {
	Groups []Group `yaml:"groups,omitempty" validate:"dive"`
	Users  []User  `yaml:"users,omitempty" validate:"dive"`
}

// Group is a managed POSIX group.
type Group struct {
	Name string `yaml:"name" validate:"required"`
	GID  int    `yaml:"gid,omitempty" validate:"omitempty,gte=1000"`
}

// User is a managed POSIX user.
type User struct {
	Name     string   `yaml:"name" validate:"required"`
	UID      int      `yaml:"uid,omitempty" validate:"omitempty,gte=1000"`
	GID      string   `yaml:"gid,omitempty"`
	Groups   []string `yaml:"groups,omitempty"`
	Home     string   `yaml:"home,omitempty"`
	Shell    string   `yaml:"shell,omitempty"`
	SSHKeys  []string `yaml:"ssh_keys,omitempty"`
	Password string   `yaml:"password,omitempty"`
}

// Directory is a managed filesystem directory.
type Directory struct {
	Path      string `yaml:"path" validate:"required,startswith=/"`
	Owner     string `yaml:"owner,omitempty"`
	Group     string `yaml:"group,omitempty"`
	Mode      string `yaml:"mode,omitempty" validate:"omitempty,len=4"`
	Recursive bool   `yaml:"recursive,omitempty"`
}

// Mount describes a persistent or transient mount.
type Mount struct {
	Type             string   `yaml:"type" validate:"required,oneof=cifs nfs bind tmpfs"`
	Source           string   `yaml:"source,omitempty"`
	Server           string   `yaml:"server,omitempty"`
	Share            string   `yaml:"share,omitempty"`
	MountPoint       string   `yaml:"mount_point" validate:"required,startswith=/"`
	Options          []string `yaml:"options,omitempty"`
	CredentialsVault string   `yaml:"credentials_vault,omitempty"`
	Persistent       bool     `yaml:"persistent,omitempty"`
}

// Packages describes install / remove / service-state package operations.
type Packages struct {
	Install          []string `yaml:"install,omitempty"`
	Remove           []string `yaml:"remove,omitempty"`
	EnabledServices  []string `yaml:"enabled_services,omitempty"`
	DisabledServices []string `yaml:"disabled_services,omitempty"`
}

// SysctlEntry is a single sysctl key/value pair.
type SysctlEntry struct {
	Key   string `yaml:"key" validate:"required"`
	Value string `yaml:"value" validate:"required"`
}

// LimitEntry is a single /etc/security/limits.d entry.
type LimitEntry struct {
	User  string `yaml:"user" validate:"required"`
	Type  string `yaml:"type" validate:"required,oneof=soft hard"`
	Item  string `yaml:"item" validate:"required"`
	Value string `yaml:"value" validate:"required"`
}

// Firewall is the firewalld / nftables config block.
type Firewall struct {
	Enabled bool                    `yaml:"enabled,omitempty"`
	Zones   map[string]FirewallZone `yaml:"zones,omitempty"`
}

// FirewallZone is a single firewalld zone.
type FirewallZone struct {
	Ports              []PortRule `yaml:"ports,omitempty" validate:"dive"`
	Sources            []string   `yaml:"sources,omitempty"`
	SourcesFromNetwork string     `yaml:"sources_from_network,omitempty"`
}

// PortRule is a single accepted port.
type PortRule struct {
	Name  string `yaml:"name,omitempty"`
	Port  int    `yaml:"port,omitempty"`
	Range string `yaml:"port_range,omitempty"`
	Proto string `yaml:"proto" validate:"required,oneof=tcp udp"`
}

// HostEntry is a single /etc/hosts line.
type HostEntry struct {
	IP    string   `yaml:"ip" validate:"required,ip"`
	Names []string `yaml:"names" validate:"required,min=1"`
}

// ServiceState is a desired systemd service state.
type ServiceState struct {
	Name    string `yaml:"name" validate:"required"`
	Enabled bool   `yaml:"enabled,omitempty"`
	State   string `yaml:"state,omitempty" validate:"omitempty,oneof=running stopped"`
}

// SSHConfig captures authorized-key drift + sshd_config keys.
type SSHConfig struct {
	AuthorizedKeys map[string][]string `yaml:"authorized_keys,omitempty"`
	SSHDConfig     map[string]string   `yaml:"sshd_config,omitempty"`
}

// SELinuxConfig captures SELinux mode + boolean overrides.
type SELinuxConfig struct {
	Mode     string          `yaml:"mode,omitempty" validate:"omitempty,oneof=enforcing permissive disabled"`
	Booleans map[string]bool `yaml:"booleans,omitempty"`
}
