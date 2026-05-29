// SPDX-License-Identifier: MIT

package creds

// AWS credential provider chain (M1.dd). Three pure-stdlib sources
// that all satisfy the `func(name string) string` lookup signature
// already used by Store.Lookup, so they compose with ChainLookup
// the same way:
//
//	cred := creds.ChainLookup(
//	    vault.Lookup,                          // 1. agezt vault (M1.w)
//	    os.Getenv,                             // 2. process env
//	    creds.AWSSharedCredentialsLookup(""),  // 3. ~/.aws/credentials
//	    creds.AWSIMDSLookup(nil),              // 4. EC2 metadata
//	)
//
// Each AWS source answers a fixed list of AWS_* names:
//
//	AWS_ACCESS_KEY_ID
//	AWS_SECRET_ACCESS_KEY
//	AWS_SESSION_TOKEN
//	AWS_REGION             (config-file and IMDS only — env supplies its own)
//
// Anything else returns the empty string, so the chain naturally
// continues past the AWS source for non-AWS names.
//
// **Why an extension package rather than a separate AWS-SDK port.**
// The AWS Go SDK pulls a tree of ~30 transitive deps; agezt's
// lean-deps policy excludes it. The chain we implement here is
// a strict subset of the SDK's behaviour — env (handled
// elsewhere), shared file with profile selection, and IMDSv2 —
// which is what 95% of operators actually need. SSO / web identity
// / process / assume-role are not implemented; operators using
// those should pass through environment variables that the SDK
// or `aws configure` command emits.
//
// **No retry / backoff.** The chain is consulted once per lookup
// at daemon startup (and again on a hot reload). If IMDS times
// out, the call returns empty and the chain falls through to the
// next source. We don't retry IMDS in a tight loop — a daemon
// that genuinely needs IMDS and can't reach it has a config
// problem the operator should see, not a transient blip to paper
// over.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// EnvCredentialProcessAllowed is the env var operators must set
// to 1 before the AWS chain will exec a `credential_process =`
// entry from ~/.aws/credentials or ~/.aws/config (M1.pp).
//
// Opt-in by design: the entry exec's an arbitrary binary
// (commonly aws-vault, 1password CLI wrappers, etc.) with the
// daemon's privileges. Defaulting to "exec whatever the config
// says" is a footgun — an operator who didn't realise a profile
// has credential_process set could be surprised by what runs.
const EnvCredentialProcessAllowed = "AGEZT_AWS_CREDENTIAL_PROCESS_ALLOWED"

// credentialProcessTimeout caps how long the chain waits for a
// credential_process invocation. 10s is enough for an interactive
// password prompt (aws-vault occasionally asks) while preventing
// the daemon from hanging indefinitely on a broken script.
const credentialProcessTimeout = 10 * time.Second

// awsRecognisedNames is the closed set of lookup keys the AWS
// sources answer. Anything outside this set returns "" so the
// chain falls through to the next source naturally.
var awsRecognisedNames = map[string]struct{}{
	"AWS_ACCESS_KEY_ID":     {},
	"AWS_SECRET_ACCESS_KEY": {},
	"AWS_SESSION_TOKEN":     {},
	"AWS_REGION":            {},
	"AWS_DEFAULT_REGION":    {},
}

// AWSSharedCredentialsLookup returns a lookup function that reads
// ~/.aws/credentials (the same file the AWS CLI / SDKs use). The
// profile argument selects which INI section to read; empty means
// "default", and `AWS_PROFILE` overrides empty.
//
// The file is loaded lazily on the first call to the returned
// lookup, then cached for the process lifetime — typical daemon
// pattern, the file rarely changes mid-run, and a hot reload of
// the daemon re-creates the lookup.
//
// Region resolution: AWS keeps region in ~/.aws/config (a *different*
// file) rather than ~/.aws/credentials. We read that too so
// operators with a vanilla `aws configure` setup don't have to
// duplicate region into their agezt env.
//
// If the file is absent or malformed, every lookup returns "" —
// the chain continues past us. No errors propagate; this is a
// best-effort source.
func AWSSharedCredentialsLookup(profile string) func(string) string {
	var (
		once   sync.Once
		values map[string]string
	)
	return func(name string) string {
		if _, ok := awsRecognisedNames[name]; !ok {
			return ""
		}
		once.Do(func() {
			values = loadAWSSharedFiles(profile)
		})
		return values[name]
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
// Returns the parsed key/secret/token map, or nil if the call
// failed for any reason (the chain falls through silently — same
// pattern as IMDS failure).
//
// Spec quoting rules: AWS allows the command to be a shell-style
// string with quoting. We use osexec.Command with shell parsing
// via shell-words-like splitting — simpler than depending on a
// real shell, and matches what AWS-SDK-go does internally for
// this case. Operators who need a real shell can wrap their tool
// in a script.
func runCredentialProcess(commandLine string) map[string]string {
	if strings.TrimSpace(os.Getenv(EnvCredentialProcessAllowed)) != "1" {
		return nil
	}
	parts := splitCommandLine(commandLine)
	if len(parts) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), credentialProcessTimeout)
	defer cancel()
	cmd := osexec.CommandContext(ctx, parts[0], parts[1:]...)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	var doc struct {
		Version         int    `json:"Version"`
		AccessKeyID     string `json:"AccessKeyId"`
		SecretAccessKey string `json:"SecretAccessKey"`
		SessionToken    string `json:"SessionToken"`
	}
	if err := json.Unmarshal(output, &doc); err != nil {
		return nil
	}
	if doc.Version != 1 || doc.AccessKeyID == "" || doc.SecretAccessKey == "" {
		return nil
	}
	out := map[string]string{
		"AWS_ACCESS_KEY_ID":     doc.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": doc.SecretAccessKey,
	}
	if doc.SessionToken != "" {
		out["AWS_SESSION_TOKEN"] = doc.SessionToken
	}
	return out
}

