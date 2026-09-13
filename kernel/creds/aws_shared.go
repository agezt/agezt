// SPDX-License-Identifier: MIT

// AWS shared credentials loader (M1.q): the entry point +
// AWSSharedCredentialsLookup.
// The credential_process sub-command parser lives in
// aws_shared_process.go; the file-loading path (loadAWSSharedFiles +
// awsConfigFilePath + readINISection) + the IMDS endpoint + timeout
// constants live in aws_shared_files.go.
// Extracted from aws_shared.go during the Day-204 god-file split.
// Public API unchanged.
package creds

import (
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
