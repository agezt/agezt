// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"fmt"
	"github.com/agezt/agezt/kernel/toolforge"
	"strings"
)

// ForgeReader exposes the selected script-tool store without lifecycle mutation.
type ForgeReader interface {
	List() []toolforge.ScriptTool
	Get(string) (toolforge.ScriptTool, bool)
}
type ForgeCatalog struct{ reader ForgeReader }

func NewForgeCatalog(reader ForgeReader) *ForgeCatalog { return &ForgeCatalog{reader: reader} }

type ForgeListInput struct{}
type ForgeListOutput struct {
	Tools       []ForgeItem `json:"tools"`
	Count       int         `json:"count"`
	ActiveCount int         `json:"active_count"`
}
type ForgeShowInput struct{ Ref string }
type ForgeShowOutput struct {
	Tool ForgeDetail `json:"tool"`
}

// List keeps the store's ordering and omits code bodies from each view.
func (s *ForgeCatalog) List(_ context.Context, _ ForgeListInput) (ForgeListOutput, error) {
	tools := s.reader.List()
	out := make([]ForgeItem, 0, len(tools))
	active := 0
	for _, st := range tools {
		out = append(out, ForgeToolView(st))
		if st.Status == toolforge.StatusActive {
			active++
		}
	}
	return ForgeListOutput{Tools: out, Count: len(out), ActiveCount: active}, nil
}
func (s *ForgeCatalog) Show(_ context.Context, in ForgeShowInput) (ForgeShowOutput, error) {
	st, found := s.reader.Get(strings.TrimSpace(in.Ref))
	if !found {
		return ForgeShowOutput{}, fmt.Errorf("unknown script tool: %s", in.Ref)
	}
	return ForgeShowOutput{Tool: ForgeDetail{ForgeItem: ForgeToolView(st), Code: st.Code}}, nil
}

// ForgeToolView projects lifecycle fields without JSON or float64 conversion.
// It is shared by read operations and the remaining lifecycle adapters.
func ForgeToolView(st toolforge.ScriptTool) ForgeItem {
	row := ForgeItem{ID: st.ID, Name: st.Name, Description: st.Description, Language: st.Language, InputSchema: st.InputSchema, Status: st.Status, TestedOK: st.TestedOK, TestedMS: st.TestedMS, CreatedMS: st.CreatedMS, UpdatedMS: st.UpdatedMS}
	if st.Status == toolforge.StatusActive {
		row.CallableAs = "forge_" + st.Name
	}
	return row
}
