// SPDX-License-Identifier: MIT

package coding

import (
	"flag"
	"os"
	"strings"
	"testing"
)

// TestGitChildDoesNotInheritSecrets: the coding tool runs git in the agent's
// repository, and git runs that repository's hooks — attacker-controlled code
// when the repo is. execCommand with no explicit env used to leave os/exec's
// default, which is the daemon's whole environment: every provider key, the
// vault passphrase. The child (here: this test binary standing in for git)
// must see none of them.
func TestGitChildDoesNotInheritSecrets(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-must-not-leak")
	t.Setenv("AGEZT_VAULT_PASSPHRASE", "must-not-leak")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	out, err := execCommand(t.Context(), t.TempDir(), nil, self, "-test.run=^TestEnvProbeHelper$", "--", "print-env")
	if err != nil {
		t.Fatalf("helper: %v\n%s", err, out)
	}
	if strings.Contains(out, "must-not-leak") {
		t.Fatalf("a daemon secret reached the git child:\n%s", out)
	}
	if !strings.Contains(strings.ToUpper(out), "PATH=") {
		t.Fatalf("helper printed no environment (did it run?):\n%s", out)
	}
}

// TestEnvProbeHelper is the child half: it prints its environment.
func TestEnvProbeHelper(t *testing.T) {
	if flag.Arg(0) != "print-env" {
		t.Skip("only runs as a helper process")
	}
	os.Stdout.WriteString(strings.Join(os.Environ(), "\n") + "\n")
}
