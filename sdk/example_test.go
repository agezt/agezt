// SPDX-License-Identifier: MIT

package sdk_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/agezt/agezt/sdk"
)

// Connect to the local daemon and run an intent, printing the final answer.
func ExampleClient_Run() {
	c, err := sdk.Dial("") // "" → $AGEZT_HOME or ~/.agezt
	if err != nil {
		log.Fatal(err)
	}
	res, err := c.Run(context.Background(), "summarise this repository",
		sdk.WithModel("claude-opus-4-8"),
		sdk.WithTimeout(2*time.Minute),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(res.Answer)
}

// Stream a run, printing the answer as the model generates it.
func ExampleClient_RunStream() {
	c, err := sdk.Dial("")
	if err != nil {
		log.Fatal(err)
	}
	_, err = c.RunStream(context.Background(), "what changed recently?",
		func(ev *sdk.Event) {
			if txt, ok := sdk.TokenText(ev); ok {
				fmt.Print(txt)
			}
		})
	if err != nil {
		log.Fatal(err)
	}
}

// List the most recent runs from the journal.
func ExampleClient_Runs() {
	c, err := sdk.Dial("")
	if err != nil {
		log.Fatal(err)
	}
	runs, err := c.Runs(context.Background(), 5)
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range runs {
		fmt.Printf("%s\t%s\t%s\n", r.CorrelationID, r.Status, r.Intent)
	}
}

// Resolve pending human-in-the-loop approval requests.
func ExampleClient_PendingApprovals() {
	c, err := sdk.Dial("")
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	pending, err := c.PendingApprovals(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, a := range pending {
		// Approve read-only fetches; deny everything else.
		if a.Capability == "http.fetch" {
			_ = c.Approve(ctx, a.ID, "read-only fetch is fine")
		} else {
			_ = c.Deny(ctx, a.ID, "not allowed by this app")
		}
	}
}
