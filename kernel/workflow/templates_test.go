// SPDX-License-Identifier: MIT

package workflow

import "testing"

// TestTemplatesValidate pins every gallery entry to the SAME Validate the
// save path uses — a schema change that breaks a template breaks the build,
// not the user's first click.
func TestTemplatesValidate(t *testing.T) {
	all := Templates()
	if len(all) < 5 {
		t.Fatalf("gallery shrank to %d entries", len(all))
	}
	seen := map[string]bool{}
	for _, tpl := range all {
		if tpl.Name == "" || tpl.Title == "" || tpl.Description == "" || tpl.Category == "" {
			t.Fatalf("template %q missing metadata: %+v", tpl.Name, tpl)
		}
		if seen[tpl.Name] {
			t.Fatalf("duplicate template slug %q", tpl.Name)
		}
		seen[tpl.Name] = true
		if err := Validate(tpl.Workflow); err != nil {
			t.Fatalf("template %q does not validate: %v", tpl.Name, err)
		}
		// Every node is positioned — the gallery must open laid-out.
		for _, n := range tpl.Workflow.Nodes {
			if n.X == 0 && n.Y == 0 {
				t.Fatalf("template %q node %s is unpositioned", tpl.Name, n.ID)
			}
		}
	}
}

func TestTemplateByName(t *testing.T) {
	tpl, ok := TemplateByName("resilient-fetch")
	if !ok || tpl.Title == "" {
		t.Fatalf("TemplateByName: %v %v", tpl, ok)
	}
	if _, ok := TemplateByName("ghost"); ok {
		t.Fatal("ghost template found")
	}
}
