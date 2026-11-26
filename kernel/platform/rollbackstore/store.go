// SPDX-License-Identifier: MIT

// Package rollbackstore owns rollback checkpoint data, catalog persistence and
// the existing file snapshot restore primitive. Callers own policy and audit.
package rollbackstore

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/internal/atomicfile"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/contract/rollbackapi"
	"os"
	"path/filepath"
	"strings"
)

const (
	CatalogVersion = 1
	KindSkill      = "skill.status"
	KindFlow       = "workflow.snapshot"
	KindFile       = "file.snapshot"
	KindConfig     = "config.setting"
	RelativePath   = "rollback/checkpoints.json"
)

type Catalog struct {
	Version     int          `json:"version"`
	Checkpoints []Checkpoint `json:"checkpoints"`
}

// Checkpoint is one moment the daemon can return to.
//
// It is the CONTRACT's row (kernel/contract/rollbackapi): the console serves
// it and the platform's store writes it, so it is declared once — here — and
// this name is an alias of it. The two can never drift apart.
type Checkpoint = rollbackapi.Checkpoint

func RestoreFile(cp Checkpoint) (map[string]any, error) {
	before := cp.Before
	if len(before) == 0 {
		return nil, fmt.Errorf("checkpoint %s is missing file snapshot data", cp.ID)
	}
	path := StringValue(before["abs_path"])
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("checkpoint %s is missing abs_path", cp.ID)
	}
	exists, _ := before["exists"].(bool)
	if li, err := os.Lstat(path); err == nil {
		if li.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("refusing to restore through symlink at %s", path)
		}
		if li.IsDir() {
			return nil, fmt.Errorf("refusing to restore over directory at %s", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if !exists {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return map[string]any{"path": path, "restored": "absent"}, nil
	}
	encoded := StringValue(before["content_b64"])
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode content_b64: %w", err)
	}
	perm := os.FileMode(0o644)
	if n := intNumber(before["mode_perm"]); n > 0 {
		perm = os.FileMode(n)
	}
	if err := WriteSnapshotFile(path, data, perm); err != nil {
		return nil, err
	}
	return map[string]any{"path": path, "restored": "content", "bytes": len(data)}, nil
}

func DefaultPath() (string, error) {
	base, err := paths.BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, filepath.FromSlash(RelativePath)), nil
}

func Load() (Catalog, error) {
	path, err := DefaultPath()
	if err != nil {
		return Catalog{}, err
	}
	return LoadAt(path)
}

func LoadAt(path string) (Catalog, error) {
	cat := Catalog{Version: CatalogVersion}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cat, nil
		}
		return cat, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return cat, nil
	}
	if err := json.Unmarshal(raw, &cat); err != nil {
		return cat, err
	}
	if cat.Version == 0 {
		cat.Version = CatalogVersion
	}
	return cat, nil
}

func WriteAt(path string, cat Catalog) error {
	if cat.Version == 0 {
		cat.Version = CatalogVersion
	}
	body, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicfile.WriteFile(path, body, 0o600)
}

func Find(cat Catalog, id string) (int, *Checkpoint) {
	for i := range cat.Checkpoints {
		if cat.Checkpoints[i].ID == id {
			return i, &cat.Checkpoints[i]
		}
	}
	return -1, nil
}

// WriteSnapshotFile writes a restored file snapshot to path atomically with
// the given permissions, creating the parent directory when it is absent.
//
// It is exported because the CLI restores the same snapshots the daemon wrote:
// a second, hand-rolled atomic writer in the CLI was a second answer to the
// same question, and the two could drift (the CLI's had no Windows fallback).
func WriteSnapshotFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicfile.WriteFile(path, data, perm)
}

func StringValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return ""
	}
}

func intNumber(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	default:
		return 0
	}
}
