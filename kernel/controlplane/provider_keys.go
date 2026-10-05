// SPDX-License-Identifier: MIT

package controlplane

// Provider keyring (M700): store many API keys per provider and pick which is
// active — "store many, pick active". Values NEVER leave the daemon — list
// returns label + active + last-4 fingerprint only, mirroring the Config
// Center's secret privacy rule.

import (
	"context"
	"net"
	"regexp"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	appproviders "github.com/agezt/agezt/kernel/app/providers"
	"github.com/agezt/agezt/kernel/catalog"
)

// providerEnvPattern constrains a keyring target to a provider-style env var
// (UPPER_SNAKE, may start with a digit — models.dev has e.g. 302AI_API_KEY). The
// AGEZT_ namespace is the Config Center's; provider creds live outside it
// (OPENAI_API_KEY, ANTHROPIC_API_KEY, …), so reject AGEZT_ here.
var providerEnvPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_]*$`)
var providerIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// keyEnv validates and returns the env-var name from req.Args["env"].
func keyEnv(req Request) (string, bool) {
	env, _ := req.Args["env"].(string)
	env = strings.TrimSpace(env)
	if !providerEnvPattern.MatchString(env) || strings.HasPrefix(env, brand.EnvPrefix) {
		return "", false
	}
	return env, true
}

// keyTarget returns the display provider/env plus the actual vault keyring
// target. Without args.provider this preserves the legacy env-global keyring;
// with args.provider it stores under provider:<id>:<ENV>, so providers that
// share the same models.dev env name do not share API keys accidentally.
func keyTarget(req Request) (provider, env, target string, ok bool) {
	env, ok = keyEnv(req)
	if !ok {
		return "", "", "", false
	}
	provider, _ = req.Args["provider"].(string)
	provider = strings.TrimSpace(provider)
	if provider != "" {
		if !providerIDPattern.MatchString(provider) {
			return "", "", "", false
		}
		target = catalog.ProviderCredentialName(provider, env)
	} else {
		target = env
	}
	return provider, env, target, true
}

func keyTargetError() Response {
	return Response{Type: RespError, Error: "args.env must be a provider env var (UPPER_SNAKE, not AGEZT_*); args.provider, when set, must be a catalog provider id"}
}

func (s *Server) handleProviderKeyList(conn net.Conn, req Request) {
	provider, env, _, ok := keyTarget(req)
	if !ok {
		resp := keyTargetError()
		resp.ID = req.ID
		s.writeResp(conn, resp)
		return
	}
	out, err := appproviders.New(s.k, s.baseDir).KeyList(context.Background(), appproviders.KeyInput{Provider: provider, Env: env})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}
func (s *Server) handleProviderKeyAdd(conn net.Conn, req Request) {
	provider, env, _, ok := keyTarget(req)
	if !ok {
		resp := keyTargetError()
		resp.ID = req.ID
		s.writeResp(conn, resp)
		return
	}
	label, _, err := argString(req.Args, "label")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	value, _, err := argString(req.Args, "value")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	active, _, err := argBool(req.Args, "active")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appproviders.New(s.k, s.baseDir).KeyAdd(context.Background(), appproviders.KeyInput{Provider: provider, Env: env, Label: label, Value: value, Active: active})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}
func (s *Server) handleProviderKeyActivate(conn net.Conn, req Request) {
	provider, env, _, ok := keyTarget(req)
	if !ok {
		resp := keyTargetError()
		resp.ID = req.ID
		s.writeResp(conn, resp)
		return
	}
	label, _, err := argString(req.Args, "label")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appproviders.New(s.k, s.baseDir).KeyActivate(context.Background(), appproviders.KeyInput{Provider: provider, Env: env, Label: label})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}
func (s *Server) handleProviderKeyRemove(conn net.Conn, req Request) {
	provider, env, _, ok := keyTarget(req)
	if !ok {
		resp := keyTargetError()
		resp.ID = req.ID
		s.writeResp(conn, resp)
		return
	}
	label, _, err := argString(req.Args, "label")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appproviders.New(s.k, s.baseDir).KeyRemove(context.Background(), appproviders.KeyInput{Provider: provider, Env: env, Label: label})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}