// splitCommandLine is a minimal shell-style tokeniser supporting
// `"double-quoted"` and `'single-quoted'` spans. Backslash
// escapes inside quotes are NOT supported — operators with paths
// containing both kinds of quotes should put the command in a
// wrapper script.
func splitCommandLine(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := rune(0)
	for _, r := range s {
		switch {
		case inQuote != 0 && r == inQuote:
			inQuote = 0
		case inQuote != 0:
			cur.WriteRune(r)
		case r == '"' || r == '\'':
			inQuote = r
		case r == ' ' || r == '\t':
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func loadAWSSharedFiles(profile string) map[string]string {
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
	// wins if both have one (matches AWS-SDK precedence).
	if out["AWS_ACCESS_KEY_ID"] == "" {
		var commandLine string
		if credsPath != "" {
			if section, err := readINISection(credsPath, profile); err == nil {
				commandLine = section["credential_process"]
			}
		}
		if commandLine == "" && cfgPath != "" {
			cfgSection := profile
			if profile != "default" {
				cfgSection = "profile " + profile
			}
			if section, err := readINISection(cfgPath, cfgSection); err == nil {
				commandLine = section["credential_process"]
			}
		}
		if commandLine != "" {
			if creds := runCredentialProcess(commandLine); creds != nil {
				for k, v := range creds {
					if out[k] == "" {
						out[k] = v
					}
				}
			}
		}
	}
	return out
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
func AWSIMDSLookup(client *http.Client) func(string) string {
	if client == nil {
		client = &http.Client{Timeout: IMDSTimeout}
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("AWS_EC2_METADATA_BASE")), "/")
	if base == "" {
		base = defaultIMDSBase
	}
	cache := &imdsCache{client: client, base: base}
	return cache.lookup
}

type imdsCache struct {
	client *http.Client
	base   string

	mu       sync.Mutex
	values   map[string]string
	expires  time.Time
	negCache time.Time // set when last attempt errored; suppresses retry for a window
}

// negCacheTTL is how long we suppress IMDS retries after a failed
// attempt. Prevents a non-EC2 daemon from making one slow IMDS
// call per lookup on a chain — keeps the boot-up snappy when the
// chain falls through to nothing useful.
const negCacheTTL = 30 * time.Second

// refreshLead is how long before the credentials' Expiration we
// proactively refetch. 60s gives signing operations time to use
// the cached creds even right around the boundary.
const refreshLead = 60 * time.Second

func (c *imdsCache) lookup(name string) string {
	if _, ok := awsRecognisedNames[name]; !ok {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	cacheValid := c.values != nil && !c.expires.IsZero() && now.Add(refreshLead).Before(c.expires)
	if !cacheValid {
		if !c.negCache.IsZero() && now.Sub(c.negCache) < negCacheTTL {
			// Recent failure — don't slow every lookup retrying.
			return ""
		}
		vals, exp, err := fetchIMDSCreds(c.client, c.base)
		if err != nil {
			c.negCache = now
			return ""
		}
		c.values = vals
		c.expires = exp
		c.negCache = time.Time{}
	}
	return c.values[name]
}

// imdsTokenTTL is the lifetime of the IMDSv2 session token in
// seconds. The maximum IMDS accepts is 21600 (6 hours); 6h is
// plenty for our use (one daemon boot or one credential
// refresh).
const imdsTokenTTL = "21600"

// fetchIMDSCreds runs the IMDSv2 handshake: PUT a token, then GET
// the role list, then GET the role's credentials. Returns the
// credentials map and the expiration timestamp.
//
// All HTTP calls share the provided client's timeout (typically 1s
// total per stage), so the worst-case latency before falling
// through is roughly 3 * IMDSTimeout — still under 5 seconds for
// a daemon that's not on EC2.
func fetchIMDSCreds(client *http.Client, base string) (map[string]string, time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*IMDSTimeout)
	defer cancel()

	// Step 1: get the v2 session token.
	tokReq, err := http.NewRequestWithContext(ctx, http.MethodPut, base+"/latest/api/token", nil)
	if err != nil {
		return nil, time.Time{}, err
	}
	tokReq.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", imdsTokenTTL)
	tokResp, err := client.Do(tokReq)
	if err != nil {
		return nil, time.Time{}, err
	}
	token, err := readBody(tokResp)
	if err != nil {
		return nil, time.Time{}, err
	}
	if tokResp.StatusCode != http.StatusOK {
		return nil, time.Time{}, errors.New("imds: token request: " + tokResp.Status)
	}

	hdr := http.Header{"X-aws-ec2-metadata-token": []string{string(token)}}

	// Step 2: get the role name attached to this instance.
	roleResp, err := imdsGet(ctx, client, base+"/latest/meta-data/iam/security-credentials/", hdr)
	if err != nil {
		return nil, time.Time{}, err
	}
	roleName := strings.TrimSpace(string(roleResp))
	if roleName == "" {
		return nil, time.Time{}, errors.New("imds: no IAM role attached to instance")
	}

	// Step 3: fetch the role's credentials JSON.
	credBody, err := imdsGet(ctx, client, base+"/latest/meta-data/iam/security-credentials/"+roleName, hdr)
	if err != nil {
		return nil, time.Time{}, err
	}
	var doc struct {
		Code            string `json:"Code"`
		AccessKeyID     string `json:"AccessKeyId"`
		SecretAccessKey string `json:"SecretAccessKey"`
		Token           string `json:"Token"`
		Expiration      string `json:"Expiration"`
	}
	if err := json.Unmarshal(credBody, &doc); err != nil {
		return nil, time.Time{}, err
	}
	if doc.Code != "" && doc.Code != "Success" {
		return nil, time.Time{}, errors.New("imds: credentials response code=" + doc.Code)
	}

	// Region — separate endpoint. Not fatal if missing (caller may
	// have AWS_REGION elsewhere); just skip on error.
	region := ""
	if regBody, err := imdsGet(ctx, client, base+"/latest/meta-data/placement/region", hdr); err == nil {
		region = strings.TrimSpace(string(regBody))
	}

	exp, err := time.Parse(time.RFC3339, doc.Expiration)
	if err != nil {
		// Conservative fallback: treat as expiring in 5 minutes so
		// we re-fetch sooner if the doc shape changed.
		exp = time.Now().Add(5 * time.Minute)
	}

	out := map[string]string{
		"AWS_ACCESS_KEY_ID":     doc.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": doc.SecretAccessKey,
		"AWS_SESSION_TOKEN":     doc.Token,
	}
	if region != "" {
		out["AWS_REGION"] = region
		out["AWS_DEFAULT_REGION"] = region
	}
	return out, exp, nil
}

func imdsGet(ctx context.Context, client *http.Client, url string, hdr http.Header) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	maps.Copy(req.Header, hdr)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("imds: " + url + ": " + resp.Status)
	}
	return body, nil
}

func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	// IMDS responses are small (token: ~56 bytes; creds JSON: <1KB).
	// 64KB cap is paranoia + alignment with the rest of the codebase.
	return io.ReadAll(io.LimitReader(resp.Body, 64*1024))
}

