// SPDX-License-Identifier: MIT

package warden

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestBuildContainerArgvWrapsShellCommand(t *testing.T) {
	dir := t.TempDir()
	argv, cliEnv, err := buildContainerArgv(Spec{
		Argv:    []string{"cmd", "/C", "echo hi"},
		WorkDir: dir,
		Env:     []string{"PATH=/usr/bin", "BAD", "=skip", "OK=value"},
		Limits:  Limits{AddressSpaceBytes: 1024},
	}, ContainerOptions{Enabled: true, Runtime: "podman", Image: "agezt/runtime:dev", Network: "bridge"})
	if err != nil {
		t.Fatalf("buildContainerArgv: %v", err)
	}
	abs, _ := filepath.Abs(dir)
	want := []string{
		"podman", "run", "--rm", "--network", "bridge",
		"-v", abs + ":/workspace", "-w", "/workspace",
		"-e", "PATH=/usr/bin", "-e", "OK=value",
		"--memory", "1024",
		"agezt/runtime:dev", "sh", "-lc", "echo hi",
	}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv = %#v\nwant %#v", argv, want)
	}
	if len(cliEnv) != 0 {
		t.Fatalf("cliEnv = %v, want empty (no secret-shaped variables)", cliEnv)
	}
}

// TestBuildContainerArgvKeepsSecretValuesOutOfArgv: a granted secret reaches
// the container by name, its value carried in the runtime CLI's environment.
// `-e NAME=VALUE` put it in argv — readable by every local user in the process
// list. Runtime-steering names stay inline and never enter the CLI's env.
func TestBuildContainerArgvKeepsSecretValuesOutOfArgv(t *testing.T) {
	const secret = "sk-live-value-must-not-appear"
	argv, cliEnv, err := buildContainerArgv(Spec{
		Argv: []string{"python", "main.py"},
		Env:  []string{"PATH=/usr/bin", "OPENAI_API_KEY=" + secret, "DOCKER_HOST_TOKEN=x"},
	}, ContainerOptions{Enabled: true, Runtime: "docker", Image: "img"})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range argv {
		if strings.Contains(a, secret) {
			t.Fatalf("secret value in argv: %#v", argv)
		}
	}
	if !slices.Contains(argv, "OPENAI_API_KEY") || !slices.Contains(cliEnv, "OPENAI_API_KEY="+secret) {
		t.Fatalf("secret not forwarded by name: argv=%#v cliEnv=%v", argv, cliEnv)
	}
	if !slices.Contains(argv, "DOCKER_HOST_TOKEN=x") || slices.ContainsFunc(cliEnv, func(kv string) bool { return strings.HasPrefix(kv, "DOCKER_") }) {
		t.Fatalf("runtime-steering name must stay inline and out of the CLI env: argv=%#v cliEnv=%v", argv, cliEnv)
	}
}

func TestContainerInnerArgvMapsHostInterpreterNames(t *testing.T) {
	got, err := containerInnerArgv(Spec{Argv: []string{`C:\Python312\python.exe`, "main.py"}})
	if err != nil {
		t.Fatalf("containerInnerArgv: %v", err)
	}
	want := []string{"python", "main.py"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
}

func TestContainerInnerArgvRemapsWorkdirPaths(t *testing.T) {
	dir := t.TempDir()
	deps := filepath.Join(dir, ".deps")
	got, err := containerInnerArgv(Spec{
		Argv:    []string{"/usr/bin/python3", "-m", "pip", "install", "--target", deps, "--flag=" + dir},
		WorkDir: dir,
	})
	if err != nil {
		t.Fatalf("containerInnerArgv: %v", err)
	}
	want := []string{"python", "-m", "pip", "install", "--target", "/workspace/.deps", "--flag=/workspace"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
}

func TestNewWithOptionsHonorsContainerEffectiveProfile(t *testing.T) {
	e := NewWithOptions(nil, Options{Container: ContainerOptions{Enabled: true, Runtime: "docker", Image: "python:3.12-slim"}})
	if got := e.EffectiveProfile(ProfileContainer); got != ProfileContainer {
		t.Fatalf("EffectiveProfile(container) = %s, want container", got)
	}
}
