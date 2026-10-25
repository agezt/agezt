// SPDX-License-Identifier: MIT

package skillapi

// Provenance: the skill rows that the retrieval and the run-time layers share:
//             Scored (a skill paired with its retrieval score),
//             ActivationDirective (an explicit skill request parsed out of an
//             intent) and HygieneReport (the operator's cleanup view). Code
//             moved from the skills module (kernel/modules/skills/skill) during
//             the W3.55 slice, so the layers above the module can name the rows
//             without naming the module.

import "errors"

// Scored is a skill paired with its retrieval score.
type Scored struct {
	Skill Skill   `json:"skill"`
	Score float64 `json:"score"`
}

// ActivationDirective is an explicit skill request embedded at the top of an
// intent. It is intentionally tiny and deterministic: runtime can strip the
// control line from the user-facing task while Forge receives the requested
// skill names/ids for activation.
type ActivationDirective struct {
	Explicit    bool
	Refs        []string
	CleanIntent string
}

// HygieneReport summarizes skill health for the cleanup view (M858).
type HygieneReport struct {
	Total  int     `json:"total"`
	Active int     `json:"active"`
	Idle   []Skill `json:"idle"` // active skills that look unused (see Hygiene)
}

// ErrIllegalTransition is returned when a lifecycle edge isn't allowed.
var ErrIllegalTransition = errors.New("skill: illegal lifecycle transition")

// ErrNotFound is returned by operations on an unknown skill id.
var ErrNotFound = errors.New("skill: not found")

// ErrEmptyBody is returned when a skill is written with no body.
var ErrEmptyBody = errors.New("skill: empty body")
