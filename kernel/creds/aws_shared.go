// SPDX-License-Identifier: MIT

// AWS shared-credentials file + credential_process: AWSSharedCredentialsLookup + runCredentialProcess + splitCommandLine + loadAWSSharedFiles + awsConfigFilePath + readINISection.
// Code extracted from aws.go during the Day-53 god-file split. Public API unchanged.
package creds


import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)


func AWSSharedCredentialsLookup(profile string) func(string) string {
	var (
		once   sync.Once
		values map[string]string // static ~/.aws values — long-lived, loaded once
		// credential_process state. procCmd is written inside once.Do and
		// only read after it (sync.Once gives the happens-before); the rest
		// is guarded by mu, mirroring imdsCache below.
		mu       sync.Mutex
		procCmd  string
		procVals map[string]string
		procExp  time.Time
		procNeg  time.Time
	)
	return func(name string) string {
		if _, ok := awsRecognisedNames[name]; !ok {
			return ""
		}
		once.Do(func() {
			values, procCmd = loadAWSSharedFiles(profile)
		})
		// Static file values win over helper output per-key (AWS-SDK
		// precedence, unchanged).
		if v := values[name]; v != "" {
			return v
		}
		if procCmd == "" {
			return ""
		}
		mu.Lock()
		defer mu.Unlock()
		now := time.Now()
		cacheValid := procVals != nil && (procExp.IsZero() || now.Add(refreshLead).Before(procExp))
		if !cacheValid {
			if !procNeg.IsZero() && now.Sub(procNeg) < negCacheTTL {
				// Recent helper failure — don't exec it on every lookup.
				return ""
			}
			vals, exp := runCredentialProcess(procCmd)
			if vals == nil {
				procNeg = now
				return ""
			}
			procVals, procExp, procNeg = vals, exp, time.Time{}
		}
		return procVals[name]
	}
}

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

func loadAWSSharedFiles(profile string) (map[string]string, string) {
	if profile == "" {
		profile = strings.TrimSpace(os.Getenv("AWS_PROFILE"))
	}
	if profile == "" {
		profile = "default"
	}
	out := make(map[string]string)

	credsPath := awsConfigFilePath("AWS_SHARED_CREDENTIALS_FILE", "credentials")
	if credsPath != "" {
		if section, err := readINISection(credsPath, profile); err == nil {
			if v := section["aws_access_key_id"]; v != "" {
				out["AWS_ACCESS_KEY_ID"] = v
			}
			if v := section["aws_secret_access_key"]; v != "" {
				out["AWS_SECRET_ACCESS_KEY"] = v
			}
			if v := section["aws_session_token"]; v != "" {
				out["AWS_SESSION_TOKEN"] = v
			}
			if v := section["region"]; v != "" {
				out["AWS_REGION"] = v
				out["AWS_DEFAULT_REGION"] = v
			}
		}
	}
	// ~/.aws/config: profile sections are `[profile name]` (literally
	// prefixed with "profile ") EXCEPT for default which is bare
	// "[default]". This is an AWS-CLI quirk we have to replicate.
	cfgPath := awsConfigFilePath("AWS_CONFIG_FILE", "config")
	if cfgPath != "" {
		cfgSection := profile
		if profile != "default" {
			cfgSection = "profile " + profile
		}
		if section, err := readINISection(cfgPath, cfgSection); err == nil {
			if v := section["region"]; v != "" && out["AWS_REGION"] == "" {
				out["AWS_REGION"] = v
				out["AWS_DEFAULT_REGION"] = v
			}
			// Some operators put credentials in config rather than
			// credentials — surface those too rather than silently
			// dropping them.
			if v := section["aws_access_key_id"]; v != "" && out["AWS_ACCESS_KEY_ID"] == "" {
				out["AWS_ACCESS_KEY_ID"] = v
			}
			if v := section["aws_secret_access_key"]; v != "" && out["AWS_SECRET_ACCESS_KEY"] == "" {
				out["AWS_SECRET_ACCESS_KEY"] = v
			}
			if v := section["aws_session_token"]; v != "" && out["AWS_SESSION_TOKEN"] == "" {
				out["AWS_SESSION_TOKEN"] = v
			}
		}
	}
	// credential_process (M1.pp). Spec applies to BOTH ~/.aws/credentials
	// and ~/.aws/config; only consulted when inline credentials are
	// absent AND the operator opted in via the env gate. Either
	// section's credential_process line can fire; credentials file
	// wins if both have one (matches AWS-SDK precedence). The helper is NOT
	// executed here: AWSSharedCredentialsLookup owns the credential_process
	// cache so temporary (Expiring) output can be re-minted — running it in
	// this once-per-process load would freeze the first mint forever.
	procCmd := ""
	if out["AWS_ACCESS_KEY_ID"] == "" {
		if credsPath != "" {
			if section, err := readINISection(credsPath, profile); err == nil {
				procCmd = section["credential_process"]
			}
		}
		if procCmd == "" && cfgPath != "" {
			cfgSection := profile
			if profile != "default" {
				cfgSection = "profile " + profile
			}
			if section, err := readINISection(cfgPath, cfgSection); err == nil {
				procCmd = section["credential_process"]
			}
		}
	}
	return out, procCmd
}

