// SPDX-License-Identifier: MIT

package executionprofile

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/creds"
)

const (
	SecretFilesLocal  = "AGEZT_EXEC_SECRET_FILES_LOCAL"
	SecretFilesWarden = "AGEZT_EXEC_SECRET_FILES_WARDEN"
	SecretFilesDocker = "AGEZT_EXEC_SECRET_FILES_DOCKER"

	secretFilesDir = ".agezt-secrets"
	dockerWorkDir  = "/workspace"
)

type SecretFileMount struct {
	Key      string
	FileName string
	EnvName  string
}

func SecretFileMountsFromEnv(profile string) []SecretFileMount {
	return parseSecretFileMounts(os.Getenv(secretFilesVar(profile)))
}

func PrepareSecretFileMounts(baseDir, profile, workDir string) ([]string, func(), []string, error) {
	mounts := SecretFileMountsFromEnv(profile)
	if len(mounts) == 0 {
		return nil, func() {}, nil, nil
	}
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return nil, func() {}, nil, fmt.Errorf("secret file mounts require AGEZT baseDir")
	}
	store := creds.NewStore(baseDir)
	if err := store.Load(); err != nil {
		return nil, func() {}, nil, fmt.Errorf("load vault for secret file mounts: %w", err)
	}
	root, cleanupRoot, err := secretFileRoot(profile, workDir)
	if err != nil {
		return nil, func() {}, nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(root)
		if cleanupRoot != nil {
			cleanupRoot()
		}
	}
	env := make([]string, 0, len(mounts))
	names := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		value := store.Get(mount.Key)
		if value == "" {
			cleanup()
			return nil, func() {}, nil, fmt.Errorf("vault secret %q is configured for %s secret file mount but is not set", mount.Key, envProfileID(profile))
		}
		hostPath := filepath.Join(root, mount.FileName)
		if err := os.WriteFile(hostPath, []byte(value), 0o600); err != nil {
			cleanup()
			return nil, func() {}, nil, fmt.Errorf("write secret file mount %q: %w", mount.Key, err)
		}
		envPath := hostPath
		if envProfileID(profile) == "docker" {
			envPath = path.Join(dockerWorkDir, secretFilesDir, mount.FileName)
		}
		env = append(env, mount.EnvName+"="+envPath)
		names = append(names, mount.Key)
	}
	return env, cleanup, names, nil
}

func ProfileSecretFileSummary(profile, base string) string {
	mounts := SecretFileMountsFromEnv(profile)
	if len(mounts) == 0 {
		return base
	}
	names := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		names = append(names, mount.Key)
	}
	return base + "; vault-backed file mounts: " + strings.Join(names, ", ") + " (AGEZT_* blocked)"
}

func parseSecretFileMounts(raw string) []SecretFileMount {
	seen := map[string]bool{}
	var out []SecretFileMount
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, fileName, _ := strings.Cut(part, ":")
		key = strings.TrimSpace(key)
		fileName = strings.TrimSpace(fileName)
		if !validVaultSecretKey(key) {
			continue
		}
		up := strings.ToUpper(key)
		if strings.HasPrefix(up, "AGEZT_") || seen[up] {
			continue
		}
		if fileName == "" {
			fileName = safeSecretFileName(key)
		} else if !validSecretFileName(fileName) {
			continue
		}
		seen[up] = true
		out = append(out, SecretFileMount{
			Key:      key,
			FileName: fileName,
			EnvName:  "SECRET_FILE_" + safeEnvSuffix(key),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToUpper(out[i].Key) < strings.ToUpper(out[j].Key)
	})
	return out
}

func secretFileRoot(profile, workDir string) (string, func(), error) {
	if strings.TrimSpace(workDir) == "" {
		if envProfileID(profile) == "docker" {
			return "", nil, fmt.Errorf("docker secret file mounts require a tool workdir")
		}
		root, err := os.MkdirTemp("", "agezt-secret-files-")
		if err != nil {
			return "", nil, err
		}
		return root, func() { _ = os.RemoveAll(root) }, nil
	}
	root := filepath.Join(workDir, secretFilesDir)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", nil, err
	}
	return root, nil, nil
}

func secretFilesVar(profile string) string {
	switch envProfileID(profile) {
	case IDLocal:
		return SecretFilesLocal
	case "docker":
		return SecretFilesDocker
	default:
		return SecretFilesWarden
	}
}

func validVaultSecretKey(key string) bool {
	if key == "" {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			continue
		case r == '_', r == '-', r == '.', r == '#':
			continue
		default:
			return false
		}
	}
	return true
}

func validSecretFileName(name string) bool {
	return name != "" &&
		name != "." &&
		name != ".." &&
		!strings.ContainsAny(name, `/\:`) &&
		!strings.Contains(name, "\x00")
}

func safeSecretFileName(key string) string {
	out := safeEnvSuffix(key)
	if out == "" {
		return "secret"
	}
	return strings.ToLower(out)
}

func safeEnvSuffix(key string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToUpper(key) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}
