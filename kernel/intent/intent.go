// SPDX-License-Identifier: MIT

// Package intent turns a user's utterance into auditable intent metadata before
// runtime policy decides whether a proposed action may execute.
package intent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
)

// CandidatePlan is one plausible interpretation of the user's utterance.
type CandidatePlan struct {
	Label       string  `json:"label"`
	Description string  `json:"description"`
	Risk        float64 `json:"risk"`
}

// RegretAxes classifies how costly a wrong interpretation would be.
type RegretAxes struct {
	Physical      float64 `json:"physical"`
	Informational float64 `json:"informational"`
	Social        float64 `json:"social"`
	Identity      float64 `json:"identity"`
}

// Max returns the strongest regret axis.
func (r RegretAxes) Max() float64 {
	m := r.Physical
	if r.Informational > m {
		m = r.Informational
	}
	if r.Social > m {
		m = r.Social
	}
	if r.Identity > m {
		m = r.Identity
	}
	return m
}

// Sum returns total regret pressure across axes.
func (r RegretAxes) Sum() float64 {
	return r.Physical + r.Informational + r.Social + r.Identity
}

// Frame is the interpreter output. It is safe to journal because the raw user
// utterance is represented by a hash rather than copied into policy metadata.
type Frame struct {
	UserUtteranceHash  string          `json:"user_utterance_hash"`
	CanonicalIntent    string          `json:"canonical_intent"`
	Assumptions        []string        `json:"assumptions,omitempty"`
	ExplicitExclusions []string        `json:"explicit_exclusions,omitempty"`
	CandidatePlans     []CandidatePlan `json:"candidate_plans,omitempty"`
	HarmfulReading     string          `json:"harmful_reading,omitempty"`
	AmbiguityScore     float64         `json:"ambiguity_score"`
	Underdetermined    bool            `json:"underdetermined"`
}

// Action is the policy-time action proposal tested against the interpreted
// intent.
type Action struct {
	ToolName          string
	Capability        string
	EffectClass       string
	Input             string
	AffectedResources []string
}

// Interpret builds a compact, deterministic intent frame. It is intentionally
// conservative: unclear mutation requests become underdetermined, while
// read/list/search requests stay low-friction.
func Interpret(utterance string) Frame {
	canonical := compactWhitespace(utterance)
	lower := strings.ToLower(canonical)
	tokens := tokenSet(lower)
	mutating := hasAny(tokens, "clean", "clear", "delete", "remove", "wipe", "purge", "reset", "destroy", "format", "sil", "temizle", "kaldir", "kaldır", "sifirla", "sıfırla")
	readOnly := hasAny(tokens, "list", "show", "read", "search", "find", "inspect", "listele", "goster", "göster", "oku", "ara", "bul")
	broadScope := hasAny(tokens, "all", "everything", "files", "data", "workspace", "project", "repo", "legacy", "old", "tum", "tüm", "hepsi", "hersey", "herşey", "dosya", "dosyalar", "veri", "proje", "eski")
	specificSafeScope := hasAny(tokens, "cache", "temp", "tmp", "temporary", "logs", "log", "dry-run", "dryrun", "gecici", "geçici", "onizle", "önizle")

	score := 0.1
	assumptions := []string(nil)
	candidates := []CandidatePlan{{Label: "literal", Description: "Execute the user's request as written.", Risk: 0.2}}
	harmful := ""

	switch {
	case mutating && broadScope && !specificSafeScope:
		score = 0.85
		assumptions = append(assumptions, "The requested mutation has a broad or underspecified target.")
		candidates = append(candidates,
			CandidatePlan{Label: "safe_probe", Description: "List or dry-run the candidate resources first.", Risk: 0.1},
			CandidatePlan{Label: "broad_mutation", Description: "Mutate every matching resource in the broad scope.", Risk: 0.95},
		)
		harmful = "The request could be read as permission to mutate a broad set of files or state."
	case mutating && !specificSafeScope:
		score = 0.6
		assumptions = append(assumptions, "The requested mutation does not name an explicit safe scope.")
		candidates = append(candidates,
			CandidatePlan{Label: "targeted_mutation", Description: "Mutate only the target implied by immediate context.", Risk: 0.45},
			CandidatePlan{Label: "wrong_target", Description: "Mutate a target the user did not intend.", Risk: 0.8},
		)
		harmful = "The action could affect the wrong target if the implied scope is mistaken."
	case readOnly:
		score = 0.15
	default:
		score = 0.3
	}

	return Frame{
		UserUtteranceHash: hashUtterance(canonical),
		CanonicalIntent:   canonical,
		Assumptions:       assumptions,
		CandidatePlans:    candidates,
		HarmfulReading:    harmful,
		AmbiguityScore:    score,
		Underdetermined:   score >= 0.6,
	}
}

