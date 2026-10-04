// SPDX-License-Identifier: MIT

// Package files applies console file mutations through the host's governed tool
// invocation port. Filesystem effects and path checks live in fileworkspace.
package files

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/fileworkspace"
)

const (
	Mkdir  = "file_mkdir"
	Rename = "file_rename"
	Delete = "file_delete"

	InvalidPath = "file_invalid_path"
	NotFound    = "file_not_found"
	Symlink     = "file_symlink"
	IOFailure   = "file_io"
	Denied      = "file_denied"
	Unavailable = "file_unavailable"
)

// Runner keeps the service independent of runtime and transport implementations.
type Runner interface {
	RunToolWithLookup(context.Context, string, string, string, json.RawMessage, toolapi.ToolLookup) (toolapi.Result, error)
}

// Error carries a transport-independent classification without losing its cause.
type Error struct {
	Code  string
	Cause error
}

func (e *Error) Error() string { return e.Cause.Error() }
func (e *Error) Unwrap() error { return e.Cause }

// Apply admits an invocation-local adapter through the kernel's existing service.
// Root creation and path resolution occur only after policy/audit admission.
func Apply(ctx context.Context, runner Runner, corr, callID, operation string, args map[string]any) (map[string]any, error) {
	switch operation {
	case Mkdir, Rename, Delete:
	default:
		return nil, &Error{Code: InvalidPath, Cause: fmt.Errorf("unknown file operation %q", operation)}
	}
	input := map[string]any{}
	if operation == Rename {
		input["from"], _ = args["from"].(string)
		input["to"], _ = args["to"].(string)
	} else {
		input["path"], _ = args["path"].(string)
		if operation == Mkdir {
			input["parents"], _ = args["parents"].(bool)
		} else {
			input["recursive"], _ = args["recursive"].(bool)
		}
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	adapter := &mutation{operation: operation}
	result, err := runner.RunToolWithLookup(ctx, corr, callID, operation, raw, adapter)
	if err != nil {
		var classified *Error
		if errors.As(err, &classified) {
			return nil, err
		}
		code := Unavailable
		if strings.HasPrefix(result.Output, "tool call denied by policy:") {
			code = Denied
		}
		return nil, &Error{Code: code, Cause: err}
	}
	if result.IsError {
		return nil, &Error{Code: IOFailure, Cause: errors.New(result.Output)}
	}
	var output map[string]any
	if err := json.Unmarshal([]byte(result.Output), &output); err != nil {
		return nil, &Error{Code: Unavailable, Cause: err}
	}
	return output, nil
}

type mutation struct{ operation string }

func (m *mutation) Definition() toolapi.ToolDef {
	capability := "file.write"
	if m.operation == Delete {
		capability = "file.delete"
	}
	return toolapi.ToolDef{
		Name: m.operation, Capability: toolapi.ToolCapability{Name: capability},
		Description: "Apply an operator file mutation within the configured console workspace.",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Effect: toolapi.ToolEffect{
			Class: toolapi.EffectIrreversible, PredictedEffects: []string{m.operation},
			AffectedResources: []string{"configured File Manager workspace"},
			RollbackNotes:     "Filesystem mutations may overwrite or remove existing entries.", Confidence: 1,
		},
	}
}

func (m *mutation) LookupTool(name string) (toolapi.Tool, bool) {
	if name != m.operation {
		return nil, false
	}
	return m, true
}

func (m *mutation) Invoke(ctx context.Context, raw json.RawMessage) (toolapi.Result, error) {
	if err := ctx.Err(); err != nil {
		return toolapi.Result{}, err
	}
	var in struct {
		Path, From, To     string
		Parents, Recursive bool
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return toolapi.Result{}, &Error{Code: InvalidPath, Cause: err}
	}
	var output map[string]any
	var err error
	if m.operation == Rename {
		if in.From == "" || in.To == "" {
			return toolapi.Result{}, &Error{Code: InvalidPath, Cause: errors.New("from and to are required")}
		}
		_, from, _, resolveErr := fileworkspace.Resolve(in.From)
		if resolveErr != nil {
			return toolapi.Result{}, &Error{Code: InvalidPath, Cause: resolveErr}
		}
		_, to, _, resolveErr := fileworkspace.Resolve(in.To)
		if resolveErr != nil {
			return toolapi.Result{}, &Error{Code: InvalidPath, Cause: resolveErr}
		}
		err = fileworkspace.Rename(from, to)
		output = map[string]any{"ok": true, "from": filepath.ToSlash(in.From), "to": filepath.ToSlash(in.To)}
	} else {
		_, target, _, resolveErr := fileworkspace.Resolve(in.Path)
		if resolveErr != nil {
			return toolapi.Result{}, &Error{Code: InvalidPath, Cause: resolveErr}
		}
		if m.operation == Mkdir {
			err = fileworkspace.Mkdir(target, in.Parents)
		} else {
			err = fileworkspace.Delete(target, in.Recursive)
		}
		output = map[string]any{"ok": true, "path": filepath.ToSlash(in.Path)}
	}
	if err != nil {
		code := IOFailure
		if m.operation == Delete && os.IsNotExist(err) {
			code = NotFound
		} else if errors.Is(err, fileworkspace.ErrSymlink) {
			code = Symlink
		}
		return toolapi.Result{}, &Error{Code: code, Cause: err}
	}
	encoded, err := json.Marshal(output)
	return toolapi.Result{Output: string(encoded)}, err
}
