// SPDX-License-Identifier: MIT

// CodeExec artifacts: stripBase64Whitespace + extractArtifactArchive + nowMillis + artifactMime + artifactKind + appendArtifactExport + splitArtifactEnvelope.
// Code extracted from artifacts.go during the Day-142 god-file split.
// Public API unchanged.
package codeexec


import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"strings"
	"time"

	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
	stdhttp "net/http"
	"path/filepath"
)

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
