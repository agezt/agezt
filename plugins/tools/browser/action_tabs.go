// SPDX-License-Identifier: MIT
//
// browser.action tab/session state: actionTabState + actionSnapshotRef types
// + resolveTabURL + ResolveTabRef + readTabState + finalizeTabOutput +
// saveTabState + extractSnapshotRefs + tabStatePath + validBrowserSessionID +
// CloseSession + CloseTab + ListTabs + URL/host validators (validateCDPURL,
// validateURL, validateHostEgress). Split from action.go during Day 211
// god-file refactor (#38). Public API unchanged.
package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/netguard"
)

type actionTabState struct {
	URL       string              `json:"url"`
	UpdatedMS int64               `json:"updated_ms"`
	Snapshot  []actionSnapshotRef `json:"snapshot,omitempty"`
}

type actionSnapshotRef struct {
	Ref      string `json:"ref"`
	Selector string `json:"selector"`
	Role     string `json:"role,omitempty"`
	Name     string `json:"name,omitempty"`
}

func (t *ActionTool) resolveTabURL(in *actionInput) error {
	if strings.TrimSpace(in.TabID) == "" {
		return nil
	}
	if in.Profile != actionProfileSession {
		return errors.New("tab_id requires profile=session and session_id")
	}
	if strings.TrimSpace(in.URL) != "" {
		return nil
	}
	st, err := t.readTabState(in.SessionID, in.TabID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(st.URL) == "" {
		return fmt.Errorf("tab_id %q has empty saved URL; call browser.open with url to refresh it", strings.TrimSpace(in.TabID))
	}
	in.URL = strings.TrimSpace(st.URL)
	return nil
}

func (t *ActionTool) ResolveTabRef(sessionID, tabID, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.New("ref required")
	}
	st, err := t.readTabState(sessionID, tabID)
	if err != nil {
		return "", err
	}
	for _, item := range st.Snapshot {
		if item.Ref == ref && strings.TrimSpace(item.Selector) != "" {
			return item.Selector, nil
		}
	}
	return "", fmt.Errorf("ref %q not found for tab_id %q in session_id %q; call browser.snapshot with the same session_id/tab_id and use a fresh ref", ref, strings.TrimSpace(tabID), strings.TrimSpace(sessionID))
}

func (t *ActionTool) readTabState(sessionID, tabID string) (actionTabState, error) {
	p, err := t.tabStatePath(sessionID, tabID)
	if err != nil {
		return actionTabState{}, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return actionTabState{}, fmt.Errorf("tab_id %q has no saved URL in session_id %q; call browser.open with url, profile=session, session_id, and tab_id first", strings.TrimSpace(tabID), strings.TrimSpace(sessionID))
	}
	if err != nil {
		return actionTabState{}, fmt.Errorf("read browser tab state %q: %w", strings.TrimSpace(tabID), err)
	}
	var st actionTabState
	if err := json.Unmarshal(data, &st); err != nil {
		return actionTabState{}, fmt.Errorf("read browser tab state %q: %w", strings.TrimSpace(tabID), err)
	}
	return st, nil
}

func (t *ActionTool) finalizeTabOutput(normalized string, in actionInput, finalURL string) string {
	tabID := strings.TrimSpace(in.TabID)
	if tabID == "" {
		return normalized
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(normalized), &out); err != nil {
		return normalized
	}
	if strings.TrimSpace(finalURL) == "" {
		finalURL = strings.TrimSpace(in.URL)
	}
	out["session_id"] = strings.TrimSpace(in.SessionID)
	out["tab_id"] = tabID
	out["tab_ref"] = map[string]any{
		"session_id": strings.TrimSpace(in.SessionID),
		"tab_id":     tabID,
		"url":        finalURL,
		"live":       false,
		"refs":       len(extractSnapshotRefs(out)),
	}
	if err := t.saveTabState(in.SessionID, tabID, finalURL, extractSnapshotRefs(out)); err != nil {
		out["tab_state_error"] = err.Error()
	}
	enc, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return normalized
	}
	return string(enc)
}

func (t *ActionTool) saveTabState(sessionID, tabID, finalURL string, refs []actionSnapshotRef) error {
	finalURL = strings.TrimSpace(finalURL)
	if finalURL == "" {
		return errors.New("empty final URL")
	}
	u, err := url.Parse(finalURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("invalid final URL for tab state: %q", finalURL)
	}
	p, err := t.tabStatePath(sessionID, tabID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("create browser tab state dir: %w", err)
	}
	now := time.Now().UnixMilli()
	if t.Now != nil {
		now = t.Now()
	}
	data, err := json.MarshalIndent(actionTabState{URL: finalURL, UpdatedMS: now, Snapshot: refs}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write browser tab state %q: %w", strings.TrimSpace(tabID), err)
	}
	return nil
}

