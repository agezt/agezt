// SPDX-License-Identifier: MIT
package market

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	core "github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/platform/schema"
	"reflect"
)

type InstallRequest struct {
	Name        json.RawMessage `json:"name,omitempty"`
	Marketplace json.RawMessage `json:"marketplace,omitempty"`
	Version     json.RawMessage `json:"version,omitempty"`
}
type WriteNameRequest struct {
	Name json.RawMessage `json:"name,omitempty"`
}
type AddSourceRequest struct {
	Name   json.RawMessage `json:"name,omitempty"`
	URL    json.RawMessage `json:"url,omitempty"`
	PubKey json.RawMessage `json:"pubkey,omitempty"`
}

var writeInputSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"name":{},"marketplace":{},"version":{},"url":{},"pubkey":{}}}`)
var marketProgressSchema = func() json.RawMessage {
	var frame map[string]any
	if err := json.Unmarshal([]byte(event.WireSchema), &frame); err != nil {
		panic(err)
	}
	raw, err := schema.FromType(reflect.TypeFor[core.Event](), true)
	if err != nil {
		panic(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		panic(err)
	}
	frame["properties"].(map[string]any)["payload"] = payload
	frame["required"] = append(frame["required"].([]any), "payload")
	out, err := json.Marshal(frame)
	if err != nil {
		panic(err)
	}
	return out
}()

func marketProgress(kind event.Kind, subject string, emit func(event.Event) error) func(core.Event) error {
	return func(progress core.Event) error {
		raw, err := json.Marshal(progress)
		if err != nil {
			return err
		}
		return emit(event.Event{Kind: kind, Subject: subject, Actor: "market", Payload: raw})
	}
}
func WriteOperations(provider func(context.Context) *Writes) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("market writer provider required")
	}
	spec := func(name string, stream bool) opapi.Spec {
		s := opapi.Spec{Name: name, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: writeInputSchema}
		if stream {
			s.Stream = opapi.StreamEvents
			s.EmissionSchema = marketProgressSchema
		}
		return s
	}
	install, err := app.NewStreamingOperation(spec("market_install", true), func(ctx context.Context, in InstallRequest, emit func(event.Event) error) (InstallOutput, error) {
		return provider(ctx).Install(ctx, InstallInput{Name: readText(in.Name), Marketplace: readText(in.Marketplace), Version: readText(in.Version)}, marketProgress(event.KindMarketInstallProgress, "market.install", emit))
	})
	if err != nil {
		return nil, err
	}
	uninstall, err := app.NewStreamingOperation(spec("market_uninstall", true), func(ctx context.Context, in WriteNameRequest, emit func(event.Event) error) (UninstallOutput, error) {
		return provider(ctx).Uninstall(ctx, UninstallInput{Name: readText(in.Name)}, marketProgress(event.KindMarketUninstallProgress, "market.uninstall", emit))
	})
	if err != nil {
		return nil, err
	}
	add, err := app.NewOperation(spec("market_add_source", false), func(ctx context.Context, in AddSourceRequest) (AddSourceOutput, error) {
		return provider(ctx).AddSource(ctx, AddSourceInput{Name: readText(in.Name), URL: readText(in.URL), PubKey: readText(in.PubKey)})
	})
	if err != nil {
		return nil, err
	}
	remove, err := app.NewOperation(spec("market_remove_source", false), func(ctx context.Context, in WriteNameRequest) (RemoveSourceOutput, error) {
		return provider(ctx).RemoveSource(ctx, RemoveSourceInput{Name: readText(in.Name)})
	})
	if err != nil {
		return nil, err
	}
	sync, err := app.NewOperation(spec("market_sync", false), func(ctx context.Context, in WriteNameRequest) (SyncOutput, error) {
		return provider(ctx).Sync(ctx, SyncInput{Name: readText(in.Name)})
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{install, uninstall, add, remove, sync}, nil
}
