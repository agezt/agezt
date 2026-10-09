// SPDX-License-Identifier: MIT

// Package sandbox lets the operator inspect, and remove, the persistent
// projects agents build with the code_exec tool (M686) under
// <home>/sandbox/projects/<name>, so the work is not invisible on disk. Every
// read is confined to the projects directory.
package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/fileworkspace"
	"github.com/agezt/agezt/kernel/platform/schema"
)

const (
	// MaxFileBytes caps one file read so a huge artifact cannot blow the
	// response or context budget; it matches the code_exec output cap.
	MaxFileBytes = 256 * 1024
	// MaxFilesPerProject and MaxProjects bound the listing on a busy daemon.
	MaxFilesPerProject = 500
	MaxProjects        = 500
)

// Service reads and removes projects under one projects root.
type Service struct{ root string }

func New(root string) *Service { return &Service{root: root} }

type FileRow struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

type Project struct {
	Name         string    `json:"name"`
	Files        []FileRow `json:"files"`
	FileCount    int       `json:"file_count"`
	TotalBytes   int64     `json:"total_bytes"`
	ModifiedUnix int64     `json:"modified_unix"`
}

type ListRequest struct{}

type ListOutput struct {
	Projects []Project `json:"projects"`
	Count    int       `json:"count"`
}

type FileRequest struct {
	Project json.RawMessage `json:"project,omitempty"`
	File    json.RawMessage `json:"file,omitempty"`
}

type FileOutput struct {
	Project   string `json:"project"`
	File      string `json:"file"`
	Bytes     int64  `json:"bytes"`
	Truncated bool   `json:"truncated"`
	Content   string `json:"content"`
}

type DeleteRequest struct {
	Project json.RawMessage `json:"project,omitempty"`
}

type DeleteOutput struct {
	Deleted string `json:"deleted"`
}

// required reads a strict string: absent or blank is required, a present
// non-string (null included) is an error. The value passes untrimmed.
func required(raw json.RawMessage, key string) (string, error) {
	var s string
	if len(raw) > 0 {
		var v any
		_ = json.Unmarshal(raw, &v)
		str, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("args.%s must be a string", key)
		}
		s = str
	}
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("args.%s required", key)
	}
	return s, nil
}

// List enumerates every project directory, most recently modified first. A
// missing projects directory means nothing was built yet, not an error.
func (s *Service) List(_ context.Context, _ ListRequest) (ListOutput, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return ListOutput{Projects: []Project{}}, nil
	}
	projects := make([]Project, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if len(projects) >= MaxProjects {
			break
		}
		projects = append(projects, projectView(filepath.Join(s.root, e.Name()), e.Name()))
	}
	sort.SliceStable(projects, func(i, j int) bool { return projects[i].ModifiedUnix > projects[j].ModifiedUnix })
	return ListOutput{Projects: projects, Count: len(projects)}, nil
}

// projectView summarises one project: each file's slash path and size in name
// order, the file count, total bytes and the newest modification time.
func projectView(dir, name string) Project {
	files := make([]FileRow, 0, 16)
	var totalBytes, modified int64
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if len(files) >= MaxFilesPerProject {
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
		files = append(files, FileRow{Name: filepath.ToSlash(rel), Bytes: info.Size()})
		totalBytes += info.Size()
		if mt := info.ModTime().Unix(); mt > modified {
			modified = mt
		}
		return nil
	})
	sort.SliceStable(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return Project{Name: name, Files: files, FileCount: len(files), TotalBytes: totalBytes, ModifiedUnix: modified}
}

// File returns one file's content, confined to a single project and capped at
// MaxFileBytes. Links are resolved before the read and the resolved path must
// still sit under the projects root, so a planted link cannot leak a file.
func (s *Service) File(_ context.Context, in FileRequest) (FileOutput, error) {
	project, err := required(in.Project, "project")
	if err != nil {
		return FileOutput{}, err
	}
	file, err := required(in.File, "file")
	if err != nil {
		return FileOutput{}, err
	}
	projDir, ok := fileworkspace.ConfineUnder(s.root, project)
	if !ok {
		return FileOutput{}, errors.New("illegal project name")
	}
	full, ok := fileworkspace.ConfineUnder(projDir, file)
	if !ok {
		return FileOutput{}, errors.New("illegal file path")
	}
	resolved, rerr := filepath.EvalSymlinks(full)
	if rerr != nil {
		return FileOutput{}, errors.New("resolve sandbox path: " + rerr.Error())
	}
	if !strings.HasPrefix(resolved, s.root+string(filepath.Separator)) {
		return FileOutput{}, errors.New("sandbox escape: resolved path " + resolved + " is outside " + s.root)
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		return FileOutput{}, errors.New("no such file")
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return FileOutput{}, errors.New("read: " + err.Error())
	}
	truncated := false
	if len(data) > MaxFileBytes {
		data = data[:MaxFileBytes]
		truncated = true
	}
	return FileOutput{Project: project, File: filepath.ToSlash(file), Bytes: info.Size(), Truncated: truncated, Content: string(data)}, nil
}

// Delete removes one project directory: the target must be a direct child of
// the projects root, never the root itself or a nested path.
func (s *Service) Delete(_ context.Context, in DeleteRequest) (DeleteOutput, error) {
	project, err := required(in.Project, "project")
	if err != nil {
		return DeleteOutput{}, err
	}
	dir, ok := fileworkspace.ConfineUnder(s.root, project)
	if !ok || filepath.Dir(dir) != filepath.Clean(s.root) {
		return DeleteOutput{}, errors.New("illegal project name")
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return DeleteOutput{}, errors.New("no such project")
	}
	if err := os.RemoveAll(dir); err != nil {
		return DeleteOutput{}, errors.New("delete: " + err.Error())
	}
	return DeleteOutput{Deleted: project}, nil
}

// Operations declares the two unaudited reads and the audited removal, all
// operator-only on the Web UI routes.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("sandbox provider required")
	}
	listOut, err := schema.FromType(reflect.TypeFor[ListOutput](), false)
	if err != nil {
		return nil, err
	}
	fileOut, err := schema.FromType(reflect.TypeFor[FileOutput](), false)
	if err != nil {
		return nil, err
	}
	deleteOut, err := schema.FromType(reflect.TypeFor[DeleteOutput](), false)
	if err != nil {
		return nil, err
	}
	list, err := app.NewOperation(opapi.Spec{Name: "sandbox_list", ReadOnly: true, OutputSchema: listOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, HTTP: opapi.HTTP{Method: "GET", Path: "/api/sandbox"}}, func(ctx context.Context, in ListRequest) (ListOutput, error) {
		return provider(ctx).List(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	file, err := app.NewOperation(opapi.Spec{Name: "sandbox_file", ReadOnly: true, OutputSchema: fileOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"project":{},"file":{}}}`), HTTP: opapi.HTTP{Method: "GET", Path: "/api/sandbox_file"}}, func(ctx context.Context, in FileRequest) (FileOutput, error) {
		return provider(ctx).File(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	del, err := app.NewOperation(opapi.Spec{Name: "sandbox_delete", OutputSchema: deleteOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"project":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/sandbox/delete"}}, func(ctx context.Context, in DeleteRequest) (DeleteOutput, error) {
		return provider(ctx).Delete(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{list, file, del}, nil
}
