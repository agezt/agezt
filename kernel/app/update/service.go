// SPDX-License-Identifier: MIT
package update

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	core "github.com/agezt/agezt/kernel/update"
)

type ApplyInput struct {
	Version, SHA256, URL, Notes string
	DecodeError                 error
}

// Service retains callback delivery until native response lifetime is represented
// by the shared operation boundary. Ports never expose a real binary to tests.
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

func (s *Service) Check(_ context.Context, reply func(map[string]any, error)) {
	if s.backend == nil {
		reply(map[string]any{"current": s.current, "update": nil, "up_to_date": true, "status": "update is disabled"}, nil)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	result, err := s.backend.Check(ctx)
	if err != nil {
		reply(nil, fmt.Errorf("update check failed: %w", err))
		return
	}
	if result.Update == nil {
		reply(map[string]any{"current": result.Current, "update": nil, "up_to_date": true}, nil)
		return
	}
	info := result.Update
	reply(map[string]any{"current": result.Current, "up_to_date": false, "update": map[string]any{"version": info.Version, "sha256": info.SHA256, "url": info.URL, "notes": info.Notes}}, nil)
}

func (s *Service) Apply(_ context.Context, in ApplyInput, reply func(map[string]any, error)) {
	if s.backend == nil {
		reply(nil, errors.New("update is disabled"))
		return
	}
	if in.DecodeError != nil {
		reply(nil, in.DecodeError)
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
		reply(nil, errors.New(strings.Join(missing, "; ")))
		return
	}
	// Caller-provided fields never earn release-source provenance or a signature.
	info := &core.UpdateInfo{Version: in.Version, SHA256: in.SHA256, URL: in.URL, Notes: in.Notes}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := s.backend.Apply(ctx, info, s.drain)
	if err != nil {
		message := fmt.Sprintf("update failed: %v", err)
		if errors.Is(err, core.ErrDrainTimeout) {
			message = "drain timed out: in-flight runs did not complete within the configured timeout"
		}
		reply(map[string]any{"applied": false, "error": message}, nil)
		return
	}
	s.sentinel()
	reply(map[string]any{"applied": true, "version": in.Version}, nil)
	s.restart(100 * time.Millisecond)
}
