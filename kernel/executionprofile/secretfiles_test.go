// SPDX-License-Identifier: MIT

package executionprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/creds"
)

func TestSecretFileMountsFromEnv(t *testing.T) {
	t.Setenv(SecretFilesDocker, "OPENAI_API_KEY:openai.key, AGEZT_WEB_PASSWORD, bad/name, GITHUB_TOKEN")
	mounts := SecretFileMountsFromEnv("docker")
	if len(mounts) != 2 {
		t.Fatalf("mount len = %d, want 2: %+v", len(mounts), mounts)
	}
	if mounts[0].Key != "GITHUB_TOKEN" || mounts[0].EnvName != "SECRET_FILE_GITHUB_TOKEN" {
		t.Fatalf("first mount = %+v", mounts[0])
	}
	if mounts[1].Key != "OPENAI_API_KEY" || mounts[1].FileName != "openai.key" {
		t.Fatalf("second mount = %+v", mounts[1])
	}
}

func TestPrepareSecretFileMountsWritesVaultFilesAndCleanup(t *testing.T) {
	baseDir := t.TempDir()
	workDir := t.TempDir()
	vault := creds.NewStore(baseDir)
	if err := vault.Load(); err != nil {
		t.Fatal(err)
	}
	if err := vault.Set("OPENAI_API_KEY", "sk-test-secret"); err != nil {
		t.Fatal(err)
	}
	if err := vault.Save(); err != nil {
		t.Fatal(err)
	}
	t.Setenv(SecretFilesDocker, "OPENAI_API_KEY:openai.key")

	env, cleanup, names, err := PrepareSecretFileMounts(baseDir, "docker", workDir)
	if err != nil {
		t.Fatalf("PrepareSecretFileMounts: %v", err)
	}
	defer cleanup()
	if strings.Join(names, ",") != "OPENAI_API_KEY" {
		t.Fatalf("names = %v", names)
	}
	if got := strings.Join(env, "\n"); !strings.Contains(got, "SECRET_FILE_OPENAI_API_KEY=/workspace/.agezt-secrets/openai.key") {
		t.Fatalf("env path should be container-visible, got:\n%s", got)
	}
	hostFile := filepath.Join(workDir, secretFilesDir, "openai.key")
	data, err := os.ReadFile(hostFile)
	if err != nil {
		t.Fatalf("read host secret file: %v", err)
	}
	if string(data) != "sk-test-secret" {
		t.Fatalf("secret file content = %q", data)
	}
	cleanup()
	if _, err := os.Stat(hostFile); !os.IsNotExist(err) {
		t.Fatalf("cleanup should remove secret file, stat err=%v", err)
	}
}

func TestPrepareSecretFileMountsFailsClosedOnMissingVaultKey(t *testing.T) {
	baseDir := t.TempDir()
	workDir := t.TempDir()
	vault := creds.NewStore(baseDir)
	if err := vault.Load(); err != nil {
		t.Fatal(err)
	}
	if err := vault.Save(); err != nil {
		t.Fatal(err)
	}
	t.Setenv(SecretFilesLocal, "MISSING_KEY")
	if _, _, _, err := PrepareSecretFileMounts(baseDir, IDLocal, workDir); err == nil || !strings.Contains(err.Error(), "MISSING_KEY") {
		t.Fatalf("expected missing key error, got %v", err)
	}
}
