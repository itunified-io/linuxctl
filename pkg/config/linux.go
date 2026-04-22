package config

// Linux is the top-level linux.yaml manifest.
type Linux struct {
	Kind       string     `yaml:"kind" validate:"required,eq=LinuxHost"`
	APIVersion string     `yaml:"apiVersion" validate:"required"`
	Hosts      []HostSpec `yaml:"hosts"`
}

// HostSpec selects hosts and carries the desired spec.
type HostSpec struct {
	Selector Selector `yaml:"selector"`
	Spec     Spec     `yaml:"spec"`
}

// Selector joins against cluster.yaml roles/distros.
type Selector struct {
	Role   []string `yaml:"role,omitempty"`
	Distro []string `yaml:"distro,omitempty"`
}

// Spec is the Linux layer covered by the 13 managers. Fields are raw for now;
// manager-specific structs populate them in Phase 3.
type Spec struct {
	DiskLayout       any            `yaml:"disk_layout,omitempty"`
	UsersGroups      any            `yaml:"users_groups,omitempty"`
	Packages         any            `yaml:"packages,omitempty"`
	Services         any            `yaml:"services,omitempty"`
	Mounts           any            `yaml:"mounts,omitempty"`
	SysctlPreset     string         `yaml:"sysctl_preset,omitempty"`
	SysctlOverrides  map[string]any `yaml:"sysctl_overrides,omitempty"`
	LimitsPreset     string         `yaml:"limits_preset,omitempty"`
	Firewall         any            `yaml:"firewall,omitempty"`
	HostsFile        any            `yaml:"hosts_file,omitempty"`
	Network          any            `yaml:"network,omitempty"`
	SSH              any            `yaml:"ssh,omitempty"`
	SELinux          any            `yaml:"selinux,omitempty"`
	Dirs             []any          `yaml:"dirs,omitempty"`
	Directories      any            `yaml:"directories,omitempty"`
}
