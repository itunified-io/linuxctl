# Troubleshooting

Top 20 real-world issues encountered while operating `linuxctl`, grouped by
subsystem. Each entry includes the symptom, root cause, and a concrete fix.

General debugging:

- Add `-v` (verbose) to any command — dumps SSH transcripts and manager
  probe commands.
- Set `LINUXCTL_LOG=debug` — JSON log to stderr for machine parsing.
- `linuxctl apply plan --format json` shows every computed change.

---

## 1. SSH authentication fails

**Symptom:** `Error: ssh: handshake failed: unable to authenticate, attempted methods [none publickey], no supported methods remain`.

**Causes:**

- Operator's public key not in target's `~/.ssh/authorized_keys`.
- Wrong user (`linuxctl` defaults to the current local user; override
  with `--user <name>` or `ssh_config`).
- Target's `sshd_config` has `PubkeyAuthentication no`.

**Fix:** Verify manually with `ssh -v <user>@<host>`. Add the key or
adjust `sshd_config` (via the `ssh` manager once you have first-run access).

---

## 2. SSH host key mismatch

**Symptom:** `Error: ssh: host key verification failed`.

**Cause:** VM was re-created with a new host key; operator's
`~/.ssh/known_hosts` still caches the old one.

**Fix:** `ssh-keygen -R <host>` then `ssh-keyscan <host> >> ~/.ssh/known_hosts`.

---

## 3. Drift reported after a successful apply

**Symptom:** `linuxctl apply verify` returns non-zero immediately after a
successful `apply`.

**Causes (in order of likelihood):**

- The manager has an idempotency bug — open a `type:fix` issue.
- External automation (cloud-init, AWX, Ansible) is also writing to the
  same file and racing with linuxctl.
- The manifest declares an option in a canonical form but the target
  stores it in a variant form (e.g. `uid=1001` vs `uid=01001`).

**Fix:** Run `linuxctl <manager> plan <env>` — the diff will show the
exact drift. If it is a variant-form mismatch, normalize in the manifest.

---

## 4. LVM partial state

**Symptom:** A prior apply was interrupted; PV exists but VG does not, or
VG exists but no LVs. Next `disk apply` fails with `WARNING: Device
already present`.

**Cause:** LVM does not implement transactions; if linuxctl's SSH session
died between `pvcreate` and `vgcreate`, the PV is orphaned.

**Fix:** Clean up manually with `vgs`, `pvs`, `lvs`. Use `pvremove
/dev/sdX` or `vgremove <name>` to reset to a known state, then re-run
`linuxctl disk apply`.

---

## 5. Package manager lock contention

**Symptom:** `Error from package manager: Another app is currently holding
the yum lock; waiting for it to exit...` or `E: Could not get lock
/var/lib/dpkg/lock-frontend`.

**Cause:** Another process (`dnf-automatic`, unattended-upgrades,
another linuxctl, a human ssh session) is running.

**Fix:** Wait for the other process. `linuxctl` will retry with backoff
for up to 5 minutes by default. If you need to force, stop the other
process (`systemctl stop dnf-automatic.timer`) and retry.

---

## 6. `firewall-cmd --reload` fails

**Symptom:** `firewalld: ERROR: COMMAND_FAILED`.

**Cause:** A zone or service name in the manifest does not exist, or the
backend is nftables and a legacy `iptables` rule is inconsistent.

**Fix:** `firewall-cmd --get-zones` to confirm the zone; `firewall-cmd
--get-services` to confirm service names. Remove conflicting iptables
rules with `iptables -F` (RHEL 7 legacy hosts only).

---

## 7. SELinux enforcing -> disabled requires reboot

**Symptom:** Apply succeeds, but `getenforce` still reports `Enforcing`.

**Cause:** `selinux=0` on the kernel command line is required to fully
disable SELinux; a runtime setenforce is not enough.

**Fix:** `linuxctl` writes a `/etc/linuxctl/reboot-required` marker.
Reboot, then `linuxctl selinux verify` will pass.

---

## 8. Missing `sudo NOPASSWD`

**Symptom:** Commands hang indefinitely waiting for a password prompt.

**Cause:** The operator user has sudo but not NOPASSWD. `linuxctl` never
sends passwords.

**Fix:**

```
echo '<user> ALL=(ALL) NOPASSWD:ALL' | sudo tee /etc/sudoers.d/linuxctl
```

Or run as `root` directly with `--user root`.

---

## 9. Hostname change not visible to running services

**Symptom:** `hostnamectl status` shows the new hostname, but `postgres`
logs show the old one.

**Cause:** Many daemons cache the hostname at startup. `hostnamectl` does
not broadcast SIGHUP.

**Fix:** Restart the affected services. `linuxctl` does not do this
automatically; declare the relevant services with `state: restarted`
(roadmap) or restart out-of-band.

---

## 10. `mount` fails for CIFS: `cifs_mount failed w/return code = -22`

**Symptom:** Mount attempt rejected with `EINVAL`.

**Causes:**

- Wrong SMB dialect (`vers=3.1.1` required on newer Windows; older NAS
  needs `vers=2.0` or `vers=1.0`).
