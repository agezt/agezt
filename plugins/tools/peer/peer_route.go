// SPDX-License-Identifier: MIT
//
// Peer tool: Invoke + render + routeCandidates + serversForModel +
// resolve + truncate.
// Extracted from peer.go during the Day-202 god-file split.
// Public API unchanged.
package peer

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/meshctx"
)

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	var in struct {
		Peer  string `json:"peer"`
		Task  string `json:"task"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid input: " + err.Error(), IsError: true}, nil
	}
	task := strings.TrimSpace(in.Task)
	if task == "" {
		return agent.Result{Output: "task is required", IsError: true}, nil
	}
	// Mesh loop guard (M209): if this run is already at the hop limit, delegating
	// further would push the peer past it (and be refused there). Refuse locally with a
	// clear message rather than make a doomed round-trip. A non-delegated run is hop 0.
	if maxHops := meshctx.MaxHopsFromEnv(); meshctx.Hop(ctx) >= maxHops {
		return agent.Result{Output: fmt.Sprintf(
			"remote_run: mesh delegation hop limit (%d) reached — refusing to delegate further to avoid a federation loop",
			maxHops), IsError: true}, nil
	}
	model := strings.TrimSpace(in.Model)

	to := t.Timeout
	if to <= 0 {
		to = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()

	// Pick the candidate peer(s). A named peer is the sole candidate; otherwise, when a
	// model is requested and several peers are configured, auto-route across the peers
	// that serve it in order (M203).
	candidates, err := t.routeCandidates(ctx, strings.TrimSpace(in.Peer), model)
	if err != nil {
		return agent.Result{Output: err.Error(), IsError: true}, nil
	}

	// Forward the model only when the caller pinned one; an absent/empty model lets
	// the peer use its own default (the restapi runRequest falls back to it), so the
	// default behaviour is byte-for-byte unchanged.
	payload := map[string]string{"intent": task}
	if model != "" {
		payload["model"] = model
	}
	body, _ := json.Marshal(payload)

	var tried []string
	for i, peer := range candidates {
		endpoint := strings.TrimRight(peer.URL, "/") + "/api/v1/runs"
		status, respBody, perr := t.post(ctx, endpoint, peer.Token, body)
		if perr != nil {
			// Transport failure: no response means the task never ran on this peer, so
			// it is safe to fall back to the next serving peer (M206). A peer that
			// RESPONDS — even with an error status — is NOT retried elsewhere, since it
			// may already have executed side effects.
			tried = append(tried, peer.Name)
			if i+1 < len(candidates) {
				continue
			}
			if len(candidates) == 1 {
				return agent.Result{Output: fmt.Sprintf("remote_run: POST %s failed: %v", endpoint, perr), IsError: true}, nil
			}
			return agent.Result{Output: fmt.Sprintf("remote_run: all %d peers serving %q unreachable (%s); last error: %v",
				len(candidates), model, strings.Join(tried, ", "), perr), IsError: true}, nil
		}

		var resp struct {
			CorrelationID string `json:"correlation_id"`
			Status        string `json:"status"`
			Answer        string `json:"answer"`
			Error         string `json:"error"`
			Model         string `json:"model"`
		}
		_ = json.Unmarshal(respBody, &resp)

		if status < 200 || status >= 300 || resp.Status == "failed" {
			msg := resp.Error
			if msg == "" {
				msg = fmt.Sprintf("status %d", status)
			}
			out := fmt.Sprintf("remote_run on peer %q failed: %s", peer.Name, msg)
			if resp.CorrelationID != "" {
				out += fmt.Sprintf(" (peer correlation: %s)", resp.CorrelationID)
			}
			return agent.Result{Output: out, IsError: true}, nil
		}

		return agent.Result{Output: render(peer.Name, resp.Model, resp.CorrelationID, resp.Answer)}, nil
	}
	// routeCandidates returns a non-empty list or an error, so the loop always returns.
	return agent.Result{Output: "remote_run: no candidate peer", IsError: true}, nil
}

func render(peerName, model, corr, answer string) string {
	var b strings.Builder
	a := strings.TrimSpace(answer)
	if a == "" {
		b.WriteString("The peer returned no answer.")
	} else {
		b.WriteString(truncate(a, MaxAnswerBytes))
	}
	// The peer echoes the model it actually routed to; surface it so the delegating
	// node's transcript records which remote model produced the answer.
	if m := strings.TrimSpace(model); m != "" {
		fmt.Fprintf(&b, "\n\n[peer=%s model=%s correlation=%s]", peerName, m, corr)
	} else {
		fmt.Fprintf(&b, "\n\n[peer=%s correlation=%s]", peerName, corr)
	}
	return b.String()
}

// routeCandidates returns the ordered peer(s) the task may run on. A named peer is
// the sole candidate. With no name, a requested model, and more than one peer, it
// returns every peer that serves the model in name order (M203/M206) — the first is
// the primary, the rest are fallbacks used only if an earlier one is unreachable.
// Otherwise it falls back to resolve (sole-peer / ambiguous-name rules).
func (t *Tool) routeCandidates(ctx context.Context, name, model string) ([]Peer, error) {
	peers := t.peersFor(ctx) // the run's effective peer set (tenant override or global)
	if name == "" && model != "" && len(peers) > 1 {
		return t.serversForModel(ctx, peers, model)
	}
	p, err := resolve(peers, name)
	if err != nil {
		return nil, err
	}
	return []Peer{p}, nil
}

// serversForModel returns every peer in the given set that lists the requested model,
// in name order so the choice is deterministic. A peer that can't be reached for
// discovery is noted but doesn't abort the search; if no peer serves the model, the
// error names which peers were checked and which were unreachable.
func (t *Tool) serversForModel(ctx context.Context, peers map[string]Peer, model string) ([]Peer, error) {
	names := make([]string, 0, len(peers))
	for n := range peers {
		names = append(names, n)
	}
	sort.Strings(names)

	var servers []Peer
	var unreachable []string
	for _, n := range names {
		p := peers[n]
		models, err := t.cachedModels(ctx, p)
		if err != nil {
			unreachable = append(unreachable, n)
			continue
		}
		for _, m := range models {
			if m == model {
				servers = append(servers, p)
				break
			}
		}
	}
	if len(servers) == 0 {
		msg := fmt.Sprintf("remote_run: no configured peer serves model %q (checked: %s", model, peerNamesOf(peers))
		if len(unreachable) > 0 {
			msg += "; unreachable: " + strings.Join(unreachable, ", ")
		}
		return nil, fmt.Errorf("%s)", msg)
	}
	return servers, nil
}

// resolve picks the named peer from the given set, or the sole peer when name is empty.
func resolve(peers map[string]Peer, name string) (Peer, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		if len(peers) == 1 {
			for _, p := range peers {
				return p, nil
			}
		}
		return Peer{}, fmt.Errorf("remote_run: a peer name is required (configured: %s)", peerNamesOf(peers))
	}
	p, ok := peers[name]
	if !ok {
		return Peer{}, fmt.Errorf("remote_run: unknown peer %q (configured: %s)", name, peerNamesOf(peers))
	}
	return p, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	// Back up to a UTF-8 rune boundary so a multi-byte rune straddling the cut
	// (common in non-ASCII peer answers) is never split into invalid UTF-8 — the
	// same fix the browser and coding tools already carry. (M468)
	prefix := strutil.Ellipsis(s, max, "")
	return prefix + fmt.Sprintf("\n… [truncated %d bytes]", len(s)-len(prefix))
}

// httpPost is the default poster: a JSON POST with a Bearer token.
