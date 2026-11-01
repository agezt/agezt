// SPDX-License-Identifier: MIT

package runtime

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
)

// TestTheDaemonReadsTheProfilesModel pins the daemon's half of the delegation
// port: the profile's effective model is the profile's own model, and — when
// it has none — its AGEZT_MODEL config override. The override vocabulary
// belongs to the daemon's configuration, so the daemon reads it; the
// delegation engine asks for the answer through one door.
func TestTheDaemonReadsTheProfilesModel(t *testing.T) {
	h := delegationHost{}
	cases := []struct {
		name string
		prof agentapi.Profile
		want string
	}{
		{
			name: "the profile's own model wins",
			prof: agentapi.Profile{Slug: "a", Model: "profile-model",
				ConfigOverrides: map[string]string{"AGEZT_MODEL": "override-model"}},
			want: "profile-model",
		},
		{
			name: "the override answers when the profile has no model",
			prof: agentapi.Profile{Slug: "b",
				ConfigOverrides: map[string]string{"AGEZT_MODEL": "override-model"}},
			want: "override-model",
		},
		{
			name: "no model and no override",
			prof: agentapi.Profile{Slug: "c"},
			want: "",
		},
		{
			name: "an empty override answers nothing",
			prof: agentapi.Profile{Slug: "d", ConfigOverrides: map[string]string{"AGEZT_MODEL": "  "}},
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := h.ProfileModel(c.prof); got != c.want {
				t.Errorf("ProfileModel = %q, want %q", got, c.want)
			}
		})
	}
}

// TestTheDaemonCarriesTheLiveDelegationPolicy pins the other half of the
// port: the engine reads the daemon's LIVE caps, so an operator's runtime
// change to a bound takes effect on the next delegation.
func TestTheDaemonCarriesTheLiveDelegationPolicy(t *testing.T) {
	cfg := delegationConfig(Config{
		SubAgentMaxDepth:           3,
		SubAgentMaxFanout:          4,
		SubAgentMaxTotal:           5,
		SubAgentMaxSpendMicrocents: 6,
	})
	if cfg.MaxDepth != 3 || cfg.MaxFanout != 4 || cfg.MaxTotal != 5 || cfg.MaxSpendMicrocents != 6 {
		t.Fatalf("the policy did not cross the port: %+v", cfg)
	}
}
