// SPDX-License-Identifier: MIT

package testfixtures

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// EnvProbePluginSource is a complete stdio plugin with one tool, "getenv",
// that reports the value of the variable named in its input as seen by the
// plugin PROCESS — the thing under test is what the host hands the child.
const EnvProbePluginSource = `package main

import (
	"bufio"
	"encoding/json"
	"os"
)

type req struct {
	ID     string          ` + "`json:\"id\"`" + `
	Method string          ` + "`json:\"method\"`" + `
	Params json.RawMessage ` + "`json:\"params\"`" + `
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	for in.Scan() {
		var r req
		if json.Unmarshal(in.Bytes(), &r) != nil {
			continue
		}
		switch r.Method {
		case "initialize":
			out.Encode(map[string]any{"id": r.ID, "result": map[string]any{
				"protocol_version": 1,
				"tools": []map[string]any{{"name": "getenv", "description": "probe",
					"input_schema": map[string]any{"type": "object"}, "capability": "introspect"}},
			}})
		case "tool/invoke":
			var p struct {
				Input struct {
					Name string ` + "`json:\"name\"`" + `
				} ` + "`json:\"input\"`" + `
			}
			json.Unmarshal(r.Params, &p)
			out.Encode(map[string]any{"id": r.ID, "result": map[string]any{"output": os.Getenv(p.Input.Name)}})
		case "shutdown":
			out.Encode(map[string]any{"id": r.ID, "result": map[string]any{}})
			return
		}
	}
}
`

// BuildEnvProbePlugin compiles the env-probe plugin into dir and returns the
// binary's path.
//
// It is shared on purpose: the plugin side measures the ENVIRONMENT it asks
// for (the request it hands the host), and the host side measures the
// ENVIRONMENT the child process really gets. The two halves of that promise
// are only worth anything if both are measured — so both use one probe.
//
// The caller owns the directory (a test's t.TempDir() is the right one) and
// the error.
func BuildEnvProbePlugin(dir string) (string, error) {
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(EnvProbePluginSource), 0o644); err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "envprobe")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "main.go")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build env probe plugin: %v\n%s", err, out)
	}
	return bin, nil
}
