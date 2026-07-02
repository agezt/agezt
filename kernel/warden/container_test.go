// SPDX-License-Identifier: MIT

package warden

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuildContainerArgvWrapsShellCommand(t *testing.T) {
	dir := t.TempDir()
	argv, err := buildContainerArgv(Spec{
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
