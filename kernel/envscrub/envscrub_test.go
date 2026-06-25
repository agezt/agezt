// SPDX-License-Identifier: MIT

package envscrub

import (
	"os"
	"strings"
	"testing"
)

func TestScrubbedDropsDaemonAndSecretNames(t *testing.T) {
	t.Setenv("AGEZT_SECRET_PROBE", "leakme")
	t.Setenv("MY_API_KEY", "leakme")
	t.Setenv("SERVICE_TOKEN", "leakme")
	t.Setenv("AWS_ACCESS_KEY_ID", "leakme")
	t.Setenv("PATH", os.Getenv("PATH"))

	for _, kv := range Scrubbed() {
		up := strings.ToUpper(kv)
		if strings.HasPrefix(up, "AGEZT_") ||
			strings.HasPrefix(up, "MY_API_KEY=") ||
			strings.HasPrefix(up, "SERVICE_TOKEN=") ||
			strings.HasPrefix(up, "AWS_ACCESS_KEY_ID=") {
			t.Fatalf("secret leaked into child env: %s", kv)
		}
	}
}

func TestScrubbedKeepsLaunchVariables(t *testing.T) {
	t.Setenv("PATH", os.Getenv("PATH"))
	joined := strings.ToUpper(strings.Join(Scrubbed(), "\n"))
	if !strings.Contains(joined, "PATH=") {
		t.Fatal("PATH missing from child env")
	}
}

func TestWithAppendsExplicitValues(t *testing.T) {
	env := With([]string{"PATH=x"}, "AGEZT_CODING_TASK=hello")
	if got, want := env[len(env)-1], "AGEZT_CODING_TASK=hello"; got != want {
		t.Fatalf("explicit env append = %q, want %q", got, want)
	}
}
