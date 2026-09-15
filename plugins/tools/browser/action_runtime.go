// SPDX-License-Identifier: MIT
//
// browser.action runtime helpers: runActionDriver (the subprocess invocation)
// + limitedBuffer + normalizeActionOutput + attachArtifacts +
// saveActionArtifact + actionArtifactRecord + isBrowserActionTempPath +
// truncateUTF8 + truncateActionOutput + errResult + ResolveActionDriverPath.
// Split from action.go during Day 211 god-file refactor (#38).
// Public API unchanged.
package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	stdhttp "net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/envscrub"
)

func runActionDriver(ctx context.Context, spec actionRunSpec) (actionRunOutput, error) {
	cmd := exec.CommandContext(ctx, spec.NodePath, spec.DriverPath)
	cmd.Dir = spec.Dir
	cmd.Env = envscrub.Scrubbed()
	cmd.Stdin = bytes.NewReader(spec.Spec)
	var stdout, stderr limitedBuffer
	stdout.max = MaxActionDriverOutputBytes
	stderr.max = MaxActionDriverOutputBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return actionRunOutput{Stdout: stdout.String(), Stderr: stderr.String()}, err
}

type limitedBuffer struct {
	bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.max <= 0 || b.Len() >= b.max {
		return len(p), nil
	}
	keep := b.max - b.Len()
	if len(p) > keep {
		_, _ = b.Buffer.Write(p[:keep])
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

func normalizeActionOutput(stdout string, maxTextChars int) (string, string, error) {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return "", "", errors.New("empty stdout")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		return "", "", err
	}
	if ok, _ := out["ok"].(bool); !ok {
		if msg, _ := out["error"].(string); msg != "" {
			return "", "", errors.New(msg)
		}
		return "", "", errors.New("driver reported ok=false")
	}
	if text, _ := out["text"].(string); text != "" && maxTextChars > 0 && len(text) > maxTextChars {
		out["text"] = truncateUTF8(text, maxTextChars) + "\n\n...[truncated]"
		out["truncated_text"] = true
	}
	source, _ := out["url"].(string)
	enc, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", "", err
	}
	return string(enc), source, nil
}

func (t *ActionTool) attachArtifacts(normalized string) string {
	if t == nil || t.index == nil {
		return normalized
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(normalized), &out); err != nil {
		return normalized
	}
	var artifacts []map[string]any
	if screenshot, _ := out["screenshot"].(string); strings.TrimSpace(screenshot) != "" {
		if e, err := t.saveActionArtifact(screenshot, "image", "browser-action-screenshot.png", "browser.action screenshot"); err == nil {
			rec := actionArtifactRecord(e)
			out["screenshot_artifact"] = rec
			artifacts = append(artifacts, rec)
		} else {
			out["screenshot_artifact_error"] = err.Error()
		}
	}
	if downloads, _ := out["downloads"].([]any); len(downloads) > 0 {
		for _, item := range downloads {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			p, _ := row["path"].(string)
			if strings.TrimSpace(p) == "" {
				continue
			}
			name, _ := row["suggested_filename"].(string)
			if e, err := t.saveActionArtifact(p, "", name, "browser.action download"); err == nil {
				rec := actionArtifactRecord(e)
				row["artifact"] = rec
				artifacts = append(artifacts, rec)
			} else {
				row["artifact_error"] = err.Error()
			}
		}
	}
	if len(artifacts) > 0 {
		out["artifacts"] = artifacts
	}
	enc, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return normalized
	}
	return string(enc)
}

func (t *ActionTool) saveActionArtifact(path, forcedKind, name, caption string) (artifact.Entry, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return artifact.Entry{}, errors.New("empty path")
	}
	if !isBrowserActionTempPath(path) {
		return artifact.Entry{}, fmt.Errorf("refusing to save browser output outside browseruse temp dir: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return artifact.Entry{}, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) == 0 {
		return artifact.Entry{}, fmt.Errorf("read %s: empty file", path)
	}
	cleanName := filepath.Base(strings.TrimSpace(name))
	if cleanName == "." || cleanName == string(filepath.Separator) || cleanName == "" {
		cleanName = filepath.Base(path)
	}
	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(cleanName)))
	if mimeType == "" {
		mimeType = stdhttp.DetectContentType(data)
	}
	kind := forcedKind
	if kind == "" {
		if strings.HasPrefix(mimeType, "image/") {
			kind = "image"
		} else {
			kind = "download"
		}
	}
	now := time.Now().UnixMilli()
	if t.Now != nil {
		now = t.Now()
	}
	return t.index.PutEntry(artifact.Entry{
		Kind:    kind,
		Source:  "browser.action",
		Name:    cleanName,
		Mime:    mimeType,
		Caption: caption,
	}, data, now)
}

func actionArtifactRecord(e artifact.Entry) map[string]any {
	return map[string]any{
		"id":   e.ID,
		"ref":  e.Ref,
		"name": e.Name,
		"mime": e.Mime,
		"kind": e.Kind,
		"size": e.Size,
	}
}

func isBrowserActionTempPath(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	temp, err := filepath.Abs(os.TempDir())
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(temp, abs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	return len(parts) > 1 && strings.HasPrefix(parts[0], "browseruse-")
}

func truncateUTF8(s string, max int) string {
	return strutil.Ellipsis(s, max, "")
}

func truncateActionOutput(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 4096 {
		return s
	}
	return truncateUTF8(s, 4096) + "\n...[truncated]"
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: msg, IsError: true}
}

// ResolveActionDriverPath finds the bundled browser-use Playwright driver when
// running from a source checkout. Packaged installs should pass an explicit
// AGEZT_BROWSER_ACTION_DRIVER path after materializing the browser-use bundle.
func ResolveActionDriverPath() string {
	candidates := []string{
		filepath.Join("plugins", "builtinskills", "browseruse", "scripts", "browse.mjs"),
		filepath.Join("..", "..", "plugins", "builtinskills", "browseruse", "scripts", "browse.mjs"),
	}
	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			candidates = append(candidates, filepath.Join(dir, "plugins", "builtinskills", "browseruse", "scripts", "browse.mjs"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(base, "plugins", "builtinskills", "browseruse", "scripts", "browse.mjs"),
			filepath.Join(base, "..", "plugins", "builtinskills", "browseruse", "scripts", "browse.mjs"),
		)
	}
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if st, err := os.Stat(abs); err == nil && !st.IsDir() {
			return abs
		}
	}
	return ""
}
