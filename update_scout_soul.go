//go:build ignore
// +build ignore

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: update_scout_soul <soul-text-file>")
		os.Exit(1)
	}

	soulBytes, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read soul file: %v\n", err)
		os.Exit(1)
	}
	soul := string(soulBytes)

	conn, err := grpc.Dial("localhost:9234",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithTimeout(5*time.Second))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: dial: %v\n", brand.CLI, err)
		os.Exit(1)
	}
	defer conn.Close()
	c := controlplane.NewControlPlaneClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get current scout profile
	listRes, err := c.CmdAgentList(ctx, &controlplane.CmdAgentListRequest{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: list agents: %v\n", brand.CLI, err)
		os.Exit(1)
	}

	var scoutID string
	for _, p := range listRes.Profiles {
		if p.Slug == "scout" {
			scoutID = p.Id
			break
		}
	}
	if scoutID == "" {
		fmt.Fprintln(os.Stderr, "scout agent not found")
		os.Exit(1)
	}

	// Fetch full profile
	listRes2, err := c.CmdAgentList(ctx, &controlplane.CmdAgentListRequest{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: list agents: %v\n", brand.CLI, err)
		os.Exit(1)
	}

	var base map[string]any
	for _, p := range listRes2.Profiles {
		if p.Slug == "scout" {
			// Convert protobuf message to map
			base = profileToMap(p)
			break
		}
	}
	if base == nil {
		fmt.Fprintln(os.Stderr, "scout profile not found in list response")
		os.Exit(1)
	}

	base["soul"] = soul

	// Update via CmdAgentEdit
	_, err = c.CmdAgentEdit(ctx, &controlplane.CmdAgentEditRequest{
		Ref:     scoutID,
		Profile: base,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: edit: %v\n", brand.CLI, err)
		os.Exit(1)
	}

	fmt.Println("scout soul updated successfully")
}

func profileToMap(p *controlplane.Profile) map[string]any {
	m := map[string]any{
		"id":            p.Id,
		"slug":          p.Slug,
		"name":          p.Name,
		"soul":          p.Soul,
		"model":         p.Model,
		"task_type":     p.TaskType,
		"description":   p.Description,
		"owner_agent":   p.OwnerAgent,
		"parent_agent":  p.ParentAgent,
		"memory_scope":  p.MemoryScope,
		"workdir":       p.Workdir,
		"trust_ceiling": p.TrustCeiling,
		"kind":          p.Kind,
	}
	if p.MaxCostMc > 0 {
		m["max_cost_mc"] = p.MaxCostMc
	}
	if p.MaxDailyMc > 0 {
		m["max_daily_mc"] = p.MaxDailyMc
	}
	if len(p.Fallbacks) > 0 {
		m["fallbacks"] = p.Fallbacks
	}
	if p.DirectCallable {
		m["direct_callable"] = p.DirectCallable
	}
	if p.Lifecycle != nil {
		m["lifecycle"] = p.Lifecycle
	}
	return m
}
