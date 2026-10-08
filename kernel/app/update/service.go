// SPDX-License-Identifier: MIT
package update

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/contract/opapi"
	core "github.com/agezt/agezt/kernel/update"
)

type ApplyInput struct {
	Version, SHA256, URL, Notes string
	DecodeError                 error
}

type ReleaseOutput struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	URL     string `json:"url"`
	Notes   string `json:"notes"`
}
type CheckOutput struct {
	Current  string         `json:"current"`
	Update   *ReleaseOutput `json:"update"`
	UpToDate bool           `json:"up_to_date"`
	Status   *string        `json:"status,omitempty"`
}
type ApplyOutput struct {
	Applied bool    `json:"applied"`
	Error   *string `json:"error,omitempty"`
	Version *string `json:"version,omitempty"`
}

// Service owns presentation and effect ordering. Direct callback lifetime is
// retained; terminal ports extend ownership through transport response writing.
type Service struct {
	backend  Backend
	current  string
	drain    func(context.Context, time.Duration) core.DrainResult
	sentinel func()
	restart  func(time.Duration)
}

func New(backend Backend, current string, drain func(context.Context, time.Duration) core.DrainResult, sentinel func(), restart func(time.Duration)) *Service {
	return &Service{backend: backend, current: current, drain: drain, sentinel: sentinel, restart: restart}
}

func (s *Service) Check(parent context.Context, reply func(CheckOutput, error)) {
	if s.backend == nil {
		status := "update is disabled"
		reply(CheckOutput{Current: s.current, UpToDate: true, Status: &status}, nil)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	retained := false
	defer func() {
		if !retained {
			cancel()
		}
	}()
	deliver := func(out CheckOutput, err error) {
		retained = opapi.DeferTerminalCleanup(parent, cancel)
		reply(out, err)
	}
	result, err := s.backend.Check(ctx)
	if err != nil {
		deliver(CheckOutput{}, fmt.Errorf("update check failed: %w", err))
		return
	}
	if result.Update == nil {
		deliver(CheckOutput{Current: result.Current, UpToDate: true}, nil)
		return
	}
	info := result.Update
	deliver(CheckOutput{Current: result.Current, Update: &ReleaseOutput{Version: info.Version, SHA256: info.SHA256, URL: info.URL, Notes: info.Notes}}, nil)
}

func (s *Service) Apply(parent context.Context, in ApplyInput, reply func(ApplyOutput, error)) {
	if s.backend == nil {
		reply(ApplyOutput{}, errors.New("update is disabled"))
		return
	}
	if in.DecodeError != nil {
		reply(ApplyOutput{}, in.DecodeError)
		return
	}
	var missing []string
	if in.Version == "" {
		missing = append(missing, "version is required")
	}
	if in.SHA256 == "" {
		missing = append(missing, "sha256 is required")
	}
	if in.URL == "" {
		missing = append(missing, "url is required")
	}
	if len(missing) > 0 {
		reply(ApplyOutput{}, errors.New(strings.Join(missing, "; ")))
		return
	}
	// Caller-provided fields never earn release-source provenance or a signature.
	info := &core.UpdateInfo{Version: in.Version, SHA256: in.SHA256, URL: in.URL, Notes: in.Notes}
	ctx, cancel := context.WithCancel(context.Background())
	retained := false
	defer func() {
		if !retained {
			cancel()
		}
	}()
	err := s.backend.Apply(ctx, info, s.drain)
	if err != nil {
		message := fmt.Sprintf("update failed: %v", err)
		if errors.Is(err, core.ErrDrainTimeout) {
			message = "drain timed out: in-flight runs did not complete within the configured timeout"
		}
		retained = opapi.DeferTerminalCleanup(parent, cancel)
		reply(ApplyOutput{Error: &message}, nil)
		return
	}
	s.sentinel()
	retained = opapi.DeferTerminalCleanup(parent, cancel)
	deferred := opapi.AfterTerminalWrite(parent, func() { s.restart(100 * time.Millisecond) })
	reply(ApplyOutput{Applied: true, Version: &in.Version}, nil)
	if !deferred {
		s.restart(100 * time.Millisecond)
	}
}
