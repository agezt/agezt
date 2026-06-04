// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// CmdStatus surfaces the resolved AWS credential chain (M307) so an operator on
// EKS can confirm IRSA/ambient credentials engaged from `agt status` rather than
// the boot banner. Omitted when AWS credentials aren't configured.
func TestStatusSurfacesCredChain(t *testing.T) {
	_, srv, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Unset → the field is omitted (no noise for non-AWS operators).
	res, err := c.Call(context.Background(), controlplane.CmdStatus, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := res["cred_chain"]; present {
		t.Errorf("cred_chain should be omitted when unset, got %v", res["cred_chain"])
	}

	// After the daemon records the chain → surfaced verbatim.
	srv.SetCredChain("AWS chain: vault → env → web_identity=EksPodRole → default(file+IMDS)")
	res, err = c.Call(context.Background(), controlplane.CmdStatus, nil)
	if err != nil {
		t.Fatal(err)
	}
	cc, _ := res["cred_chain"].(string)
	if !strings.Contains(cc, "web_identity=EksPodRole") {
		t.Errorf("cred_chain = %q, want the web-identity layer surfaced", cc)
	}
}
