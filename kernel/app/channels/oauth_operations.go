// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

type OAuthStartRequest struct {
	Kind         json.RawMessage `json:"kind,omitempty"`
	Label        json.RawMessage `json:"label,omitempty"`
	ClientID     json.RawMessage `json:"client_id,omitempty"`
	ClientSecret json.RawMessage `json:"client_secret,omitempty"`
	RedirectURI  json.RawMessage `json:"redirect_uri,omitempty"`
	InstanceURL  json.RawMessage `json:"instance_url,omitempty"`
}
type OAuthStatusRequest struct {
	State json.RawMessage `json:"state,omitempty"`
}

// Native OAuth inputs have always treated missing and non-string values as empty.
func oauthText(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	text, _ := value.(string)
	return text
}

func OAuthOperations(provider func(context.Context) *OAuth) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("channel OAuth service provider required")
	}
	spec := func(name string, read bool, properties string) opapi.Spec {
		return opapi.Spec{Name: name, ReadOnly: read, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true,
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{` + properties + `}}`)}
	}
	startSpec := spec(opapi.ChannelOAuthStart, false, `"kind":{"type":"string"},"label":{"type":"string"},"client_id":{"type":"string"},"client_secret":{"type":"string"},"redirect_uri":{"type":"string"},"instance_url":{"type":"string"}`)
	start, err := app.NewOperation(startSpec, func(ctx context.Context, in OAuthStartRequest) (OAuthStartOutput, error) {
		return provider(ctx).Start(ctx, OAuthStartInput{Kind: oauthText(in.Kind), Label: oauthText(in.Label), ClientID: oauthText(in.ClientID), ClientSecret: oauthText(in.ClientSecret), RedirectURI: oauthText(in.RedirectURI), InstanceURL: oauthText(in.InstanceURL)})
	})
	if err != nil {
		return nil, err
	}
	// The public GET callback is an existing Web UI page which forwards with its
	// own operator credential. This internal operation declares no public HTTP
	// route. Its request row is the callback's own: the code and the state the
	// provider sent, and the declared input is derived from that row.
	callback, err := app.NewOperation(opapi.Spec{Name: opapi.ChannelOAuthCallback, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in opapi.ChannelOAuthCallbackRequest) (OAuthCallbackOutput, error) {
		return provider(ctx).Callback(ctx, OAuthCallbackInput{Code: in.Code, State: in.State})
	})
	if err != nil {
		return nil, err
	}
	statusSpec := spec(opapi.ChannelOAuthStatus, true, `"state":{"type":"string"}`)
	status, err := app.NewOperation(statusSpec, func(ctx context.Context, in OAuthStatusRequest) (OAuthStatusOutput, error) {
		return provider(ctx).Status(ctx, OAuthStatusInput{State: oauthText(in.State)})
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{start, callback, status}, nil
}
