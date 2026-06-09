// SPDX-License-Identifier: MIT

package controlplane

// Read-only inspection of the sandbox projects the agent BUILT with the
// code_exec tool (M686). Persistent projects live under <baseDir>/sandbox/
// projects/<name>; this surfaces them — and their files' contents — so the
// operator can see (and, in the UI, download) what their agents actually built,
// instead of the work being invisible on disk. No mutation; every read is
// confined to the projects directory.

import (
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// maxSandboxFileBytes caps a single file read so a huge artifact can't blow
	// the response/context budget. 256 KiB matches the code_exec output cap.
	maxSandboxFileBytes = 256 * 1024
	// maxSandboxFilesPerProject / maxSandboxProjects bound the listing on a busy
	// daemon so the response stays small.
	maxSandboxFilesPerProject = 500
	maxSandboxProjects        = 500
)

// sandboxProjectsDir is the on-disk root of persistent code_exec projects.
func (s *Server) sandboxProjectsDir() string {
	return filepath.Join(s.k.BaseDir(), "sandbox", "projects")
}

// handleSandboxList enumerates every persistent project with its files. Missing
// directory (no projects yet) is an empty list, not an error.
func (s *Server) handleSandboxList(conn net.Conn, req Request) {
	root := s.sandboxProjectsDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		// No projects dir yet → nothing built. Not an error condition.
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"projects": []any{}, "count": 0}})
		return
	}

	projects := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if len(projects) >= maxSandboxProjects {
			break
		}
		projects = append(projects, projectView(filepath.Join(root, e.Name()), e.Name()))
	}
	// Most-recently-touched first.
	sort.SliceStable(projects, func(i, j int) bool {
		return asInt64(projects[i]["modified_unix"]) > asInt64(projects[j]["modified_unix"])
	})
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"projects": projects, "count": len(projects)}})
}

// projectView walks one project dir and summarises it: each file's relative path
// + size, the file count, total bytes, and the newest modtime.
func projectView(dir, name string) map[string]any {
	files := make([]map[string]any, 0, 16)
	var totalBytes, modified int64
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if len(files) >= maxSandboxFilesPerProject {
			return filepath.SkipAll
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return nil
		}
		files = append(files, map[string]any{"name": filepath.ToSlash(rel), "bytes": info.Size()})
		totalBytes += info.Size()
		if mt := info.ModTime().Unix(); mt > modified {
			modified = mt
		}
		return nil
	})
	sort.SliceStable(files, func(i, j int) bool {
		return files[i]["name"].(string) < files[j]["name"].(string)
	})
	return map[string]any{
		"name":          name,
		"files":         files,
		"file_count":    len(files),
		"total_bytes":   totalBytes,
		"modified_unix": modified,
	}
}

// handleSandboxFile returns one file's content, path-confined to a single
// project under the projects dir. Rejects traversal; caps the read.
func (s *Server) handleSandboxFile(conn net.Conn, req Request) {
	project, _ := req.Args["project"].(string)
	file, _ := req.Args["file"].(string)
	if strings.TrimSpace(project) == "" || strings.TrimSpace(file) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.project and args.file required"})
		return
	}

	root := s.sandboxProjectsDir()
	// Confine the project to a single segment under root.
	projDir, ok := confineUnder(root, project)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "illegal project name"})
		return
	}
	full, ok := confineUnder(projDir, file)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "illegal file path"})
		return
	}

	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "no such file"})
		return
	}
	data, err := os.ReadFile(full)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "read: " + err.Error()})
		return
	}
	truncated := false
	if len(data) > maxSandboxFileBytes {
		data = data[:maxSandboxFileBytes]
		truncated = true
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"project":   project,
		"file":      filepath.ToSlash(file),
		"bytes":     info.Size(),
		"truncated": truncated,
		"content":   string(data),
	}})
}

// confineUnder joins an untrusted relative path to root and confirms the result
// stays inside root (rejecting "..", absolute paths, and Windows drive-relative
// escapes). Returns the cleaned absolute path on success.
func confineUnder(root, rel string) (string, bool) {
	rel = strings.TrimSpace(rel)
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, "\x00") {
		return "", false
	}
	full := filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
	cleanRoot := filepath.Clean(root)
	if full != cleanRoot && !strings.HasPrefix(full, cleanRoot+string(filepath.Separator)) {
		return "", false
	}
	return full, true
}

func asInt64(v any) int64 {
	if n, ok := v.(int64); ok {
		return n
	}
	return 0
}
