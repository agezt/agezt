// SPDX-License-Identifier: MIT
//
// kernel/creds web-identity STS response XML decoder (parseWebIdentityResponse).
// Extracted from web_identity.go during Day 211 god-file refactor (#93).
// Public API unchanged.
package creds

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/creds/sigv4"
)

func parseWebIdentityResponse(resp *http.Response) (*AssumedCreds, error) {
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("sts web-identity: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		excerpt := string(raw)
		if len(excerpt) > 512 {
			excerpt = strutil.Ellipsis(excerpt, 512, "...")
		}
		return nil, fmt.Errorf("sts web-identity: %s: %s", resp.Status, excerpt)
	}

	var env struct {
		XMLName xml.Name `xml:"AssumeRoleWithWebIdentityResponse"`
		Result  struct {
			Credentials struct {
				AccessKeyID     string `xml:"AccessKeyId"`
				SecretAccessKey string `xml:"SecretAccessKey"`
				SessionToken    string `xml:"SessionToken"`
				Expiration      string `xml:"Expiration"`
			} `xml:"Credentials"`
		} `xml:"AssumeRoleWithWebIdentityResult"`
	}
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("sts web-identity: parse XML: %w", err)
	}
	c := env.Result.Credentials
	if c.AccessKeyID == "" || c.SecretAccessKey == "" || c.SessionToken == "" {
		return nil, fmt.Errorf("sts web-identity: response missing credential fields: %s", string(raw))
	}
	exp, err := time.Parse(time.RFC3339, c.Expiration)
	if err != nil {
		return nil, fmt.Errorf("sts web-identity: parse Expiration %q: %w", c.Expiration, err)
	}
	return &AssumedCreds{
		Creds: sigv4.Creds{
			AccessKeyID:     c.AccessKeyID,
			SecretAccessKey: c.SecretAccessKey,
			SessionToken:    c.SessionToken,
		},
		Expiration: exp,
	}, nil
}
