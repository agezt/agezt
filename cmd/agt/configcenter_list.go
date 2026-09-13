// SPDX-License-Identifier: MIT

package main

// agt configcenter LIST subcommand: cmdConfigCenterList. Carved out
// of configcenter.go during the Day 194 god-file split so the main
// file can stay focused on the dispatcher + Help + Get + Delete
// and the set file can stay focused on the set subcommand.
// Public API unchanged.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
)

func cmdConfigCenterList(args []string, stdout, stderr io.Writer) int {
	var rating string
	asJSON := false

	for _, a := range args {
		switch a {
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s configcenter list [--rating <rating>] [--json]\n", brand.CLI)
			return 0
		case "--rating":
			// Will be processed below
		case "--json":
			asJSON = true
		default:
			if rating == "" && !strings.HasPrefix(a, "--rating") {
				rating = a
			} else {
				fmt.Fprintf(stderr, "%s configcenter list: unexpected arg %q\n", brand.CLI, a)
				return 2
			}
		}
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	params := map[string]any{}
	if rating != "" {
		params["rating"] = rating
	}

	res, err := c.Call(ctx, controlplane.CmdConfigCenterList, params)
	if err != nil {
		fmt.Fprintf(stderr, "%s configcenter list: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}

	entries, _ := res["entries"].([]any)
	fmt.Fprintf(stdout, "%s configcenter: %d entries\n", brand.CLI, len(entries))

	// Sort by rating (secret first, then restricted, internal, public)
	type entryInfo struct {
		key       string
		rating    string
		value     string
		updatedAt int64
	}
	byRating := make(map[configcenter.Rating][]entryInfo)

	for _, e := range entries {
		em := e.(map[string]any)
		ei := entryInfo{
			key:       em["key"].(string),
			rating:    em["rating"].(string),
			value:     em["value"].(string),
			updatedAt: int64(em["updated_at"].(float64)),
		}
		byRating[configcenter.Rating(em["rating"].(string))] = append(byRating[configcenter.Rating(em["rating"].(string))], ei)
	}

	// Print by rating order
	for _, r := range []configcenter.Rating{configcenter.RatingSecret, configcenter.RatingRestricted, configcenter.RatingInternal, configcenter.RatingPublic} {
		entries := byRating[r]
		if len(entries) == 0 {
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })

		ratingLabel := string(r)
		if r == configcenter.RatingSecret {
			fmt.Fprintf(stdout, "\n🔴 %s (%d):\n", ratingLabel, len(entries))
		} else if r == configcenter.RatingRestricted {
			fmt.Fprintf(stdout, "\n🟡 %s (%d):\n", ratingLabel, len(entries))
		} else if r == configcenter.RatingInternal {
			fmt.Fprintf(stdout, "\n🔵 %s (%d):\n", ratingLabel, len(entries))
		} else {
			fmt.Fprintf(stdout, "\n🟢 %s (%d):\n", ratingLabel, len(entries))
		}

		for _, e := range entries {
			val := e.value
			if len(val) > 40 {
				val = val[:40] + "..."
			}
			if r == configcenter.RatingSecret {
				val = "********"
			}
			fmt.Fprintf(stdout, "  %-40s = %s\n", e.key, val)
		}
	}

	return 0
}

