// SPDX-License-Identifier: MIT
//
// cmd/agt compare helpers (validCompareTarget + resolveCompareRoot +
// isAgeztRepoRoot + compareCapabilityTargets + compareEvidencePresent +
// compareEvidenceCounts).
// Extracted from compare.go during Day 211 god-file refactor (#72).
// Public API unchanged.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validCompareTarget(target string) bool {
	switch target {
	case compareTargetAll, compareTargetOpenClaw, compareTargetHermes:
		return true
	default:
		return false
	}
}
func resolveCompareRoot(rootArg string) (string, error) {
	if strings.TrimSpace(rootArg) != "" {
		abs, err := filepath.Abs(rootArg)
		if err != nil {
			return "", err
		}
		st, err := os.Stat(abs)
		if err != nil {
			return "", err
		}
		if !st.IsDir() {
			return "", fmt.Errorf("--root %s is not a directory", rootArg)
		}
		return abs, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if isAgeztRepoRoot(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd, nil
		}
	}
}
func isAgeztRepoRoot(dir string) bool {
	for _, p := range []string{"go.mod", "README.md", filepath.Join("cmd", "agt")} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			return false
		}
	}
	return true
}
func compareCapabilityTargets(cap compareCapability, target string) bool {
	if target == compareTargetAll {
		return true
	}
	for _, t := range cap.Targets {
		if t == target || t == compareTargetAll {
			return true
		}
	}
	return false
}
func compareEvidencePresent(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}
func compareEvidenceCounts(row compareAuditRow) (present, total int) {
	for _, ev := range row.Evidence {
		total++
		if ev.Present {
			present++
		}
	}
	return present, total
}