func extractSnapshotRefs(out map[string]any) []actionSnapshotRef {
	rows, _ := out["snapshot"].([]any)
	if len(rows) == 0 {
		return nil
	}
	refs := make([]actionSnapshotRef, 0, len(rows))
	for _, item := range rows {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		ref, _ := row["ref"].(string)
		selector, _ := row["selector"].(string)
		ref = strings.TrimSpace(ref)
		selector = strings.TrimSpace(selector)
		if ref == "" || selector == "" {
			continue
		}
		role, _ := row["role"].(string)
		name, _ := row["name"].(string)
		refs = append(refs, actionSnapshotRef{Ref: ref, Selector: selector, Role: strings.TrimSpace(role), Name: strings.TrimSpace(name)})
	}
	return refs
}

func (t *ActionTool) tabStatePath(sessionID, tabID string) (string, error) {
	tabID = strings.TrimSpace(tabID)
	if !validBrowserSessionID(tabID) {
		return "", errors.New("tab_id must be 1-80 chars using letters, numbers, dot, underscore, or dash; it cannot start with dot")
	}
	dir, err := t.sessionDir(sessionID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".agezt-tabs", tabID+".json"), nil
}

func validBrowserSessionID(id string) bool {
	if id == "" || len(id) > 80 || strings.HasPrefix(id, ".") {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func (t *ActionTool) CloseSession(id string) (string, error) {
	id = strings.TrimSpace(id)
	dir, err := t.sessionDir(id)
	if err != nil {
		return "", err
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("close browser session %q: %w", id, err)
	}
	out, err := json.MarshalIndent(map[string]any{
		"closed":     true,
		"session_id": id,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (t *ActionTool) CloseTab(sessionID, tabID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	tabID = strings.TrimSpace(tabID)
	p, err := t.tabStatePath(sessionID, tabID)
	if err != nil {
		return "", err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("close browser tab %q in session %q: %w", tabID, sessionID, err)
	}
	out, err := json.MarshalIndent(map[string]any{
		"closed":     true,
		"session_id": sessionID,
		"tab_id":     tabID,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (t *ActionTool) ListTabs(sessionID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	dir, err := t.sessionDir(sessionID)
	if err != nil {
		return "", err
	}
	tabDir := filepath.Join(dir, ".agezt-tabs")
	entries, err := os.ReadDir(tabDir)
	if errors.Is(err, os.ErrNotExist) {
		entries = nil
	} else if err != nil {
		return "", fmt.Errorf("list browser tabs for session %q: %w", sessionID, err)
	}
	tabs := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		tabID := strings.TrimSuffix(entry.Name(), ".json")
		if !validBrowserSessionID(tabID) {
			continue
		}
		st, err := t.readTabState(sessionID, tabID)
		if err != nil {
			tabs = append(tabs, map[string]any{
				"tab_id": tabID,
				"error":  err.Error(),
				"live":   false,
			})
			continue
		}
		tabs = append(tabs, map[string]any{
			"tab_id":     tabID,
			"url":        st.URL,
			"updated_ms": st.UpdatedMS,
			"refs":       len(st.Snapshot),
			"live":       false,
		})
	}
	out, err := json.MarshalIndent(map[string]any{
		"session_id": sessionID,
		"tabs":       tabs,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (t *ActionTool) validateCDPURL(ctx context.Context, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid remote-cdp url: %w", err)
	}
	switch u.Scheme {
	case "http", "https", "ws", "wss":
	default:
		return fmt.Errorf("remote-cdp url scheme %q not allowed (only http/https/ws/wss)", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("remote-cdp url missing host")
	}
	if err := t.validateHostEgress(ctx, u.Hostname()); err != nil {
		return fmt.Errorf("remote-cdp %w", err)
	}
	return nil
}

func (t *ActionTool) validateURL(ctx context.Context, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url scheme %q not allowed (only http/https)", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("url missing host")
	}
	if !t.AllowAll && !hostAllowed(u.Host, t.AllowedHosts) {
		return fmt.Errorf("%w: %s", ErrHostDenied, u.Hostname())
	}
	return t.validateHostEgress(ctx, u.Hostname())
}

func (t *ActionTool) validateHostEgress(ctx context.Context, host string) error {
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		lookup := t.lookupIP
		if lookup == nil {
			lookup = net.DefaultResolver.LookupIP
		}
		var err error
		ips, err = lookup(ctx, "ip", host)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", host, err)
		}
		if len(ips) == 0 {
			return fmt.Errorf("resolve %s: no addresses", host)
		}
	}
	var opts []netguard.Option
	if t.AllowLoopback {
		opts = append(opts, netguard.AllowLoopback())
	}
	if t.AllowPrivate {
		opts = append(opts, netguard.AllowPrivate())
	}
	if t.OnBlock != nil {
		opts = append(opts, netguard.OnBlock(t.OnBlock))
	}
	g := netguard.New(opts...)
	for _, ip := range ips {
		if ok, reason := g.Allowed(ip); !ok {
			return fmt.Errorf("egress blocked: %s resolves to %s (%s)", host, ip, reason)
		}
	}
	return nil
}
