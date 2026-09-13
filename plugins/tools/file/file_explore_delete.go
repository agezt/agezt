// SPDX-License-Identifier: MIT

package file

// File-tool DELETE subcommand: doDelete. Carved out of file_explore.go
// during the Day 189 god-file split so the explore file can stay
// focused on the four READ methods (doList + doSearch + doGlob +
// doStat) and the paths file can stay focused on path-resolution
// + result helpers.
// Public API unchanged.

import (
	"context"
	"errors"
	"os"

	"github.com/agezt/agezt/kernel/agent"
)

func (t *Tool) doDelete(ctx context.Context, in fileInput) (agent.Result, error) {
	p, err := t.resolve(in.Path)
	if err != nil {
		return errResult(err.Error()), nil
	}
	if p == t.root {
		return errResult("refusing to delete workspace root"), nil
	}
	info, err := os.Stat(p)
	if err != nil {
		return errResult("stat: " + err.Error()), nil
	}
	if info.IsDir() {
		// For M1 we refuse recursive delete; the model can list and delete
		// individual files. Recursive ops are a future Edict-gated action.
		return errResult(in.Path + " is a directory; recursive delete is not allowed in M1"), nil
	}
	if err := t.checkpointFileSnapshot(ctx, "file.delete", in.Path, p); err != nil {
		return errResult("checkpoint: " + err.Error()), nil
	}
	if err := os.Remove(p); err != nil {
		return errResult("remove: " + err.Error()), nil
	}
	return agent.Result{Output: "deleted " + in.Path}, nil
}

// ----- containment -----

// ErrEscape is returned when a requested path resolves outside the root.
var ErrEscape = errors.New("file: path escapes workspace root")

// resolve canonicalizes the requested relative path and asserts it lives
// inside t.root.