// RegretForAction estimates action-class regret independently of the model.
func RegretForAction(a Action) RegretAxes {
	capability := strings.ToLower(a.Capability)
	tool := strings.ToLower(a.ToolName)
	effect := strings.ToLower(a.EffectClass)
	input := strings.ToLower(a.Input)
	joinedResources := strings.ToLower(strings.Join(a.AffectedResources, " "))

	var r RegretAxes
	switch effect {
	case "read_only":
		r.Informational = 0.1
	case "reversible":
		r.Informational = 0.35
	case "compensable":
		r.Informational = 0.45
		r.Social = 0.25
	case "irreversible":
		r.Informational = 0.75
	default:
		r.Informational = 0.55
	}

	haystack := strings.Join([]string{capability, tool, input, joinedResources}, " ")
	if containsAny(haystack, "file.delete", "delete", "remove", "rm ", "unlink", "wipe", "purge", "sil", "temizle") {
		r.Informational = max(r.Informational, 0.9)
	}
	if containsAny(haystack, "shell", "code.exec", "codeexec", "remote_run", "command", "powershell", "bash", "cmd") {
		r.Physical = max(r.Physical, 0.65)
		r.Informational = max(r.Informational, 0.6)
	}
	if containsAny(haystack, "notify", "http.post", "email", "message", "slack", "telegram", "boss", "publish", "post") {
		r.Social = max(r.Social, 0.8)
	}
	if containsAny(haystack, "auth", "account", "identity", "profile", "config.write", "token", "key") {
		r.Identity = max(r.Identity, 0.65)
	}
	return r
}

// RequiresConfirmation returns true when intent is underdetermined and the
// proposed action has enough regret pressure to justify targeted HITL.
func RequiresConfirmation(frame Frame, axes RegretAxes) bool {
	return frame.Underdetermined && frame.AmbiguityScore >= 0.6 && (axes.Max() >= 0.75 || axes.Sum() >= 1.0)
}

// ConfirmationPrompt compresses the harmful interpretation into a prompt the
// operator can answer quickly.
func ConfirmationPrompt(frame Frame, action Action, axes RegretAxes) string {
	resource := "the proposed resources"
	if len(action.AffectedResources) > 0 {
		resource = action.AffectedResources[0]
	}
	harmful := frame.HarmfulReading
	if harmful == "" {
		harmful = "The action may not match the user's intended scope."
	}
	return fmt.Sprintf("I interpreted the request as permission to run %s on %s. If that reading is wrong, regret risk is %.2f. %s Confirm this exact scope?", action.ToolName, resource, axes.Max(), harmful)
}

func hashUtterance(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func compactWhitespace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func tokenSet(s string) map[string]struct{} {
	out := map[string]struct{}{}
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out[b.String()] = struct{}{}
		b.Reset()
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func hasAny(tokens map[string]struct{}, words ...string) bool {
	for _, word := range words {
		if _, ok := tokens[word]; ok {
			return true
		}
	}
	return false
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
