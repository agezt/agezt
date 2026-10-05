// SPDX-License-Identifier: MIT

package providers

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestOAuthCallbackBusinessAdmissionBlocksExchangeAndEffects(t *testing.T) {
	for _, tc := range []struct {
		name           string
		in             providerCallbackInput
		want           providerCallbackResult
		state, message string
	}{
		{"denial", providerCallbackInput{Error: "denied", State: "wrong"}, providerCallbackResult{Message: "Authorization was denied.", Close: true}, "error", "authorization denied: denied"},
		{"missing-code", providerCallbackInput{State: "expected"}, providerCallbackResult{Message: "Invalid or expired sign-in. Start again from the console."}, "pending", ""},
		{"wrong-state", providerCallbackInput{Code: "fixture-code", State: "wrong"}, providerCallbackResult{Message: "Invalid or expired sign-in. Start again from the console."}, "pending", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			effects := 0
			auth := NewOAuth(nil, t.TempDir(), func() ([]string, string) { effects++; return nil, "" })
			login := &providerLogin{state: "expected", verifier: "fixture-verifier", status: "pending"}
			result := auth.completeProviderLogin(context.Background(), login, tc.in, func(context.Context, string, string) error { effects++; return nil })
			if result != tc.want || effects != 0 || login.status != tc.state || login.errMsg != tc.message || auth.chatgpt != nil {
				t.Fatalf("result/state/effects = %+v %q %q %d manager=%v", result, login.status, login.errMsg, effects, auth.chatgpt)
			}
		})
	}
}

func TestOAuthCallbackBusinessSuccessPreservesIdentityBudgetAndEffectOrder(t *testing.T) {
	type parentKey struct{}
	parent := context.WithValue(context.Background(), parentKey{}, "parent-marker")
	login := &providerLogin{state: "expected", verifier: "fixture-verifier", status: "pending"}
	var order []string
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), OnReload: func() error {
		if login.status != "done" || login.errMsg != "" {
			t.Error("reload entered before terminal login state")
		}
		order = append(order, "reload")
		return errors.New("legacy ignored reload failure")
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	auth := NewOAuth(k, dir, func() ([]string, string) {
		order = append(order, "models")
		return []string{}, ""
	})
	var exchangeContext context.Context
	result := auth.completeProviderLogin(parent, login, providerCallbackInput{Code: "fixture-code", State: "expected"}, func(ctx context.Context, code, verifier string) error {
		exchangeContext = ctx
		if ctx.Value(parentKey{}) != "parent-marker" || code != "fixture-code" || verifier != "fixture-verifier" {
			t.Fatal("exchange lost parent/code/verifier")
		}
		deadline, ok := ctx.Deadline()
		if remaining := time.Until(deadline); !ok || remaining <= 29*time.Second || remaining > 30*time.Second {
			t.Fatalf("exchange deadline changed: %v %v", deadline, ok)
		}
		order = append(order, "exchange")
		return nil
	})
	if result != (providerCallbackResult{Success: true, Close: true}) || !reflect.DeepEqual(order, []string{"exchange", "reload", "models"}) || auth.chatgpt != nil {
		t.Fatalf("success/order = %+v %v manager=%v", result, order, auth.chatgpt)
	}
	if !errors.Is(exchangeContext.Err(), context.Canceled) {
		t.Fatal("exchange context was not released")
	}
}

func TestOAuthCallbackBusinessExchangeFailureAndCancellationPreserveTerminalShape(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		parent, cancel := context.WithCancel(context.Background())
		defer cancel()
		cause := errors.New("fixture exchange failed")
		if canceled {
			cancel()
			cause = context.Canceled
		}
		models := 0
		auth := NewOAuth(nil, t.TempDir(), func() ([]string, string) { models++; return nil, "" })
		login := &providerLogin{state: "expected", verifier: "fixture-verifier", status: "pending"}
		result := auth.completeProviderLogin(parent, login, providerCallbackInput{Code: "fixture-code", State: "expected"}, func(ctx context.Context, _, _ string) error {
			if canceled && !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatal("parent cancellation lost")
			}
			return cause
		})
		if result != (providerCallbackResult{Message: cause.Error(), Close: true}) || login.status != "error" || login.errMsg != cause.Error() || models != 0 {
			t.Fatalf("exchange failure changed: %+v status=%s message=%s models=%d", result, login.status, login.errMsg, models)
		}
	}
}
