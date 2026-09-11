// SPDX-License-Identifier: MIT

// Memory helpers: recordView + jsonMap + registerMemoryCommands.
// Code extracted from memory.go during the Day-61 god-file split. Public API unchanged.
package controlplane


import (
	"encoding/json"
	"github.com/agezt/agezt/kernel/memory"
	"time"
)


func recordView(r memory.Record) map[string]any {
	v := map[string]any{
		"id":           r.ID,
		"type":         string(r.Type),
		"subject":      r.Subject,
		"content":      r.Content,
		"confidence":   r.Confidence,
		"created_ms":   r.CreatedMS,
		"last_seen_ms": r.LastSeenMS,
	}
	if len(r.Tags) > 0 {
		v["tags"] = r.Tags
	}
	if r.SourceEvent != "" {
		v["source_event"] = r.SourceEvent
	}
	if r.Evidence != "" {
		v["evidence"] = string(r.Evidence)
	}
	if r.HalfLifeMS > 0 {
		v["half_life_ms"] = r.HalfLifeMS
		v["expires_ms"] = r.LastSeenMS + r.HalfLifeMS
		if r.Expired(time.Now().UnixMilli()) {
			v["expired"] = true
		}
	}
	if r.AddedBy != "" {
		v["added_by"] = r.AddedBy
	}
	if r.UpdatedBy != "" {
		v["updated_by"] = r.UpdatedBy
	}
	if r.SupersededBy != "" {
		v["superseded_by"] = r.SupersededBy
	}
	if r.Tombstoned {
		v["tombstoned"] = true
	}
	if r.Suspended() {
		v["suspended_ms"] = r.SuspendedMS
		v["suspended"] = true
		if r.SuspendedReason != "" {
			v["suspended_reason"] = r.SuspendedReason
		}
	}
	return v
}

func jsonMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = json.Unmarshal(b, &out)
	return out, err
}

// registerMemoryCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerMemoryCommands() {
	register(
		commandSpec{Cmd: CmdMemoryAdd, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryAdd(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemorySupersede, Handler: func(dc *DispatchCtx) { dc.S.handleMemorySupersede(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryList, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryGet, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemorySearch, Handler: func(dc *DispatchCtx) { dc.S.handleMemorySearch(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryConsolidate, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryConsolidate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProfileRebuild, Handler: func(dc *DispatchCtx) { dc.S.handleProfileRebuild(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryForget, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryForget(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryPromote, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryPromote(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryPrune, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryPrune(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryTidy, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryTidy(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryBulkForget, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryBulkForget(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryFindRelated, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryFindRelated(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryAudit, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryAudit(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdMemoryClean, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleMemoryClean(dc.Conn, dc.Req) }},
	)
}
