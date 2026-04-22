# Licensing

`linuxctl` is AGPL-3.0 open-source software. It runs forever without a
license for single-host Community-tier use. Commercial features are
unlocked by an Ed25519-signed JWT license, following the same tier model
as `proxctl`, `mcp-proxmox-enterprise`, and `mcp-postgres-enterprise`.

---

## 1. Tier matrix

| Feature                                         | Community | Business | Enterprise |
|-------------------------------------------------|:---------:|:--------:|:----------:|
| 13 subsystem managers                           | yes       | yes      | yes        |
| Single-host apply / verify / rollback           | yes       | yes      | yes        |
| Local + SSH session                             | yes       | yes      | yes        |
| `oracle-19c` preset                             | yes       | yes      | yes        |
| Drift detection (`linuxctl diff`)               | yes       | yes      | yes        |
| Fleet operations (`--all`, `--parallel`)        | no        | yes      | yes        |
| Advanced presets (`pg-16`, `hardened-cis`)      | no        | yes      | yes        |
| Custom user presets                             | no        | yes      | yes        |
| Persistent rollback via SQLite state            | no        | yes      | yes        |
| `linuxctl ssh setup-cluster` (cluster SSH bootstrap) | no   | yes      | yes        |
| RBAC (operator + approver roles)                | no        | no       | yes        |
| Audit log export (JSON, syslog, SIEM)           | no        | no       | yes        |
| SSO (OIDC / SAML operator authentication)       | no        | no       | yes        |
| Policy lifecycle (pre-apply approval, auto-rollback) | no   | no       | yes        |
| Air-gap license activation (offline verify)     | no        | partial  | yes        |
| Priority support (P1 < 4h, P2 < 24h)            | no        | yes      | yes        |

The same model is used across the itunified.io CLI product line.

---

## 2. License format

A license is a single-line JWT signed with the itunified.io Ed25519
license key. The payload is the public subset of the license record:

```json
{
  "iss": "itunified.io",
  "sub": "linuxctl-business",
  "aud": "linuxctl",
  "exp": 1777363200,
  "iat": 1745827200,
  "tier": "business",
  "features": ["fleet", "advanced-presets", "persistent-rollback", "cluster-ssh"],
  "customer": "acme-gmbh",
  "seats": 50
}
```

Signature verification uses the public key embedded at build time in
`pkg/license`. The binary itself is unsigned from the operator's
perspective; only the JWT is signed.

---

## 3. Loading a license

Default path: `~/.linuxctl/license.jwt`.

Alternatives, evaluated in order:

1. `--license <path>` flag.
2. `LINUXCTL_LICENSE_FILE` environment variable.
3. `~/.linuxctl/license.jwt`.
4. `/etc/linuxctl/license.jwt`.

Inspect:

```bash
linuxctl license show
linuxctl license verify
```

`license verify` validates signature + expiry + audience and prints a
concise decision (`valid`, `expired`, `invalid-signature`, `wrong-audience`).

---

## 4. Feature gating at runtime

Every Business / Enterprise feature is gated by a capability check:

```go
if err := license.Require(ctx, "fleet"); err != nil {
    return err   // TierRequiredError
}
```

The error is structured and surfaces in the CLI as:

```
Error: feature 'fleet' requires Business license — see https://itunified.io/linuxctl
```

`config validate` **does not** enforce feature gates. This lets your CI
validate manifests that reference Business-tier presets without having
the license loaded on every build runner.

---

## 5. Grace and expiry

- **Grace period:** 14 days after `exp`. The CLI warns on every
  invocation and blocks new applies only after the grace window closes.
- **Expired license:** all Business / Enterprise features return
  `LicenseExpiredError`. Community features continue to work.
- **Revoked license:** on next call to the online revocation list (if
  enabled), the license is rejected immediately. Offline / air-gap
  licenses are not revocation-checked — rely on the `exp` claim.

---

## 6. Air-gapped activation

Enterprise customers can request an offline-verifiable license with a
multi-year `exp`. The flow:

1. Customer generates a machine fingerprint on the operator host:

   ```bash
   linuxctl license fingerprint
   ```

2. Itunified.io issues a JWT bound to that fingerprint.
3. Customer copies the JWT to `~/.linuxctl/license.jwt` on every
   operator workstation.

The JWT encodes the fingerprint hash; the CLI compares at load time.

---

## 7. How to buy

- **Community:** free forever, no registration.
- **Business:** subscription, per-seat (operator workstation) pricing.
  Order via https://itunified.io/linuxctl/business.
- **Enterprise:** annual contract, unlimited seats, priority SLA.
  Contact sales@itunified.io.

AGPL-3.0 source remains free to copy, fork, and run. The license JWT
gates **our binary's** Business / Enterprise features; it does not
restrict your rights under AGPL.

---

## 8. FAQ

**Q: Can I run the Community tier on 1000 hosts?**
A: Yes, one at a time. Fleet flags (`--all`, `--parallel`) are Business.

**Q: Does a Business license on my workstation unlock it for teammates?**
A: No; each operator workstation needs its own license (seat).

**Q: What happens when my license expires mid-apply?**
A: The current apply completes (features evaluated at start of run).
Next invocation fails with `LicenseExpiredError`.

**Q: Can I write custom presets on Community?**
A: You can declare them in YAML and `config validate` passes, but apply
returns `TierRequiredError`. Upgrade to Business to unlock.

**Q: Is there a trial?**
A: Yes — 30-day Business trial via the itunified.io portal.
