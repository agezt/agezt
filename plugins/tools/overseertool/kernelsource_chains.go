// SPDX-License-Identifier: MIT

package overseertool

// TaskModelChains persistence for the kernelSource: taskModelChainsSource
// interface + taskModelChain + setTaskModelChain + persistTaskModelChains
// + encodeTaskModelChains + sanitizeTaskModelChain. Carved out of
// kernelsource.go during the Day 164 god-file split so the main file
// can focus on the CRUD / lifecycle / bulk / wake / search / repair /
// help surface.
// Public API unchanged.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/settings"
)
type taskModelChainsSource interface {
	TaskModelChainsView() map[string][]string
	SetTaskModelChains(map[string][]string)
}

func (s *kernelSource) taskModelChain(taskType string) []string {
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		return nil
	}
	gov, ok := s.k.Provider().(taskModelChainsSource)
	if !ok {
		return nil
	}
	chains := gov.TaskModelChainsView()
	src := chains[taskType]
	if len(src) == 0 {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

func (s *kernelSource) setTaskModelChain(taskType string, chain []string) error {
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		return fmt.Errorf("task model chain target task type is empty")
	}
	gov, ok := s.k.Provider().(taskModelChainsSource)
	if !ok {
		return fmt.Errorf("live provider does not support task model chains")
	}
	chains := gov.TaskModelChainsView()
	clean := make([]string, 0, len(chain))
	for _, model := range chain {
		if model = strings.TrimSpace(model); model != "" {
			clean = append(clean, model)
		}
	}
	if len(clean) == 0 {
		return fmt.Errorf("task model chain for %s is empty", taskType)
	}
	chains[taskType] = clean
	gov.SetTaskModelChains(chains)
	if err := persistTaskModelChains(s.baseDir, chains); err != nil {
		return fmt.Errorf("persist task model chains: %w", err)
	}
	return nil
}

func persistTaskModelChains(baseDir string, chains map[string][]string) error {
	store := settings.NewStore(baseDir)
	if err := store.Load(); err != nil {
		return err
	}
	envName := brand.EnvPrefix + "TASK_MODEL_CHAINS"
	if spec := encodeTaskModelChains(chains); spec != "" {
		store.Set(envName, spec)
	} else {
		store.Remove(envName)
	}
	return store.Save()
}

func encodeTaskModelChains(chains map[string][]string) string {
	keys := make([]string, 0, len(chains))
	for task, models := range chains {
		if strings.TrimSpace(task) == "" || len(models) == 0 {
			continue
		}
		keys = append(keys, task)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, task := range keys {
		models := sanitizeTaskModelChain(chains[task])
		if len(models) == 0 {
			continue
		}
		parts = append(parts, task+"="+strings.Join(models, ","))
	}
	return strings.Join(parts, ";")
}

func sanitizeTaskModelChain(models []string) []string {
	out := make([]string, 0, len(models))
	for _, model := range models {
		if model = strings.TrimSpace(model); model != "" {
			out = append(out, model)
		}
	}
	return out
}
