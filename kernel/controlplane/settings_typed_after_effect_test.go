// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	appsettings "github.com/agezt/agezt/kernel/app/settings"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/providers/mock"
	"net"
	"strings"
	"testing"
	"time"
)

type closingSettingsWriter struct {
	nativeSettingsWriter
	k *runtime.Kernel
}
type closingSettingsStore struct {
	appsettings.ValueStore
	k *runtime.Kernel
}

func (s closingSettingsStore) Save() error {
	if err := s.ValueStore.Save(); err != nil {
		return err
	}
	return s.k.Journal().Close()
}
func (w closingSettingsWriter) ConfigStore() appsettings.ValueStore {
	return closingSettingsStore{ValueStore: w.nativeSettingsWriter.ConfigStore(), k: w.k}
}
func (w closingSettingsWriter) Register(sec appsettings.Section) error {
	if err := w.nativeSettingsWriter.Register(sec); err != nil {
		return err
	}
	return w.k.Journal().Close()
}
func (w closingSettingsWriter) Unregister(id string, force bool) (bool, error) {
	existed, err := w.nativeSettingsWriter.Unregister(id, force)
	if err != nil {
		return existed, err
	}
	return existed, w.k.Journal().Close()
}
func TestSettingsTypedNativeJournalFailureAfterEffectsDoesNotRollback(t *testing.T) {
	for _, command := range []string{CmdConfigSet, CmdConfigSchemaRegister, CmdConfigSchemaUnregister} {
		t.Run(command, func(t *testing.T) {
			dir := t.TempDir()
			p := mock.New()
			k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: p})
			if err != nil {
				t.Fatal(err)
			}
			defer k.Close()
			reg := settings.NewRegistry(dir)
			seed := settings.Section{ID: "owned-seed", Name: "Owned", Fields: []settings.Field{{Env: "AGEZT_W45C_END_PUBLIC", Type: settings.TypeText}}}
			if err := reg.Register(seed); err != nil {
				t.Fatal(err)
			}
			s := NewServer(k, dir)
			s.token = "primary"
			saved := settingsOperations
			defer func() { settingsOperations = saved }()
			settingsOperations, err = appsettings.Operations(func(context.Context) *appsettings.Reads { return s.settingsReads() }, func(context.Context) *appsettings.Writes {
				return appsettings.NewWrites(closingSettingsWriter{nativeSettingsWriter: nativeSettingsWriter{server: s}, k: k})
			})
			if err != nil {
				t.Fatal(err)
			}
			args := map[string]any{}
			switch command {
			case CmdConfigSet:
				args = map[string]any{"name": "AGEZT_W45C_END_PUBLIC", "value": "owned"}
			case CmdConfigSchemaRegister:
				args = map[string]any{"section": map[string]any{"id": "owned-add", "name": "Added", "fields": []any{map[string]any{"env": "AGEZT_W45C_END_ADDED", "type": "text"}}}}
			case CmdConfigSchemaUnregister:
				args = map[string]any{"id": "owned-seed"}
			}
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			a.SetDeadline(time.Now().Add(3 * time.Second))
			done := make(chan struct{})
			go func() { defer close(done); s.handleConn(context.Background(), b) }()
			raw, _ := json.Marshal(Request{ID: "owned", Cmd: command, Token: "primary", Args: args})
			a.Write(append(raw, 10))
			line, err := bufio.NewReader(a).ReadBytes(10)
			if err != nil {
				t.Fatal(err)
			}
			a.Close()
			<-done
			var reply Response
			json.Unmarshal(line, &reply)
			effect := false
			switch command {
			case CmdConfigSet:
				store := settings.NewStore(dir)
				store.Load()
				value, ok := store.Get("AGEZT_W45C_END_PUBLIC")
				effect = ok && value == "owned"
			case CmdConfigSchemaRegister:
				_, effect = reg.FieldByEnv("AGEZT_W45C_END_ADDED")
			case CmdConfigSchemaUnregister:
				_, present := reg.FieldByEnv("AGEZT_W45C_END_PUBLIC")
				effect = !present
			}
			if reply.Type != RespError || !strings.Contains(reply.Error, "journal:") || !effect || p.CallCount() != 0 {
				t.Fatal(reply, effect, p.CallCount())
			}
		})
	}
}
