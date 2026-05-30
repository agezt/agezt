// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/creds"
)

// cmdProviderImport implements `agt provider import` (SPEC-15 §1.3): discover
// API keys the operator already has on the machine — process environment, a
// local `.env`, and well-known CLI credential files (Codex, Gemini) — and offer
// to copy the recognised ones into the encrypted vault. Recognised = a name
// some synced catalog provider declares in its Env list; with `--all` any
// credential-shaped name is included. Values are never echoed (always masked);
// nothing is written without confirmation unless `--yes` is given.
//
// Offline by design: it writes the vault file directly, exactly like
// `provider creds set`. It prints the `provider reload` reminder so a running
// daemon can pick the keys up without a restart.
func cmdProviderImport(args []string, stdout, stderr io.Writer) int {
	var (
		fromPath          string
		assumeYes         bool
		dryRun            bool
		includeUnrecognis bool
		asJSON            bool
	)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s provider import [--from <file>] [--all] [--yes] [--dry-run] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "discover API keys already on this machine (env, .env, Codex/Gemini CLI) and store them in the vault\n")
			return 0
		case a == "--from":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s provider import: --from needs a path\n", brand.CLI)
				return 2
			}
			i++
			fromPath = args[i]
		case strings.HasPrefix(a, "--from="):
			fromPath = strings.TrimPrefix(a, "--from=")
		case a == "--all":
			includeUnrecognis = true
		case a == "--yes" || a == "-y":
			assumeYes = true
		case a == "--dry-run" || a == "-n":
			dryRun = true
		case a == "--json":
			asJSON = true
		default:
			fmt.Fprintf(stderr, "%s provider import: unknown argument %q\n", brand.CLI, a)
			return 2
		}
	}

	store, err := openCredsStore(stderr)
	if err != nil {
		return 1
	}

	// Recognised credential names come from the synced catalog. With no
	// catalog yet, fall back to the credential-shape heuristic so the command
	// is still useful on a fresh machine.
	recognised := map[string]string{} // env name -> provider id
	cat, _ := loadCatalogIfAny(stderr)
	if cat != nil {
		for _, p := range cat.ProviderList() {
			for _, env := range p.Env {
				if _, ok := recognised[env]; !ok {
					recognised[env] = p.ID
				}
			}
		}
	}
	if len(recognised) == 0 {
		includeUnrecognis = true
	}

	sources := defaultCredSources(fromPath)
	found := discoverCredentials(sources, recognised, includeUnrecognis)

	// Classify each against the current vault.
	type row struct {
		d      discovered
		status string // "new", "update", "unchanged"
	}
	var rows []row
	for _, d := range found {
		st := "new"
		if cur := store.Get(d.Name); cur != "" {
			if cur == d.Value {
				st = "unchanged"
			} else {
				st = "update"
			}
		}
		rows = append(rows, row{d: d, status: st})
	}

	if asJSON {
		out := make([]map[string]string, 0, len(rows))
		for _, r := range rows {
			out = append(out, map[string]string{
				"name": r.d.Name, "source": r.d.Source,
				"status": r.status, "provider": recognised[r.d.Name],
				"masked": creds.MaskValue(r.d.Value),
			})
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"discovered": out})
		return 0
	}

	if len(rows) == 0 {
		fmt.Fprintf(stdout, "no credentials discovered (looked in: env, .env, Codex/Gemini CLI files)\n")
		fmt.Fprintf(stdout, "tip: `%s provider import --all` widens the search to any *_API_KEY / *_TOKEN name\n", brand.CLI)
		return 0
	}

	fmt.Fprintf(stdout, "discovered %d credential%s:\n\n", len(rows), plural(len(rows), "", "s"))
	for _, r := range rows {
		pid := recognised[r.d.Name]
		if pid == "" {
			pid = "(unrecognised)"
		}
		fmt.Fprintf(stdout, "  %-28s %-10s %-12s %s  [%s]\n",
			r.d.Name, "["+r.status+"]", pid, creds.MaskValue(r.d.Value), r.d.Source)
	}
	fmt.Fprintln(stdout)

	if dryRun {
		fmt.Fprintf(stdout, "(dry run — nothing written)\n")
		return 0
	}

	stored := 0
	for _, r := range rows {
		if r.status == "unchanged" {
			continue
		}
		if !assumeYes {
			fmt.Fprintf(stdout, "store %s (%s)? [y/N]: ", r.d.Name, r.status)
			ans := strings.ToLower(strings.TrimSpace(readLine(stdin())))
			if ans != "y" && ans != "yes" {
				continue
			}
		}
		if err := store.Set(r.d.Name, r.d.Value); err != nil {
			fmt.Fprintf(stderr, "%s: %s: %v\n", brand.CLI, r.d.Name, err)
			continue
		}
		stored++
	}
	if stored == 0 {
		fmt.Fprintf(stdout, "nothing stored.\n")
		return 0
	}
	if err := store.Save(); err != nil {
		fmt.Fprintf(stderr, "%s: save vault: %v\n", brand.CLI, err)
		return 1
	}
	fmt.Fprintf(stdout, "stored %d credential%s in %s\n", stored, plural(stored, "", "s"), store.Path)
	fmt.Fprintf(stdout, "run `%s provider reload` to apply (or restart the daemon)\n", brand.CLI)
	return 0
}

