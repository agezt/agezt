// SPDX-License-Identifier: MIT

package discord

import "testing"

// TestValidDiscordAttachmentURL guards the H-001 attachment-fetch host policy:
// only https Discord-CDN URLs may be fetched server-side; anything else (other
// hosts, http, internal/SSRF targets, malformed) is refused before the dial.
func TestValidDiscordAttachmentURL(t *testing.T) {
	good := []string{
		"https://cdn.discordapp.com/attachments/1/2/photo.png",
		"https://media.discordapp.net/attachments/1/2/clip.mp3",
		"https://images-ext-1.discordapp.net/external/abc/photo.png",
		"https://anything.discordapp.com/x",
	}
	for _, u := range good {
		if err := validDiscordAttachmentURL(u); err != nil {
			t.Errorf("expected %q to be allowed, got %v", u, err)
		}
	}

	bad := []string{
		"http://cdn.discordapp.com/x",               // not https
		"https://cdn.discordapp.com.evil.com/x",     // suffix-spoof host
		"https://evildiscordapp.com/x",              // no sub-domain boundary
		"https://example.com/x",                     // foreign host
		"https://169.254.169.254/latest/meta-data/", // cloud metadata SSRF
		"https://localhost/x",                       // loopback SSRF
		"http://[::1]/x",                            // ipv6 loopback
		"file:///etc/passwd",                        // non-http scheme
		"https:// bad host/x",                       // malformed
		"",                                          // empty
	}
	for _, u := range bad {
		if err := validDiscordAttachmentURL(u); err == nil {
			t.Errorf("expected %q to be rejected, got nil error", u)
		}
	}
}
