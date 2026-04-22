// Package license provides the tool catalog and license-gate helpers for linuxctl.
//
// Real license verification (Ed25519 JWT, seat counting, CRL) is delegated to
// github.com/itunified-io/dbx/pkg/core/license in later phases. This scaffold
// exposes the tool catalog and a no-op Check for Community-tier commands.
package license

import "context"

// Tier names a license tier. Keep in sync with dbx/pkg/core/license.
type Tier string

const (
	TierCommunity  Tier = "community"
	TierBusiness   Tier = "business"
	TierEnterprise Tier = "enterprise"
)

// Caps describes the capabilities required by a handler.
type Caps struct {
	Tool     string
	Op       string
	Tier     Tier
	Features []string
}

// Tool describes a gated surface in the linuxctl catalog.
type Tool struct {
	Name string
	Tier Tier
}

// ToolCatalog enumerates the 35 community-tier surfaces covered by the public
// MCP wrapper plus Business / Enterprise extensions. Used by smoke tests and
// the MCP adapter for parity checks.
var ToolCatalog = []Tool{
	// config (3)
	{"linuxctl_config_validate", TierCommunity},
	{"linuxctl_config_render", TierCommunity},
	{"linuxctl_config_use_context", TierCommunity},
	// env (7)
	{"linuxctl_env_new", TierCommunity},
	{"linuxctl_env_list", TierCommunity},
	{"linuxctl_env_use", TierCommunity},
	{"linuxctl_env_current", TierCommunity},
	{"linuxctl_env_add", TierCommunity},
	{"linuxctl_env_remove", TierCommunity},
	{"linuxctl_env_show", TierCommunity},
	// per-manager curated subset (13 plan + selected verbs → 18 in MCP)
	{"linuxctl_disk_plan", TierCommunity},
	{"linuxctl_user_plan", TierCommunity},
	{"linuxctl_package_plan", TierCommunity},
	{"linuxctl_service_plan", TierCommunity},
	{"linuxctl_mount_plan", TierCommunity},
	{"linuxctl_sysctl_plan", TierCommunity},
	{"linuxctl_limits_plan", TierCommunity},
	{"linuxctl_firewall_plan", TierCommunity},
	{"linuxctl_hosts_plan", TierCommunity},
	{"linuxctl_network_plan", TierCommunity},
	{"linuxctl_ssh_plan", TierCommunity},
	{"linuxctl_selinux_plan", TierCommunity},
	{"linuxctl_dir_plan", TierCommunity},
	// apply (4)
	{"linuxctl_apply_plan", TierCommunity},
	{"linuxctl_apply_apply", TierCommunity},
	{"linuxctl_apply_verify", TierCommunity},
	{"linuxctl_apply_rollback", TierCommunity},
	// diff (1)
	{"linuxctl_diff", TierCommunity},
	// license (3)
	{"linuxctl_license_status", TierCommunity},
	{"linuxctl_license_activate", TierCommunity},
	{"linuxctl_license_show", TierCommunity},
	// Business (+5)
	{"linuxctl_fleet_apply", TierBusiness},
	{"linuxctl_fleet_rollback", TierBusiness},
	{"linuxctl_preset_list", TierBusiness},
	{"linuxctl_preset_apply", TierBusiness},
	{"linuxctl_drift_auto_remediate", TierBusiness},
	// Enterprise (+8)
	{"linuxctl_state_sync_central", TierEnterprise},
	{"linuxctl_audit_export", TierEnterprise},
	{"linuxctl_rbac_role_add", TierEnterprise},
	{"linuxctl_rbac_bind_add", TierEnterprise},
	{"linuxctl_selinux_policy_sync", TierEnterprise},
	{"linuxctl_apparmor_policy_sync", TierEnterprise},
	{"linuxctl_airgap_bundle", TierEnterprise},
	{"linuxctl_airgap_apply", TierEnterprise},
}

// Check is the scaffold license gate. It always allows Community operations
// and refuses Business / Enterprise ones with a clear error — real verification
// is wired up in Phase 3.
func Check(ctx context.Context, caps Caps) error {
	_ = ctx
	switch caps.Tier {
	case "", TierCommunity:
		return nil
	default:
		return ErrTierNotActive{Tier: caps.Tier, Op: caps.Op}
	}
}

// ErrTierNotActive is returned when a handler requires a tier that is not yet
// active in this scaffold build.
type ErrTierNotActive struct {
	Tier Tier
	Op   string
}

func (e ErrTierNotActive) Error() string {
	return "license: tier " + string(e.Tier) + " not active for op " + e.Op + " (scaffold build)"
}