// discovered is one credential found by a source.
type discovered struct {
	Name   string
	Value  string
	Source string
}

// credSource is a named provider of name→value candidates. Modelled as data
// (not a live scan) so tests can inject fixtures without touching the
// filesystem or process environment.
type credSource struct {
	Label  string
	Values map[string]string
}

// discoverCredentials merges the sources into a de-duplicated, sorted list.
// A name is kept when it is recognised (declared by a catalog provider) or,
// when includeUnrecognised is set, when it looks like a credential. The first
// source to supply a non-empty value wins (source order is the priority).
func discoverCredentials(sources []credSource, recognised map[string]string, includeUnrecognised bool) []discovered {
	seen := map[string]discovered{}
	for _, src := range sources {
		for name, val := range src.Values {
			if strings.TrimSpace(val) == "" {
				continue
			}
			if _, dup := seen[name]; dup {
				continue
			}
			_, isKnown := recognised[name]
			if !isKnown && !(includeUnrecognised && looksLikeCredName(name)) {
				continue
			}
			seen[name] = discovered{Name: name, Value: val, Source: src.Label}
		}
	}
	out := make([]discovered, 0, len(seen))
	for _, d := range seen {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// looksLikeCredName is the heuristic for `--all` / no-catalog mode: an
// upper-case name that names a key/token/secret.
func looksLikeCredName(name string) bool {
	if name != strings.ToUpper(name) {
		return false
	}
	for _, suf := range []string{"_API_KEY", "_API_TOKEN", "_ACCESS_KEY", "_SECRET_KEY", "_SECRET", "_TOKEN", "_KEY"} {
		if strings.HasSuffix(name, suf) {
			return true
		}
	}
	return false
}

// defaultCredSources assembles the real-machine sources, in priority order:
// process environment, a project-local `.env`, an explicit --from file, then
// well-known CLI credential files. Missing/unreadable sources contribute
// nothing (best-effort).
func defaultCredSources(fromPath string) []credSource {
	var sources []credSource

	// 1. Process environment.
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if eq := strings.IndexByte(kv, '='); eq > 0 {
			env[kv[:eq]] = kv[eq+1:]
		}
	}
	sources = append(sources, credSource{Label: "env", Values: env})

	// 2. Project-local .env (cwd).
	if v := parseDotEnvFile(".env"); len(v) > 0 {
		sources = append(sources, credSource{Label: ".env", Values: v})
	}

	// 3. Explicit --from file (parsed as .env).
	if fromPath != "" {
		sources = append(sources, credSource{Label: fromPath, Values: parseDotEnvFile(fromPath)})
	}

	// 4. Well-known CLI credential files (best-effort JSON).
	if home, err := os.UserHomeDir(); err == nil {
		for _, kf := range knownCredFiles(home) {
			if v := parseJSONCredFile(kf.path, kf.names); len(v) > 0 {
				sources = append(sources, credSource{Label: kf.label, Values: v})
			}
		}
	}
	return sources
}

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