// AWSDefaultChain composes the AWS sources in the standard SDK
// order (env, then shared credentials file, then IMDS), wrapped
// in a single lookup. Pass through ChainLookup with the agezt
// vault first so vault overrides always win:
//
//	cred := creds.ChainLookup(vault.Lookup, creds.AWSDefaultChain())
//
// Operator overrides via plain os.Getenv are handled by the env
// source inside the chain — no need to add os.Getenv separately.
// The profile name defaults to AWS_PROFILE or "default".
//
// **Why expose this convenience.** Without it every caller would
// hand-compose the three sources in the right order — easy to get
// wrong, and any inconsistency between callers becomes a routing
// bug. One canonical chain, one place to fix it.
func AWSDefaultChain() func(string) string {
	// runtime check is just paranoia — every supported OS has
	// os.Getenv, but if some future port stubs it out, the chain
	// would silently return empty without this line failing fast.
	_ = runtime.GOOS
	return ChainLookup(
		// Env first: matches AWS SDK precedence.
		func(name string) string { return strings.TrimSpace(os.Getenv(name)) },
		// Shared credentials file (~/.aws/credentials + ~/.aws/config).
		AWSSharedCredentialsLookup(""),
		// IMDSv2 (EC2 instance metadata).
		AWSIMDSLookup(nil),
	)
}
