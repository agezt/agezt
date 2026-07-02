// SPDX-License-Identifier: MIT

package executionprofile

import (
	"os"
	"strings"
)

const RemoteSecretPolicyEnv = "AGEZT_EXEC_REMOTE_SECRET_POLICY"

type SecretPolicy struct {
	Mode              string `json:"mode"`
	Scope             string `json:"scope"`
	ValuesForwarded   bool   `json:"values_forwarded"`
	MetadataForwarded bool   `json:"metadata_forwarded"`
	Valid             bool   `json:"valid"`
	Detail            string `json:"detail"`
}

func RemoteSecretPolicyFromEnv() SecretPolicy {
	return ParseRemoteSecretPolicy(os.Getenv(RemoteSecretPolicyEnv))
}

func ParseRemoteSecretPolicy(raw string) SecretPolicy {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "", "off", "none", "deny":
		return SecretPolicy{
			Mode:   "deny",
			Scope:  "remote/cloud",
			Valid:  true,
			Detail: "local secret values and secret metadata are not exported to remote-agezt or cloud profiles",
		}
	case "metadata", "metadata-only", "names", "names-only":
		return SecretPolicy{
			Mode:              "metadata",
			Scope:             "remote/cloud",
			MetadataForwarded: true,
			Valid:             true,
			Detail:            "remote/cloud adapters may receive secret names or labels only; local secret values are never exported by this policy",
		}
	default:
		return SecretPolicy{
			Mode:   "deny",
			Scope:  "remote/cloud",
			Valid:  false,
			Detail: "invalid " + RemoteSecretPolicyEnv + " value; local secret export is denied",
		}
	}
}

func RemoteSecretPolicySummary(base string) string {
	p := RemoteSecretPolicyFromEnv()
	if strings.TrimSpace(base) == "" {
		base = "remote daemon or cloud adapter secrets"
	}
	return base + "; policy=" + p.Mode + ": " + p.Detail
}
