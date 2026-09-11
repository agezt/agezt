// SPDX-License-Identifier: MIT

// AWS IMDS (EC2 metadata) + default chain: AWSIMDSLookup + imdsCache.lookup + fetchIMDSCreds + imdsGet + readBody + AWSDefaultChain.
// Code extracted from aws.go during the Day-53 god-file split. Public API unchanged.
package creds


import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)


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
