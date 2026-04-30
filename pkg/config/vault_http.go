package config

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// HTTPVaultReader is a config.VaultReader that talks to HashiCorp Vault over
// HTTP. Reads VAULT_ADDR (default https://vault.int.itunified.io) and
// VAULT_TOKEN from the environment per ADR-0042. Path syntax in placeholders:
//
//	${vault:<path>#<field>}      → GET v1/<path>, return data.data[<field>]
//	${vault:<path>}              → GET v1/<path>, return data.data as JSON blob
//
// Used by:
//   - LoadEnv (ssh_keys, password, etc. inline ${vault:…} placeholders)
//   - MountManager (CIFS credentials_vault dereference)
type HTTPVaultReader struct {
	Addr   string
	Token  string
	client *http.Client
}

// NewHTTPVaultReader returns a VaultReader configured from environment.
// If VAULT_TOKEN is unset, Read calls will return an error pointing at
// the missing config — but constructing the reader is harmless.
func NewHTTPVaultReader() *HTTPVaultReader {
	addr := strings.TrimRight(os.Getenv("VAULT_ADDR"), "/")
	if addr == "" {
		addr = "https://vault.int.itunified.io"
	}
	insecure := strings.EqualFold(os.Getenv("VAULT_SKIP_VERIFY"), "true") ||
		os.Getenv("VAULT_SKIP_VERIFY") == "1"
	return &HTTPVaultReader{
		Addr:  addr,
		Token: os.Getenv("VAULT_TOKEN"),
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
			},
		},
	}
}

// Read implements VaultReader.
func (r *HTTPVaultReader) Read(expr string) (string, error) {
	if r == nil || r.Token == "" {
		return "", fmt.Errorf("vault: VAULT_TOKEN env var not set (cannot resolve ${vault:%s})", expr)
	}
	path, field := expr, ""
	if i := strings.Index(expr, "#"); i >= 0 {
		path = expr[:i]
		field = expr[i+1:]
	}
	url := fmt.Sprintf("%s/v1/%s", r.Addr, strings.TrimLeft(path, "/"))
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("vault: build request: %w", err)
	}
	req.Header.Set("X-Vault-Token", r.Token)
	req.Header.Set("Accept", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vault: GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("vault: GET %s: status %d: %s", url, resp.StatusCode, string(body))
	}
	// KV v2 nests the data under data.data.<field>. KV v1 puts it at
	// data.<field>. Try v2 first, fall back to v1.
	var v2 struct {
		Data struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &v2); err == nil && v2.Data.Data != nil {
		if field == "" {
			out, _ := json.Marshal(v2.Data.Data)
			return string(out), nil
		}
		v, ok := v2.Data.Data[field]
		if !ok {
			return "", fmt.Errorf("vault: %s has no field %q", path, field)
		}
		return vaultToString(v)
	}
	var v1 struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &v1); err == nil && v1.Data != nil {
		if field == "" {
			out, _ := json.Marshal(v1.Data)
			return string(out), nil
		}
		v, ok := v1.Data[field]
		if !ok {
			return "", fmt.Errorf("vault: %s has no field %q", path, field)
		}
		return vaultToString(v)
	}
	return "", fmt.Errorf("vault: %s: response has no data field", path)
}

func vaultToString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case float64:
		return fmt.Sprintf("%v", x), nil
	case bool:
		return fmt.Sprintf("%v", x), nil
	case nil:
		return "", nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}
