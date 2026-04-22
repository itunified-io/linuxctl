package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// placeholderRe matches ${scheme:expr} with a non-greedy expr that may contain
// colons (vault paths like kv/foo:bar use them).
var placeholderRe = regexp.MustCompile(`\$\{([a-z]+):([^}]+)\}`)

// VaultReader is the hook an outer caller provides to resolve ${vault:…}.
// The default Resolver leaves vault placeholders untouched if Reader is nil.
type VaultReader interface {
	Read(path string) (string, error)
}

// Generator is the hook for ${gen:…} — e.g. random passwords. Nil = unsupported.
type Generator interface {
	Generate(spec string) (string, error)
}

// Resolver resolves ${env:…} / ${file:…} / ${vault:…} / ${gen:…} / ${ref:…}
// placeholders inside string values.
type Resolver struct {
	Vault   VaultReader
	Gen     Generator
	Refs    map[string]string // reserved for cross-document refs
	BaseDir string            // used to resolve relative ${file:…} paths
}

// NewResolver returns a resolver without external backends (env + file only).
func NewResolver() *Resolver {
	return &Resolver{Refs: map[string]string{}}
}

// Resolve substitutes every placeholder found in input. Unknown schemes or
// missing backends return an error.
func (r *Resolver) Resolve(input string) (string, error) {
	if !strings.Contains(input, "${") {
		return input, nil
	}
	var firstErr error
	out := placeholderRe.ReplaceAllStringFunc(input, func(match string) string {
		m := placeholderRe.FindStringSubmatch(match)
		if len(m) != 3 {
			return match
		}
		scheme, expr := m[1], m[2]
		v, err := r.lookup(scheme, expr)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return match
		}
		return v
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

func (r *Resolver) lookup(scheme, expr string) (string, error) {
	switch scheme {
	case "env":
		v, ok := os.LookupEnv(expr)
		if !ok {
			return "", fmt.Errorf("env var %s is not set", expr)
		}
		return v, nil
	case "file":
		path := expr
		if r.BaseDir != "" && !strings.HasPrefix(path, "/") {
			path = r.BaseDir + "/" + path
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("file %s: %w", path, err)
		}
		return strings.TrimRight(string(b), "\n"), nil
	case "vault":
		if r.Vault == nil {
			return "", fmt.Errorf("vault resolver not configured (expr=%s)", expr)
		}
		return r.Vault.Read(expr)
	case "gen":
		if r.Gen == nil {
			return "", fmt.Errorf("gen resolver not configured (expr=%s)", expr)
		}
		return r.Gen.Generate(expr)
	case "ref":
		if v, ok := r.Refs[expr]; ok {
			return v, nil
		}
		return "", fmt.Errorf("unknown ref %s", expr)
	default:
		return "", fmt.Errorf("unknown placeholder scheme %q", scheme)
	}
}

// resolveLinuxSecrets walks well-known string fields in Linux and substitutes
// any placeholders. Focused list — full reflection-based walk can come later.
func resolveLinuxSecrets(l *Linux, r *Resolver) error {
	if l == nil {
		return nil
	}
	if l.UsersGroups != nil {
		for i := range l.UsersGroups.Users {
			u := &l.UsersGroups.Users[i]
			if u.Password != "" {
				v, err := r.Resolve(u.Password)
				if err != nil {
					return fmt.Errorf("resolve password for user %s: %w", u.Name, err)
				}
				u.Password = v
			}
			for j, k := range u.SSHKeys {
				v, err := r.Resolve(k)
				if err != nil {
					return fmt.Errorf("resolve ssh_keys[%d] for user %s: %w", j, u.Name, err)
				}
				u.SSHKeys[j] = v
			}
		}
	}
	for i := range l.Mounts {
		m := &l.Mounts[i]
		if m.CredentialsVault != "" {
			v, err := r.Resolve(m.CredentialsVault)
			if err != nil {
				return fmt.Errorf("resolve credentials_vault for mount %s: %w", m.MountPoint, err)
			}
			m.CredentialsVault = v
		}
	}
	if l.SSHConfig != nil {
		for user, keys := range l.SSHConfig.AuthorizedKeys {
			for j, k := range keys {
				v, err := r.Resolve(k)
				if err != nil {
					return fmt.Errorf("resolve authorized_keys[%s][%d]: %w", user, j, err)
				}
				keys[j] = v
			}
			l.SSHConfig.AuthorizedKeys[user] = keys
		}
	}
	return nil
}
