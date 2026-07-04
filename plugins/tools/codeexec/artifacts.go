// SPDX-License-Identifier: MIT

package codeexec

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	stdhttp "net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/executionprofile"
	"github.com/agezt/agezt/kernel/warden"
)

const (
	artifactExportDir           = ".agezt-artifacts"
	maxExportedArtifactFiles    = 32
	maxExportedArtifactBytes    = 50 << 20
	maxExportedArtifactTotal    = 100 << 20
	artifactExportCheckTimeout  = 10 * time.Second
	artifactExportCopyTimeout   = 60 * time.Second
	artifactExportLocalDirPerms = 0o700
)

const (
	modalArtifactBegin = "__AGEZT_MODAL_ARTIFACTS_BEGIN__"
	modalArtifactEnd   = "__AGEZT_MODAL_ARTIFACTS_END__"
)

type artifactExportRecord struct {
	ID   string `json:"id"`
	Ref  string `json:"ref"`
	Name string `json:"name"`
	Mime string `json:"mime"`
	Kind string `json:"kind"`
	Size int64  `json:"size"`
}

var errArtifactExportStop = errors.New("artifact export stopped")

func (t *Tool) exportSSHArtifacts(ctx context.Context, w warden.Engine, cfg executionprofile.SSHConfig, remoteDir string) ([]artifactExportRecord, error) {
	if t.index == nil {
		return nil, nil
	}
	remoteExportDir := path.Join(remoteDir, artifactExportDir)
	check, err := runSSHCommand(ctx, w, cfg, "test -d "+executionprofile.ShellQuote(remoteExportDir), artifactExportCheckTimeout, "tool.code_exec.ssh.artifacts.check")
	if err != nil {
		return nil, fmt.Errorf("check remote artifact directory: %w", err)
	}
	if check.ExitCode != 0 {
		return nil, nil
	}
	localDir, cleanup, err := t.tempArtifactExportDir()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	res, err := w.Run(ctx, warden.Spec{
		Profile: warden.ProfileNone,
		Argv:    cfg.SCPFromArgv(path.Join(remoteExportDir, "."), localDir),
		Env:     sshClientEnv(),
		Limits: warden.Limits{
			Timeout:        artifactExportCopyTimeout,
			MaxOutputBytes: MaxOutputBytes,
		},
		Actor:         "tool.code_exec.ssh.artifacts.download",
		CorrelationID: warden.CorrelationFrom(ctx),
	})
	if err != nil {
		return nil, fmt.Errorf("download remote artifacts: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("download remote artifacts failed (exit %d): %s", res.ExitCode, installTail(res))
	}
	return t.exportArtifactsFromDir(ctx, localDir, "ssh")
}

func (t *Tool) exportK8sArtifacts(ctx context.Context, w warden.Engine, cfg executionprofile.K8sConfig, remoteDir string) ([]artifactExportRecord, error) {
	if t.index == nil {
		return nil, nil
	}
	remoteExportDir := path.Join(remoteDir, artifactExportDir)
	check, err := runK8sCommand(ctx, w, cfg, "test -d "+executionprofile.ShellQuote(remoteExportDir), artifactExportCheckTimeout, "tool.code_exec.k8s.artifacts.check")
	if err != nil {
		return nil, fmt.Errorf("check remote artifact directory: %w", err)
	}
	if check.ExitCode != 0 {
		return nil, nil
	}
	localDir, cleanup, err := t.tempArtifactExportDir()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	res, err := w.Run(ctx, warden.Spec{
		Profile: warden.ProfileNone,
		Argv:    cfg.CopyFromArgv(path.Join(remoteExportDir, "."), localDir),
		Env:     kubectlClientEnv(),
		Limits: warden.Limits{
			Timeout:        artifactExportCopyTimeout,
			MaxOutputBytes: MaxOutputBytes,
		},
		Actor:         "tool.code_exec.k8s.artifacts.download",
		CorrelationID: warden.CorrelationFrom(ctx),
	})
	if err != nil {
		return nil, fmt.Errorf("download remote artifacts: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("download remote artifacts failed (exit %d): %s", res.ExitCode, installTail(res))
	}
	return t.exportArtifactsFromDir(ctx, localDir, "k8s")
}

func (t *Tool) tempArtifactExportDir() (string, func(), error) {
	root := strings.TrimSpace(t.SandboxRoot)
	if root == "" {
		root = os.TempDir()
	}
	if err := os.MkdirAll(root, artifactExportLocalDirPerms); err != nil {
		return "", func() {}, fmt.Errorf("prepare artifact export temp root: %w", err)
	}
	dir, err := os.MkdirTemp(root, "artifact-export-")
	if err != nil {
		return "", func() {}, fmt.Errorf("prepare artifact export temp dir: %w", err)
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

func (t *Tool) exportArtifactsFromDir(ctx context.Context, dir, profile string) ([]artifactExportRecord, error) {
	if t.index == nil {
		return nil, nil
	}
	root := filepath.Join(dir, artifactExportDir)
	if profile == "ssh" || profile == "k8s" || profile == "modal" || profile == "daytona" {
		root = dir
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat artifact export directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", artifactExportDir)
	}

	var paths []string
	if err := filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == root {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		paths = append(paths, p)
		if len(paths) > maxExportedArtifactFiles {
			return errArtifactExportStop
		}
		return nil
	}); err != nil && !errors.Is(err, errArtifactExportStop) {
		return nil, fmt.Errorf("scan artifact export directory: %w", err)
	}
	sort.Strings(paths)

	var records []artifactExportRecord
	var problems []string
	var total int64
	if len(paths) > maxExportedArtifactFiles {
		problems = append(problems, fmt.Sprintf("only the first %d artifact files were exported", maxExportedArtifactFiles))
		paths = paths[:maxExportedArtifactFiles]
	}
	for _, p := range paths {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			problems = append(problems, fmt.Sprintf("skip %s: %v", filepath.Base(p), err))
			continue
		}
		rel = filepath.ToSlash(rel)
		if clean, ok := sanitizeRelFile(rel); ok {
			rel = clean
		} else {
			problems = append(problems, fmt.Sprintf("skip illegal artifact path %q", rel))
			continue
		}
		info, err := os.Stat(p)
		if err != nil {
			problems = append(problems, fmt.Sprintf("skip %s: %v", rel, err))
			continue
		}
		if info.Size() > maxExportedArtifactBytes {
			problems = append(problems, fmt.Sprintf("skip %s: exceeds %d MiB", rel, maxExportedArtifactBytes>>20))
			continue
		}
		if total+info.Size() > maxExportedArtifactTotal {
			problems = append(problems, fmt.Sprintf("skip remaining artifacts: total export limit is %d MiB", maxExportedArtifactTotal>>20))
			break
		}
		data, err := os.ReadFile(p)
		if err != nil {
			problems = append(problems, fmt.Sprintf("skip %s: %v", rel, err))
			continue
		}
		total += int64(len(data))
		mimeType := artifactMime(rel, data)
		entry, err := t.index.PutEntry(artifact.Entry{
			Kind:    artifactKind(mimeType),
			Source:  "code_exec",
			Name:    rel,
			Mime:    mimeType,
			Corr:    warden.CorrelationFrom(ctx),
			Caption: "code_exec " + profile + " artifact export",
		}, data, t.nowMillis())
		if err != nil {
			problems = append(problems, fmt.Sprintf("save %s: %v", rel, err))
			continue
		}
		records = append(records, artifactExportRecord{
			ID:   entry.ID,
			Ref:  entry.Ref,
			Name: entry.Name,
			Mime: entry.Mime,
			Kind: entry.Kind,
			Size: entry.Size,
		})
	}
	if len(problems) > 0 {
		return records, errors.New(strings.Join(problems, "; "))
	}
	return records, nil
}

func (t *Tool) exportTarGzBase64Artifacts(ctx context.Context, encoded, profile string) ([]artifactExportRecord, error) {
	if t.index == nil {
		return nil, nil
	}
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(stripBase64Whitespace(encoded))
	if err != nil {
		return nil, fmt.Errorf("decode remote artifact archive: %w", err)
	}
	localDir, cleanup, err := t.tempArtifactExportDir()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	if err := extractArtifactArchive(data, localDir); err != nil {
		return nil, err
	}
	return t.exportArtifactsFromDir(ctx, localDir, profile)
}

func stripBase64Whitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case ' ', '\n', '\r', '\t':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func extractArtifactArchive(data []byte, dest string) error {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("open remote artifact archive: %w", err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	var files int
	var total int64
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read remote artifact archive: %w", err)
		}
		name, ok := sanitizeRelFile(hdr.Name)
		if !ok {
			return fmt.Errorf("remote artifact archive contains illegal path %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(filepath.Join(dest, filepath.FromSlash(name)), 0o700); err != nil {
				return err
			}
			continue
		case tar.TypeReg:
		default:
			continue
		}
		files++
		if files > maxExportedArtifactFiles {
			return fmt.Errorf("remote artifact archive exceeds %d files", maxExportedArtifactFiles)
		}
		if hdr.Size > maxExportedArtifactBytes {
			return fmt.Errorf("remote artifact %s exceeds %d MiB", name, maxExportedArtifactBytes>>20)
		}
		total += hdr.Size
		if total > maxExportedArtifactTotal {
			return fmt.Errorf("remote artifact archive exceeds %d MiB", maxExportedArtifactTotal>>20)
		}
		out := filepath.Join(dest, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.CopyN(f, tr, hdr.Size)
		closeErr := f.Close()
		if copyErr != nil && !errors.Is(copyErr, io.EOF) {
			return fmt.Errorf("extract remote artifact %s: %w", name, copyErr)
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func (t *Tool) nowMillis() int64 {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now().UnixMilli()
}

func artifactMime(name string, data []byte) string {
	if mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); mt != "" {
		return mt
	}
	sample := data
	if len(sample) > 512 {
		sample = sample[:512]
	}
	return stdhttp.DetectContentType(sample)
}

func artifactKind(mimeType string) string {
	if strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return "image"
	}
	return "file"
}

func appendArtifactExport(res agent.Result, profile string, artifacts []artifactExportRecord, exportErr error) agent.Result {
	if len(artifacts) == 0 && exportErr == nil {
		return res
	}
	out := strings.TrimRight(res.Output, "\n")
	if len(artifacts) > 0 {
		payload, err := json.MarshalIndent(map[string]any{
			"profile":   profile,
			"directory": artifactExportDir,
			"artifacts": artifacts,
		}, "", "  ")
		if err == nil {
			out += "\n[artifact_export]\n" + string(payload)
		}
	}
	if exportErr != nil {
		out += "\n[artifact_export_error] " + exportErr.Error()
	}
	res.Output = out
	return res
}

func splitArtifactEnvelope(stdout []byte, begin, end string) (clean []byte, payload string, found bool, err error) {
	text := string(stdout)
	start := strings.Index(text, begin)
	if start < 0 {
		return stdout, "", false, nil
	}
	payloadStart := start + len(begin)
	stopRel := strings.Index(text[payloadStart:], end)
	if stopRel < 0 {
		return stdout, "", true, fmt.Errorf("artifact envelope missing end marker")
	}
	stop := payloadStart + stopRel
	before := strings.TrimRight(text[:start], "\n")
	after := strings.TrimLeft(text[stop+len(end):], "\n")
	switch {
	case before != "" && after != "":
		clean = []byte(before + "\n" + after)
	case before != "":
		clean = []byte(before)
	default:
		clean = []byte(after)
	}
	return clean, strings.TrimSpace(text[payloadStart:stop]), true, nil
}
