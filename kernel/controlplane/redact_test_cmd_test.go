// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/redact"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestRedactTest(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// No redactor installed → disabled, nothing scrubbed.
	res, err := c.Call(context.Background(), controlplane.CmdRedactTest,
		map[string]any{"text": "sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123"})
	if err != nil {
		t.Fatalf("redact_test: %v", err)
	}
	if en, _ := res["enabled"].(bool); en {
		t.Errorf("enabled=true with no redactor installed")
	}

	// Install a live redactor with a configured literal secret.
	red := redact.New()
	red.SetSecrets([]string{"hunter2-the-vault-secret"})
	k.Bus().SetRedactor(red)

	// A built-in pattern hit.
	res, _ = c.Call(context.Background(), controlplane.CmdRedactTest,
		map[string]any{"text": "my key is sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123 ok"})
	if w, _ := res["would_redact"].(bool); !w {
		t.Errorf("pattern secret not flagged would_redact")
	}
	if rd, _ := res["redacted"].(string); rd == "" || rd == "my key is sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123 ok" {
		t.Errorf("redacted form not scrubbed: %q", rd)
	}
	cats, _ := res["categories"].([]any)
	if len(cats) == 0 {
		t.Errorf("expected a matched category for an sk- key")
	}

	// A configured literal hit (no built-in pattern) → literal_hit true.
	res, _ = c.Call(context.Background(), controlplane.CmdRedactTest,
		map[string]any{"text": "password=hunter2-the-vault-secret"})
	if lit, _ := res["literal_hit"].(bool); !lit {
		t.Errorf("configured literal not flagged literal_hit")
	}

	// Ordinary prose → not redacted.
	res, _ = c.Call(context.Background(), controlplane.CmdRedactTest,
		map[string]any{"text": "just a normal sentence"})
	if w, _ := res["would_redact"].(bool); w {
		t.Errorf("ordinary prose flagged would_redact")
	}
}
