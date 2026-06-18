// SPDX-License-Identifier: MIT

package main

import "testing"

func TestWorkflowContractText_StatesReusableChainSurface(t *testing.T) {
	if got := workflowContractText(true, "cron (every 1h)"); got != "enabled reusable chain · trigger cron (every 1h) · runnable by user, agent, schedule, or webhook" {
		t.Fatalf("enabled contract = %q", got)
	}
	if got := workflowContractText(false, ""); got != "disabled reusable chain · trigger manual/API · runnable by user, agent, schedule, or webhook" {
		t.Fatalf("disabled contract = %q", got)
	}
}
