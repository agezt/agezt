// SPDX-License-Identifier: MIT

package restapiapi

// The rows the daemon serves, and the rows the contract declares.
//
// The same law the board's rows follow: a RESPONSE row is the STABLE SUBSET
// of the payload — every field the contract names must be on the row the
// daemon serves, under the same name and a type the contract allows, and the
// served row may carry more (the server may add a field to a response).
//
// The two rows here are the ones a DOMAIN contract owns:
// kernel/contract/remoteapi owns the artifact index's rows and
// kernel/contract/updateapi owns the self-update surface's rows. The REST
// surface serves them, so the daemon's answers and the SDKs' vocabulary are
// held to each other.

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/remoteapi"
	"github.com/agezt/agezt/kernel/contract/updateapi"
)

// domainRows are the rows the daemon serves, by the contract's row name.
var domainRows = []struct {
	ContractRow string
	Sent        reflect.Type
}{
	{RowArtifacts, reflect.TypeOf(remoteapi.ArtifactList{})},
	{RowUpdateApply, reflect.TypeOf(updateapi.Release{})},
	{RowHealth, reflect.TypeOf(Health{})},
	{RowModels, reflect.TypeOf(Models{})},
	{RowProbeStatus, reflect.TypeOf(ProbeStatus{})},
	{RowAPIError, reflect.TypeOf(APIError{})},
	{RowRunResult, reflect.TypeOf(RunResult{})},
	{RowRunArc, reflect.TypeOf(RunArc{})},
}

// TestTheDomainRowsAreTheStableSubsetOfTheRowsTheDaemonSends is the ratchet
// for the rows the daemon serves: the ones a domain contract owns and the
// ones the REST surface owns.
func TestTheDomainRowsAreTheStableSubsetOfTheRowsTheDaemonSends(t *testing.T) {
	for _, entry := range domainRows {
		checkSubset(t, entry.ContractRow, entry.Sent)
	}
}
