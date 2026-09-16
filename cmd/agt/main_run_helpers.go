// SPDX-License-Identifier: MIT
//
// cmd/agt `run` mode helpers (imageMediaType, loadImageDataURL, parseUSDToMicrocents,
// toStringSlice, cmdSimple).
// Extracted from main_run_modes.go during Day 211 god-file refactor (#95).
// Public API unchanged.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/internal/brand"
)

func imageMediaType(path string) (string, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png", true
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".gif":
		return "image/gif", true
	case ".webp":
		return "image/webp", true
	}
	return "", false
}
func loadImageDataURL(path string) (string, error) {
	mt, ok := imageMediaType(path)
	if !ok {
		return "", fmt.Errorf("unsupported image type %q (use .png, .jpg, .gif, or .webp)", filepath.Ext(path))
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(b) == 0 {
		return "", fmt.Errorf("image is empty")
	}
	// The control plane caps a single request at 16 MiB (server.go
	// maxRequestBytes) and base64 inflates by ~4/3, so refuse early with a
	// clear message instead of letting the daemon reject an oversized frame.
	const maxRaw = 12 << 20
	if len(b) > maxRaw {
		return "", fmt.Errorf("image is %d bytes; the limit is %d (control-plane request cap)", len(b), maxRaw)
	}
	return "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(b), nil
}
func parseUSDToMicrocents(s string) (int64, error) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "$"))
	usd, err := strconv.ParseFloat(s, 64)
	if err != nil || usd <= 0 {
		return 0, fmt.Errorf("invalid amount %q", s)
	}
	return int64(usd * 1_000_000_000), nil
}
func toStringSlice(v any) []string {
	if xs, ok := v.([]string); ok {
		return append([]string(nil), xs...)
	}
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
func cmdSimple(cmd string, args map[string]any, stdout, stderr io.Writer) int {
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Call(ctx, cmd, args)
	if err != nil {
		fmt.Fprintf(stderr, "%s %s: %v\n", brand.CLI, cmd, err)
		return 1
	}
	enc, _ := json.MarshalIndent(res, "", "  ")
	fmt.Fprintf(stdout, "%s\n", enc)
	return 0
}
