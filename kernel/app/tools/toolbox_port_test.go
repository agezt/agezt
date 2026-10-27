// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolboxapi"
	"github.com/agezt/agezt/kernel/event"
)

// TestToolboxWithoutAPortAnswersUnavailableAndPublishesNothing: a daemon
// without a toolbox host must say so — and must not journal an install it
// never ran.
//
// "The toolbox is not available on this daemon" and "this machine has no
// tools" are different facts, and the second one must never be reported as
// the first.
func TestToolboxWithoutAPortAnswersUnavailableAndPublishesNothing(t *testing.T) {
	reads := NewToolboxReads(nil)
	inventory, err := reads.Detect(context.Background(), ToolboxDetectInput{})
	if !errors.Is(err, toolboxapi.ErrUnavailable) || inventory.OS != "" || inventory.Tools != nil {
		t.Fatalf("EXPECTED:the read says the toolbox is unavailable ACTUAL:%+v %v", inventory, err)
	}
	outdated, err := reads.Outdated(context.Background(), ToolboxOutdatedInput{})
	if !errors.Is(err, toolboxapi.ErrUnavailable) || outdated.Count != 0 || outdated.Outdated != nil {
		t.Fatalf("EXPECTED:the outdated read says the toolbox is unavailable ACTUAL:%+v %v", outdated, err)
	}

	published := 0
	install := NewToolboxInstall(nil, func(event.Kind, map[string]any) error { published++; return nil })
	summary, err := install.Install(context.Background(), ToolboxInstallInput{Names: []string{"owned"}}, nil)
	if !errors.Is(err, toolboxapi.ErrUnavailable) || published != 0 {
		t.Fatalf("EXPECTED:the install refuses before it publishes ACTUAL:%+v %v published=%d", summary, err, published)
	}
	if summary.Installed != nil || summary.Failed != nil || summary.Skipped != nil {
		t.Fatalf("EXPECTED:no summary rows ACTUAL:%+v", summary)
	}
}

// TestToolboxInstallStillValidatesNamesBeforeThePort: a malformed request is
// malformed on every daemon — the port's absence does not change the
// validation answer.
func TestToolboxInstallStillValidatesNamesBeforeThePort(t *testing.T) {
	install := NewToolboxInstall(nil, func(event.Kind, map[string]any) error { return nil })
	_, err := install.Install(context.Background(), ToolboxInstallInput{}, nil)
	if err == nil || err.Error() != "args.names (non-empty list) required" {
		t.Fatalf("EXPECTED:the names are still required ACTUAL:%v", err)
	}
}
