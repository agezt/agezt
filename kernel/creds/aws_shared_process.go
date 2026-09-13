// SPDX-License-Identifier: MIT
//
// aws shared credential_process sub-command parser: runCredentialProcess +
// splitCommandLine.
// Extracted from aws_shared.go during the Day-204 god-file split.
// Public API unchanged.
package creds

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"strings"
	"time"
)

// runCredentialProcess executes the configured `credential_process`
// command and decodes its stdout per the AWS spec:
// https://docs.aws.amazon.com/sdkref/latest/guide/feature-process-credentials.html
//
//	{
//	  "Version": 1,
//	  "AccessKeyId": "AKIA...",
//	  "SecretAccessKey": "...",
//	  "SessionToken": "...",        // optional
//	  "Expiration": "2025-01-01T00:00:00Z"  // optional, RFC3339
//	}
//
// Returns the parsed key/secret/token map and the credentials'
// Expiration. The map is nil if the call failed for any reason (the
// chain falls through silently — same pattern as IMDS failure); a
// zero Expiration means the helper advertised none, i.e. the
// credentials are permanent and never need re-minting.
//
// Spec quoting rules: AWS allows the command to be a shell-style
// string with quoting. We use osexec.Command with shell parsing
// via shell-words-like splitting — simpler than depending on a
// real shell, and matches what AWS-SDK-go does internally for
// this case. Operators who need a real shell can wrap their tool
// in a script.
func runCredentialProcess(commandLine string) (map[string]string, time.Time) {
	if strings.TrimSpace(os.Getenv(EnvCredentialProcessAllowed)) != "1" {
		return nil, time.Time{}
	}
	parts, err := splitCommandLine(commandLine)
	if err != nil || len(parts) == 0 {
		// Mis-split (e.g. an unterminated quote) must NOT run a half-parsed
		// argv — fall through silently like any other cred-source failure.
		return nil, time.Time{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), credentialProcessTimeout)
	defer cancel()
	cmd := osexec.CommandContext(ctx, parts[0], parts[1:]...)
	// Never inherit the daemon's environment (SEC-003). See credentialProcessEnv.
	cmd.Env = credentialProcessEnv()
	output, err := cmd.Output()
	if err != nil {
		return nil, time.Time{}
	}
	var doc struct {
		Version         int    `json:"Version"`
		AccessKeyID     string `json:"AccessKeyId"`
		SecretAccessKey string `json:"SecretAccessKey"`
		SessionToken    string `json:"SessionToken"`
		Expiration      string `json:"Expiration"`
	}
	if err := json.Unmarshal(output, &doc); err != nil {
		return nil, time.Time{}
	}
	if doc.Version != 1 || doc.AccessKeyID == "" || doc.SecretAccessKey == "" {
		return nil, time.Time{}
	}
	out := map[string]string{
		"AWS_ACCESS_KEY_ID":     doc.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": doc.SecretAccessKey,
	}
	if doc.SessionToken != "" {
		out["AWS_SESSION_TOKEN"] = doc.SessionToken
	}
	var expires time.Time
	if doc.Expiration != "" {
		if parsed, err := time.Parse(time.RFC3339, doc.Expiration); err == nil {
			expires = parsed
		}
	}
	return out, expires
}

// splitCommandLine is a minimal shell-style tokeniser supporting
// `"double-quoted"` and `'single-quoted'` spans. Backslashes are kept LITERAL
// (not treated as escapes) so Windows paths like C:\Tools\creds.exe tokenise
// correctly; operators needing a literal quote inside an argument should wrap
// the command in a script. An unterminated quote is reported as an error so the
// caller refuses to exec a mis-split argv rather than silently swallowing the
// rest of the line into one token.
func splitCommandLine(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	started := false // distinguishes an empty quoted arg ("") from no arg
	inQuote := rune(0)
	for _, r := range s {
		switch {
		case inQuote != 0 && r == inQuote:
			inQuote = 0
		case inQuote != 0:
			cur.WriteRune(r)
		case r == '"' || r == '\'':
			inQuote = r
			started = true
		case r == ' ' || r == '\t':
			if started {
				out = append(out, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if inQuote != 0 {
		return nil, fmt.Errorf("creds: unterminated %c-quote in credential_process command", inQuote)
	}
	if started {
		out = append(out, cur.String())
	}
	return out, nil
}
