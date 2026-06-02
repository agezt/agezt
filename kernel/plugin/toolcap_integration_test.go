// SPDX-License-Identifier: MIT

package plugin_test

// Live proof for M182: a plugin advertising more tools than
// Config.MaxAdvertisedTools fails to spawn. The echo fixture advertises
// 4 tools, so a cap of 2 must reject it with ErrTooManyTools.

import (
	"context"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/plugin"
)

func TestSpawn_RejectsTooManyAdvertisedTools(t *testing.T) {
	bin := buildEchoPlugin(t)
	p, err := plugin.Spawn(context.Background(), plugin.Config{
		Path:               bin,
		MaxAdvertisedTools: 2, // echo advertises 4 (echo, fail, slowwork, callhost)
	})
	if err == nil {
		p.Close()
		t.Fatal("Spawn succeeded despite over-cap tool count")
	}
	if !errors.Is(err, plugin.ErrTooManyTools) {
		t.Errorf("Spawn err = %v; want ErrTooManyTools", err)
	}
}
