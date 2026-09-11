// SPDX-License-Identifier: MIT

// AWS chain constants + credential_process env scrub: EnvCredentialProcessAllowed, EnvCredentialProcessEnv, credentialProcessAWSSelectors, credentialProcessEnv.
// Code extracted from aws.go during the Day-53 god-file split. Public API unchanged.
package creds


import (
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/envscrub"
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

// EnvCredentialProcessEnv is a comma-separated allowlist of EXTRA environment
// variable names to forward to the credential_process helper, on top of the
// scrubbed base and the AWS selectors below (SEC-003).
//
// The escape hatch exists because the scrub is deliberately aggressive and some
// legitimate helpers are configured through their own environment — a
// HashiCorp Vault-backed helper needs VAULT_ADDR, and `envscrub` drops it (and
// VAULT_TOKEN, which contains "TOKEN") along with everything else unrecognised.
// Rather than weaken the default for everyone, the operator names what their
// helper actually needs.
const EnvCredentialProcessEnv = "AGEZT_AWS_CREDENTIAL_PROCESS_ENV"

// credentialProcessAWSSelectors are the AWS_* variables forwarded to the helper
// even though envscrub drops the whole AWS_ prefix as secret-shaped. None of
// these is a credential: they select WHICH identity the helper should produce
// (profile, region, config file paths). A helper that cannot see them serves the
// wrong profile or fails outright, so dropping them would break the feature
// rather than secure it. AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY /
// AWS_SESSION_TOKEN are deliberately absent — the helper's job is to PRODUCE
// those, not to receive the daemon's.
var credentialProcessAWSSelectors = []string{
	"AWS_PROFILE", "AWS_DEFAULT_PROFILE",
	"AWS_REGION", "AWS_DEFAULT_REGION",
	"AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE",
	"AWS_SDK_LOAD_CONFIG", "AWS_CA_BUNDLE",
	"AWS_ROLE_ARN", "AWS_ROLE_SESSION_NAME", "AWS_WEB_IDENTITY_TOKEN_FILE",
	"AWS_STS_REGIONAL_ENDPOINTS", "AWS_EC2_METADATA_DISABLED",
}

// credentialProcessEnv builds the helper's environment (SEC-003). Previously
// cmd.Env was never set, so the helper inherited the daemon's ENTIRE
// environment: AGEZT_VAULT_PASSPHRASE, every provider API key, the console
// password. That is backwards — a credential helper is frequently a third-party
// binary named by a config file, invoked precisely because we do NOT want to
// hold the credential ourselves, and it was handed every other secret we own.
func credentialProcessEnv() []string {
	env := envscrub.Scrubbed()
	seen := make(map[string]bool, len(credentialProcessAWSSelectors))
	forward := func(name string) {
		name = strings.ToUpper(strings.TrimSpace(name))
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	for _, name := range credentialProcessAWSSelectors {
		forward(name)
	}
	for _, name := range strings.Split(os.Getenv(EnvCredentialProcessEnv), ",") {
		forward(name)
	}
	return env
}

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
// the daemon re-creates the lookup. A `credential_process` entry is
// the one exception: helpers commonly mint TEMPORARY credentials
// (aws-vault, 1Password wrappers — anything STS-backed), so their
// output is cached only until `Expiration - refreshLead` and the
// helper is re-run on the next lookup, exactly like the IMDS cache.
// A helper that advertises no Expiration keeps the run-once behavior.
//
// Region resolution: AWS keeps region in ~/.aws/config (a *different*
// file) rather than ~/.aws/credentials. We read that too so
// operators with a vanilla `aws configure` setup don't have to
// duplicate region into their agezt env.
//
// If the file is absent or malformed, every lookup returns "" —
// the chain continues past us. No errors propagate; this is a
// best-effort source.