// SPDX-License-Identifier: MIT

package runtime

// Provenance: Runtime workboard: proof-verification (ProveTask). Code extracted from
//             workboard.go during the Day-121 god-file split. Public API unchanged.
//
// The proof CYCLE lives in the work module now (W3.70): the board keeps the
// task and the proof that closes it, so the board keeps the judgment — the
// evidence to gather, the criterion prompt, the reply to parse and the verdict
// to publish. The daemon keeps its NAME and forwards.

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
)

// ProveTask proves a task against its acceptance criteria.
//
// The daemon holds the board's face and asks the module: the answer is the
// module's, the proof is the board's, and the daemon's name is unchanged.
func (k *Kernel) ProveTask(ctx context.Context, corr, id, answer string) (workboardapi.Task, error) {
	svc, err := k.boardOrRefusal()
	if err != nil {
		return workboardapi.Task{}, err
	}
	return svc.Prove(ctx, corr, id, answer)
}
