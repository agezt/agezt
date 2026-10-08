// SPDX-License-Identifier: MIT
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	core "github.com/agezt/agezt/kernel/update"
)

type fixtureBackend struct {
	check func(context.Context) (*core.CheckResult, error)
	apply func(context.Context, *core.UpdateInfo, func(context.Context, time.Duration) core.DrainResult) error
}

func (b fixtureBackend) Check(ctx context.Context) (*core.CheckResult, error) { return b.check(ctx) }
func (b fixtureBackend) Apply(ctx context.Context, in *core.UpdateInfo, drain func(context.Context, time.Duration) core.DrainResult) error {
	return b.apply(ctx, in, drain)
}

type privateKey struct{}

func TestUpdateCheckDisabledResultsCauseAndCallbackLifetime(t *testing.T) {
	for _, mode := range []string{"disabled", "current", "available", "error", "panic", "nil-result"} {
		t.Run(mode, func(t *testing.T) {
			cause := errors.New("owned check failure")
			var call context.Context
			callbacks := 0
			var backend Backend
			if mode != "disabled" {
				backend = fixtureBackend{check: func(ctx context.Context) (*core.CheckResult, error) {
					call = ctx
					deadline, ok := ctx.Deadline()
					if !ok || time.Until(deadline) < 59*time.Second || time.Until(deadline) > 60*time.Second || ctx.Err() != nil || ctx.Value(privateKey{}) != nil {
						t.Fatal("check context", ctx)
					}
					if mode == "panic" {
						panic("owned check panic")
					}
					if mode == "nil-result" {
						return nil, nil
					}
					if mode == "error" {
						return nil, cause
					}
					result := &core.CheckResult{Current: "backend-current", Err: cause}
					if mode == "available" {
						result.Update = &core.UpdateInfo{Version: "raw-version", SHA256: "raw-sha", URL: "raw-url", Notes: "", Provenance: core.ProvenanceGitHubRelease, Signature: "private"}
					}
					return result, nil
				}}
			}
			s := New(backend, "disabled-current", nil, nil, nil)
			parent, cancel := context.WithCancel(context.WithValue(context.Background(), privateKey{}, "private"))
			cancel()
			func() {
				defer func() {
					value := recover()
					if mode == "panic" {
						if value != "owned check panic" {
							t.Fatal(value)
						}
					} else if mode == "nil-result" {
						if value == nil {
							t.Fatal("legacy nil result no longer panics")
						}
					} else if value != nil {
						t.Fatal(value)
					}
				}()
				s.Check(parent, func(out CheckOutput, err error) {
					callbacks++
					if call != nil && call.Err() != nil {
						t.Fatal("check canceled before callback")
					}
					if mode == "error" {
						if !errors.Is(err, cause) || err.Error() != "update check failed: owned check failure" || out != (CheckOutput{}) {
							t.Fatal(out, err)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					raw, _ := json.Marshal(out)
					expected := `{"current":"backend-current","update":null,"up_to_date":true}`
					if mode == "disabled" {
						expected = `{"current":"disabled-current","update":null,"up_to_date":true,"status":"update is disabled"}`
					} else if mode == "available" {
						expected = `{"current":"backend-current","update":{"version":"raw-version","sha256":"raw-sha","url":"raw-url","notes":""},"up_to_date":false}`
					}
					if string(raw) != expected {
						t.Fatal(string(raw), expected)
					}
				})
			}()
			if call != nil && !errors.Is(call.Err(), context.Canceled) {
				t.Fatal("check resource leaked")
			}
			want := 1
			if mode == "panic" || mode == "nil-result" {
				want = 0
			}
			if callbacks != want {
				t.Fatal(callbacks, want)
			}
		})
	}
}

func TestUpdateApplyValidationAndEffectOrdering(t *testing.T) {
	for _, mode := range []string{"disabled", "decode", "missing", "success", "drain", "failure", "backend-panic", "reply-panic"} {
		t.Run(mode, func(t *testing.T) {
			var events []string
			var call context.Context
			calls := 0
			replyCalls := 0
			decode := errors.New("owned codec failure")
			failure := errors.New("owned apply failure")
			in := ApplyInput{Version: " raw-version ", SHA256: "raw-sha", URL: "raw-url", Notes: " raw-notes "}
			if mode == "decode" || mode == "disabled" {
				in.DecodeError = decode
			}
			if mode == "missing" {
				in.Version = ""
				in.SHA256 = ""
				in.URL = ""
			}
			drain := func(ctx context.Context, d time.Duration) core.DrainResult {
				if ctx != call || d != 37*time.Millisecond {
					t.Fatal("drain contract")
				}
				events = append(events, "drain")
				return core.DrainResult{Timeout: true, ActiveRuns: 7}
			}
			var backend Backend
			if mode != "disabled" {
				backend = fixtureBackend{apply: func(ctx context.Context, info *core.UpdateInfo, provided func(context.Context, time.Duration) core.DrainResult) error {
					calls++
					events = append(events, "apply")
					call = ctx
					if _, has := ctx.Deadline(); has || ctx.Err() != nil || ctx.Value(privateKey{}) != nil {
						t.Fatal("apply context")
					}
					if info.Version != in.Version || info.SHA256 != in.SHA256 || info.URL != in.URL || info.Notes != in.Notes || info.Provenance != core.ProvenanceUnverified || info.Signature != "" {
						t.Fatal("untrusted manifest changed", info)
					}
					if mode == "backend-panic" {
						panic("owned apply panic")
					}
					if mode == "drain" {
						result := provided(ctx, 37*time.Millisecond)
						if !result.Timeout || result.ActiveRuns != 7 {
							t.Fatal(result)
						}
						return fmt.Errorf("wrapped: %w", core.ErrDrainTimeout)
					}
					if mode == "failure" {
						return failure
					}
					return nil
				}}
			}
			s := New(backend, "current", drain, func() {
				if call.Err() != nil {
					t.Fatal("sentinel context canceled")
				}
				events = append(events, "sentinel")
			}, func(delay time.Duration) {
				if delay != 100*time.Millisecond || call.Err() != nil {
					t.Fatal("restart scheduling")
				}
				events = append(events, "restart")
			})
			parent, cancel := context.WithCancel(context.WithValue(context.Background(), privateKey{}, "private"))
			cancel()
			func() {
				defer func() {
					value := recover()
					if mode == "backend-panic" {
						if value != "owned apply panic" {
							t.Fatal(value)
						}
					} else if mode == "reply-panic" {
						if value != "owned reply panic" {
							t.Fatal(value)
						}
					} else if value != nil {
						t.Fatal(value)
					}
				}()
				s.Apply(parent, in, func(out ApplyOutput, err error) {
					replyCalls++
					events = append(events, "reply")
					if call != nil && call.Err() != nil {
						t.Fatal("canceled before reply")
					}
					switch mode {
					case "disabled":
						if err == nil || err.Error() != "update is disabled" || out != (ApplyOutput{}) {
							t.Fatal(out, err)
						}
					case "decode":
						if !errors.Is(err, decode) || out != (ApplyOutput{}) {
							t.Fatal(out, err)
						}
					case "missing":
						if err == nil || err.Error() != "version is required; sha256 is required; url is required" || out != (ApplyOutput{}) {
							t.Fatal(out, err)
						}
					case "drain":
						if err != nil || out.Applied || (out.Error == nil || *out.Error != "drain timed out: in-flight runs did not complete within the configured timeout") || out.Version != nil {
							t.Fatal(out, err)
						}
					case "failure":
						if err != nil || out.Applied || (out.Error == nil || *out.Error != "update failed: owned apply failure") || out.Version != nil {
							t.Fatal(out, err)
						}
					case "reply-panic":
						panic("owned reply panic")
					default:
						if err != nil || !out.Applied || out.Version == nil || *out.Version != in.Version || out.Error != nil {
							t.Fatal(out, err)
						}
					}
				})
			}()
			if call != nil && !errors.Is(call.Err(), context.Canceled) {
				t.Fatal("apply context leaked")
			}
			want := []string{"reply"}
			if mode == "success" {
				want = []string{"apply", "sentinel", "reply", "restart"}
			} else if mode == "reply-panic" {
				want = []string{"apply", "sentinel", "reply"}
			} else if mode == "drain" {
				want = []string{"apply", "drain", "reply"}
			} else if mode == "failure" {
				want = []string{"apply", "reply"}
			} else if mode == "backend-panic" {
				want = []string{"apply"}
			}
			if !reflect.DeepEqual(events, want) {
				t.Fatal(events, want)
			}
			expectedCalls := 1
			if mode == "disabled" || mode == "decode" || mode == "missing" {
				expectedCalls = 0
			}
			if calls != expectedCalls {
				t.Fatal(calls, expectedCalls)
			}
			expectedReply := 1
			if mode == "backend-panic" {
				expectedReply = 0
			}
			if replyCalls != expectedReply {
				t.Fatal(replyCalls, expectedReply)
			}
		})
	}
}
