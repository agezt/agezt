// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	appconfigcenter "github.com/agezt/agezt/kernel/app/configcenter"
	core "github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type closingCenterWriter struct {
	*core.Center
	k *runtime.Kernel
}

func (w closingCenterWriter) Set(e *core.ConfigEntry) error {
	if err := w.Center.Set(e); err != nil {
		return err
	}
	return w.k.Journal().Close()
}
func (w closingCenterWriter) Delete(key string) error {
	if err := w.Center.Delete(key); err != nil {
		return err
	}
	return w.k.Journal().Close()
}

func TestConfigCenterTypedNativeJournalFailureAfterEffectsDoesNotRollback(t *testing.T) {
	for _, command := range []string{CmdConfigCenterSet, CmdConfigCenterDelete, CmdConfigCenterSetRating, CmdConfigCenterSetAccess} {
		t.Run(command, func(t *testing.T) {
			dir := t.TempDir()
			p := mock.New()
			k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: p})
			if err != nil {
				t.Fatal(err)
			}
			defer k.Close()
			seed := core.NewConfigEntry("owned", "seed")
			if err := k.ConfigCenter().Set(seed); err != nil {
				t.Fatal(err)
			}
			s := NewServer(k, dir)
			s.token = "primary"
			saved := configCenterOperations
			defer func() { configCenterOperations = saved }()
			configCenterOperations, err = appconfigcenter.Operations(func(context.Context) *appconfigcenter.Reads { return s.configCenterReads() }, func(context.Context) *appconfigcenter.Writes {
				return appconfigcenter.NewWrites(closingCenterWriter{Center: k.ConfigCenter(), k: k})
			})
			if err != nil {
				t.Fatal(err)
			}
			args := map[string]any{"key": "owned", "value": "replacement", "rating": "public", "allowed_agents": []any{"changed"}}
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			a.SetDeadline(time.Now().Add(3 * time.Second))
			done := make(chan struct{})
			go func() { defer close(done); s.handleConn(context.Background(), b) }()
			raw, _ := json.Marshal(Request{ID: "owned", Cmd: command, Token: "primary", Args: args})
			if _, err := a.Write(append(raw, 10)); err != nil {
				t.Fatal(err)
			}
			line, err := bufio.NewReader(a).ReadBytes(10)
			if err != nil {
				t.Fatal(err)
			}
			a.Close()
			<-done
			var reply Response
			if err := json.Unmarshal(line, &reply); err != nil {
				t.Fatal(err)
			}
			entry, lookupErr := k.ConfigCenter().GetEntry("owned")
			effect := false
			switch command {
			case CmdConfigCenterSet:
				effect = lookupErr == nil && entry.Value == "replacement"
			case CmdConfigCenterDelete:
				effect = lookupErr != nil
			case CmdConfigCenterSetRating:
				effect = lookupErr == nil && entry.Rating == core.RatingPublic
			case CmdConfigCenterSetAccess:
				effect = lookupErr == nil && len(entry.AllowedAgents) == 1 && entry.AllowedAgents[0] == "changed"
			}
			if reply.Type != RespError || reply.Error == "" || !effect || p.CallCount() != 0 {
				t.Fatalf("journal terminal failure lost effect/error: response=%s effect=%v", reply.Type, effect)
			}
			reopened, err := core.New(core.DefaultConfig(dir))
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			persisted, persistErr := reopened.GetEntry("owned")
			if command == CmdConfigCenterDelete {
				if persistErr == nil {
					t.Fatal("delete was rolled back on disk")
				}
			} else {
				if persistErr != nil || persisted.Value != entry.Value || persisted.Rating != entry.Rating || len(persisted.AllowedAgents) != len(entry.AllowedAgents) {
					t.Fatal("post-effect journal failure lost disk state", persistErr)
				}
			}
		})
	}
}
