// SPDX-License-Identifier: MIT

package file

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)

const (
	rollbackCatalogVersion      = 1
	rollbackCatalogRelativePath = "rollback/checkpoints.json"
	rollbackCheckpointKindFile  = "file.snapshot"
)

type rollbackCatalog struct {
	Version     int                  `json:"version"`
	Checkpoints []rollbackCheckpoint `json:"checkpoints"`
}

type rollbackCheckpoint struct {
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	Action      string         `json:"action"`
	RunID       string         `json:"run_id,omitempty"`
	SubjectID   string         `json:"subject_id"`
	SubjectName string         `json:"subject_name,omitempty"`
	Before      map[string]any `json:"before,omitempty"`
	CreatedMS   int64          `json:"created_ms"`
}

func (t *Tool) checkpointFileSnapshot(ctx context.Context, action, relPath, absPath string) error {
	if strings.TrimSpace(t.rollbackBase) == "" {
		return nil
	}
	cp, err := newFileSnapshotCheckpoint(action, agent.CorrelationFromContext(ctx), relPath, absPath, time.Now())
	if err != nil {
		return err
	}
	return appendRollbackCheckpoint(filepath.Join(t.rollbackBase, filepath.FromSlash(rollbackCatalogRelativePath)), cp)
}

func newFileSnapshotCheckpoint(action, runID, relPath, absPath string, now time.Time) (rollbackCheckpoint, error) {
	before := map[string]any{
		"path":     relPath,
		"abs_path": absPath,
		"exists":   false,
	}
	info, err := os.Lstat(absPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return rollbackCheckpoint{}, err
		}
	} else {
		if info.Mode()&os.ModeSymlink != 0 {
			return rollbackCheckpoint{}, fmt.Errorf("refusing to checkpoint symlink at %s", relPath)
		}
		if info.IsDir() {
			return rollbackCheckpoint{}, fmt.Errorf("refusing to checkpoint directory at %s", relPath)
		}
		if info.Size() > MaxScanBytes {
			return rollbackCheckpoint{}, fmt.Errorf("%s is too large to checkpoint (%d bytes, max %d)", relPath, info.Size(), MaxScanBytes)
		}
		data, rerr := os.ReadFile(absPath)
		if rerr != nil {
			return rollbackCheckpoint{}, rerr
		}
		before["exists"] = true
		before["content_b64"] = base64.StdEncoding.EncodeToString(data)
		before["mode_perm"] = int(info.Mode().Perm())
		before["size"] = len(data)
	}
	return rollbackCheckpoint{
		ID:          fileCheckpointID(now, absPath),
		Kind:        rollbackCheckpointKindFile,
		Action:      action,
		RunID:       strings.TrimSpace(runID),
		SubjectID:   absPath,
		SubjectName: relPath,
		Before:      before,
		CreatedMS:   now.UnixMilli(),
	}, nil
}

func fileCheckpointID(now time.Time, absPath string) string {
	sum := sha256.Sum256([]byte(absPath))
	return fmt.Sprintf("rb-%d-file-%s", now.UnixNano(), hex.EncodeToString(sum[:])[:12])
}

func appendRollbackCheckpoint(path string, cp rollbackCheckpoint) error {
	cat, err := loadRollbackCatalog(path)
	if err != nil {
		return err
	}
	cat.Checkpoints = append(cat.Checkpoints, cp)
	return writeRollbackCatalog(path, cat)
}

func loadRollbackCatalog(path string) (rollbackCatalog, error) {
	cat := rollbackCatalog{Version: rollbackCatalogVersion}
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
		cat.Version = rollbackCatalogVersion
	}
	return cat, nil
}

func writeRollbackCatalog(path string, cat rollbackCatalog) error {
	if cat.Version == 0 {
		cat.Version = rollbackCatalogVersion
	}
	body, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			if removeErr := os.Remove(path); removeErr == nil {
				if retryErr := os.Rename(tmp, path); retryErr == nil {
					return nil
				} else {
					err = retryErr
				}
			}
		}
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
