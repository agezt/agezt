// SPDX-License-Identifier: MIT
package configcenter

import (
	core "github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/creds"
)

func entryRow(e *core.ConfigEntry) EntryRow {
	value := e.Value
	masked := e.Rating == core.RatingSecret
	if masked {
		value = creds.MaskValue(e.Value)
	}
	return EntryRow{
		Key: e.Key, Value: value, Rating: string(e.Rating), CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt, Version: e.Version,
		Masked: masked, Description: e.Description, Tags: e.Tags, AccessPolicy: string(e.AccessPolicy),
		AllowedAgents: append([]string(nil), e.AllowedAgents...), ExcludedAgents: append([]string(nil), e.ExcludedAgents...),
	}
}
