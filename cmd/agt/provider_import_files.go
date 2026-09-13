// SPDX-License-Identifier: MIT

// Package main: file-parsing helpers for `agt provider import`
// (knownCredFiles builds the list of well-known CLI credential files;
// parseDotEnvFile reads .env into a name→value map; parseJSONCredFile reads
// JSON creds files picking the fields named in `names`). Extracted from
// provider_import.go during the Day-211 god-file split. Public API unchanged.
package main


import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type knownCredFile struct {
	label string
	path  string
	names map[string]string // json key (case-insensitive) -> canonical env name
}

// knownCredFiles lists the credential files popular agent CLIs leave on disk.
// Extend this table as new tools standardise their formats.
func knownCredFiles(home string) []knownCredFile {
	return []knownCredFile{
		{
			label: "codex",
			path:  filepath.Join(home, ".codex", "auth.json"),
			names: map[string]string{"openai_api_key": "OPENAI_API_KEY", "OPENAI_API_KEY": "OPENAI_API_KEY"},
		},
		{
			label: "gemini",
			path:  filepath.Join(home, ".gemini", "settings.json"),
			names: map[string]string{"gemini_api_key": "GEMINI_API_KEY", "api_key": "GEMINI_API_KEY"},
		},
	}
}

// parseDotEnvFile reads a `.env`-style file into name→value. It tolerates
// `export ` prefixes, `#` comments, blank lines, and single/double quotes.
func parseDotEnvFile(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = strings.TrimSuffix(val, "\r")
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key != "" {
			out[key] = val
		}
	}
	return out
}

// parseJSONCredFile extracts the named credential fields from a flat JSON
// object (best-effort; unreadable/invalid files contribute nothing).
func parseJSONCredFile(path string, names map[string]string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil
	}
	out := map[string]string{}
	for k, v := range obj {
		s, ok := v.(string)
		if !ok || s == "" {
			continue
		}
		if canonical, want := names[k]; want {
			out[canonical] = s
		} else if canonical, want := names[strings.ToLower(k)]; want {
			out[canonical] = s
		}
	}
	return out
}
