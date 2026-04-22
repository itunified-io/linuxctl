package root

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd_Runs(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc", Date: "now"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("root --help: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"disk", "user", "package", "service", "mount", "sysctl", "limits", "firewall", "hosts", "network", "ssh", "selinux", "dir", "apply"} {
		if !strings.Contains(out, want) {
			t.Errorf("--help missing subcommand %q\n%s", want, out)
		}
	}
}

func TestVersionCmd_Runs(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "v0.0.0", Commit: "c", Date: "d"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(buf.String(), "linuxctl v0.0.0") {
		t.Errorf("version output unexpected: %s", buf.String())
	}
}
