// SPDX-License-Identifier: MIT

package board

import "github.com/agezt/agezt/kernel/contract/boardapi"

// The board's outputs carry the CONTRACT's row.
//
// The row itself lives in the contract layer (kernel/contract/boardapi): the
// REST mailbox surface, the control plane's board operations and the SDKs all
// serve it, and none of them may spell its field names a second time. This
// package keeps the name (Message) for its callers, so the question is
// whether the name still IS the contract's row or a copy that drifted.
//
// The function-type form is deliberate: an ASSIGNMENT would also accept a
// copy with the same fields, but a function type requires IDENTITY — so a copy
// declared here would not compile.
var (
	_ func(ReadOutput) []boardapi.Mail    = func(o ReadOutput) []boardapi.Mail { return o.Messages }
	_ func(HelpOutput) []boardapi.Mail    = func(o HelpOutput) []boardapi.Mail { return o.OpenHelp }
	_ func(InboxOutput) []boardapi.Mail   = func(o InboxOutput) []boardapi.Mail { return o.Waiting }
	_ func(GetOutput) boardapi.Mail       = func(o GetOutput) boardapi.Mail { return o.Message }
	_ func(RepliesOutput) []boardapi.Mail = func(o RepliesOutput) []boardapi.Mail {
		return o.Replies
	}
	_ func(SendOutput) boardapi.Mail = func(o SendOutput) boardapi.Mail { return o.Sent }
)
