// SPDX-License-Identifier: MIT

package nextcloudtalk

// Nextcloud Talk helpers: parseActivity + randomHex + sha256Sum +
// seenBefore. Carved out of nextcloudtalk.go during the Day 194
// god-file split so the main file can stay focused on types +
// lifecycle + inbound handling + verify/sign and the send file
// can stay focused on outbound Send.
// Public API unchanged.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// nextcloudtalkBody is a Nextcloud Talk activity: the activity type, the
// actor, the object (whose content is a JSON string inside a string) and the
// target.
//
// The shape is NAMED, not anonymous: the contract
// (kernel/contract/channelhookapi) holds the same shape as a JSON Schema, and
// the plugin compares the two when it is built — so a contract that mistyped a
// field stops the daemon instead of quietly disagreeing with the member that
// reads it.
type nextcloudtalkBody struct {
	Type  string `json:"type"`
	Actor struct {
		ID string `json:"id"`
	} `json:"actor"`
	Object struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	} `json:"object"`
	Target struct {
		ID string `json:"id"`
	} `json:"target"`
}

// parseActivity reads an Activity-Streams 2.0 bot event. Only "Create" message
// events are kept; the message text is pulled from object.content's JSON.
//
// The body arrives already DECODED and already MEASURED — the door
// (handleInbound) holds it to the contract's schema before it gets here — so
// this function only reads the fields it needs.
func parseActivity(a nextcloudtalkBody) (inbound, bool) {
	if a.Type != "Create" {
		return inbound{}, false
	}
	// object.content is a JSON-encoded string: {"message":"...","parameters":{...}}.
	text := a.Object.Content
	var content struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(a.Object.Content), &content); err == nil && content.Message != "" {
		text = content.Message
	}
	return inbound{
		token:  a.Target.ID,
		sender: a.Actor.ID,
		text:   strings.TrimSpace(text),
		id:     a.Object.ID,
	}, true
}

// --- helpers --------------------------------------------------------------

func randomHex() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sha256Sum(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

// seenBefore records id and reports whether it was already processed, bounded by
// a ring.
func (c *Channel) seenBefore(id string) bool {
	c.dmu.Lock()
	defer c.dmu.Unlock()
	if _, ok := c.seen[id]; ok {
		return true
	}
	c.seen[id] = struct{}{}
	c.ring = append(c.ring, id)
	if len(c.ring) > dedupCapacity {
		old := c.ring[0]
		c.ring = c.ring[1:]
		delete(c.seen, old)
	}
	return false
}