// awsConfigFilePath resolves the path to an AWS config file. The
// envOverride env var (e.g. AWS_SHARED_CREDENTIALS_FILE) takes
// precedence; otherwise we build ~/.aws/<basename>. Returns ""
// if neither override nor home dir is available.
func awsConfigFilePath(envOverride, basename string) string {
	if v := strings.TrimSpace(os.Getenv(envOverride)); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".aws", basename)
}

// readINISection parses path as INI and returns the key/value map
// for the named section. Sections are `[name]`; lines are
// `key = value` or `key=value`. `#` and `;` start comments;
// blank lines are skipped. Keys are case-insensitive (lowercased
// on return) so callers can lookup without worrying about how
// the operator typed them in.
//
// This is a deliberately tiny parser — the AWS INI dialect is a
// subset of full INI (no nested sections, no quoted strings, no
// multiline values). If we ever hit a config that needs more, we'll
// pull in a real INI lib then; today we don't.
func readINISection(path, section string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]string{}
	scanner := bufio.NewScanner(f)
	// Some operators have unusually long values (SSO session tokens
	// can run several KB). Bump the buffer from the default 64KB
	// line cap.
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	currentSection := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' && line[len(line)-1] == ']' {
			currentSection = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if currentSection != section {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 1 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		val := strings.TrimSpace(line[eq+1:])
		// Trim trailing inline comments (only on a leading whitespace
		// before # or ; — AWS values commonly contain `=` and may
		// legitimately contain # mid-string, so we're conservative).
		out[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// IMDS endpoint. Overridden by env so tests can point at an
// httptest.Server.
const defaultIMDSBase = "http://169.254.169.254"

// IMDSTimeout caps the total time the chain spends trying to reach
// EC2 instance metadata. Default 1s — fast enough that a non-EC2
// developer machine doesn't notice startup latency, generous
// enough that a real EC2 instance with normal network conditions
// will succeed.
const IMDSTimeout = 1 * time.Second

// AWSIMDSLookup returns a lookup function backed by EC2 instance
// metadata (IMDSv2 — the token-protected variant; the legacy
// v1 unauthenticated path is deprecated and many AMIs now disable
// it). Resolves AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY,
// AWS_SESSION_TOKEN, AWS_REGION from the instance's role.
//
// Pass `nil` for client to use a default http.Client with the
// IMDS timeout. Tests override via the URL env (AWS_EC2_METADATA_BASE).
//
// **Caching with expiry.** IMDS responses include an `Expiration`
// timestamp. We cache until 60 seconds before expiry, then refresh
// on the next lookup. A daemon long enough to outlive one set of
// metadata credentials (typically 6 hours) will silently rotate
// without operator action.
//
// **Failure mode.** Any error — non-200 from IMDS, network
// timeout, missing role — returns empty strings for every key
// from this lookup, letting the chain fall through. The error
// is NOT surfaced; debug logs would help, but the daemon already
// publishes "no credentials found" errors clearly when the whole
// chain dries up.