- Credentials file is missing, not root-only, or has Windows line
  endings.

**Fix:** Check `options`, ensure `/etc/cifs/*.cred` is mode `0600`, owner
root, Unix line endings.

---

## 11. `linuxctl config validate` says "preset not found"

**Symptom:** `Error: sysctl_preset 'pg-16' not found`.

**Cause:** `pg-16` is a Business-tier preset; the validator also checks
registry existence.

**Fix:** Either upgrade to Business and re-run, or pick `oracle-19c`,
or define the sysctl keys explicitly in `sysctl:` for now.

---

## 12. `authorized_keys` wiped after apply

**Symptom:** Keys added manually on the host disappear after `linuxctl
ssh apply`.

**Cause:** The `ssh` manager is declarative. It rewrites the entire
`authorized_keys` file from the manifest. Manual edits are drift.

**Fix:** Add the missing key to the `linux.yaml` `ssh.authorized_keys`
list and re-apply. Do not edit `authorized_keys` by hand on managed hosts.

---

## 13. `linuxctl apply` hangs on a specific manager

**Symptom:** Progress stops; no output for > 60s.

**Causes:**

- Long-running `dnf install` on a slow-mirror repo.
- Package post-install script prompts for input (very rare).
- SSH connection stalled (TCP keepalive not set).

**Fix:** Run with `-v` to see the exact command. Pass `--timeout 30m` to
extend the per-manager timeout. If the SSH session itself is dead, add
`ServerAliveInterval 30` to your operator `~/.ssh/config`.

---

## 14. Duplicate hosts_entries after apply

**Symptom:** `/etc/hosts` has duplicate lines outside the managed block.

**Cause:** An earlier manual edit placed the same entries outside the
`# BEGIN linuxctl / # END linuxctl` markers.

**Fix:** Remove the manual entries outside the block. `linuxctl` only
touches content inside the markers.

---

## 15. `service` manager reports drift after reboot

**Symptom:** On some hosts, `service verify` fails only after a reboot.

**Cause:** A service declared `state: running` but its unit file
`Requires=` a target that fails early in boot (e.g., a CIFS mount that
is not yet available).

**Fix:** Check unit dependencies with `systemctl list-dependencies
<unit>`. Declare the mount before the service (the DAG already orders
mount before service).

---

## 16. License error on every invocation

**Symptom:** `feature 'fleet' requires Business license` even though a
JWT is present.

**Causes:**

- JWT file has CRLF line endings (Windows edit).
- JWT in wrong location.
- `exp` claim has passed.

**Fix:** `linuxctl license verify` — it prints the exact error. For CRLF,
re-save the file with Unix line endings.

---

## 17. Firewall rule rollback leaves orphan services

**Symptom:** After `linuxctl apply rollback`, `firewall-cmd --list-services`
still shows a service added by the rolled-back run.

**Cause:** `linuxctl`'s in-session rollback reverses port / source rules
but does not unwind service references in the same zone (a common
surface-area issue across firewalld config tools).

**Fix:** Manually `firewall-cmd --permanent --zone=public --remove-service=<name>`,
then `firewall-cmd --reload`. File an issue if this trips you up
repeatedly — we treat it as a latent bug.

---

## 18. `linuxctl diff` reports drift that isn't really drift

**Symptom:** Trailing whitespace, comment differences, or ordering
diffs in managed files.

**Cause:** A third-party tool rewrote the file. linuxctl reads verbatim
and compares byte-for-byte on the managed block.

**Fix:** Identify the rewriter (`ls -la /etc/sysctl.d/`, compare mtime
against cron / package install times). Silence that automation or
declare the file OWNED BY linuxctl in your runbook.

---

## 19. `docker run ghcr.io/itunified-io/linuxctl` fails with permission errors

**Symptom:** `open /root/.ssh/id_ed25519: permission denied`.

**Cause:** The container runs as UID 0, but your mounted `~/.ssh` is
owned by UID 1000 with mode 0700. Docker for Mac / Linux rootless mode
may not share UIDs.

**Fix:** Mount with `-v $HOME/.ssh:/root/.ssh:ro,Z` and `chmod 0400` on
your private key, or use SSH agent forwarding via `-v
$SSH_AUTH_SOCK:/ssh-agent -e SSH_AUTH_SOCK=/ssh-agent`.

---

## 20. "unknown manager" error

**Symptom:** `Error: unknown manager: nic`.

**Cause:** NIC management (bonding, VLAN, static IP) is Phase 4b —
planned for `mcp-host-enterprise`, not `linuxctl`. The public CLI only
exposes the 13 managers in [`manager-reference.md`](manager-reference.md).

**Fix:** Use the `network` manager for hostname + resolv.conf today; use
Ansible / cloud-init / NetworkManager profiles for NIC config until
Phase 4b lands.

---

## Still stuck?

- Check the verbose output: `linuxctl ... -v 2>&1 | tee /tmp/linuxctl.log`.
- Open an issue with the log attached:
  https://github.com/itunified-io/linuxctl/issues/new
- Business / Enterprise customers: open a support ticket.
