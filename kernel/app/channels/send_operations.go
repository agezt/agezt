// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

type SendRequest struct {
	Channel json.RawMessage `json:"channel,omitempty"`
	To      json.RawMessage `json:"to,omitempty"`
	Text    json.RawMessage `json:"text,omitempty"`
}

func sendText(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	text, _ := value.(string)
	return text
}

func SendOperations(provider func(context.Context) *Outbound) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("channel sender service provider required")
	}
	op, err := app.NewOperation(opapi.Spec{Name: "send", Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true,
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"channel":{},"to":{},"text":{}}}`),
		HTTP:        opapi.HTTP{Method: "POST", Path: "/api/send"}}, func(ctx context.Context, in SendRequest) (output SendOutput, cause error) {
		// Retain native panic opacity; Send releases sender resources on unwind.
		defer func() {
			if recover() != nil {
				output = SendOutput{}
				cause = errors.New("internal error")
			}
		}()
		provider(ctx).Send(ctx, SendInput{Channel: sendText(in.Channel), To: sendText(in.To), Text: sendText(in.Text)}, func(out SendOutput, err error) { output, cause = out, err })
		return output, cause
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}
