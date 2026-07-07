// SPDX-License-Identifier: MIT

package skilltool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/skill"
)

// covForge is a test-only forge fake. The existing skilltool_test.go already
// declares a fakeForge — we use a distinct name to avoid duplicate symbols.
type covForge struct {
	skills     []skill.Skill
	createErr  error
	createOK   bool
	listErr    error
	getErr     error
	gotGet     string
	gotPromote string
	promoteTo  skill.Status
	promoteErr error
	quarantine string
	quarErr    error
	bundles    *skill.BundleStore
}

func (f *covForge) Create(_ string, spec skill.CreateSpec) (skill.Skill, bool, error) {
	if f.createErr != nil {
		return skill.Skill{}, false, f.createErr
	}
	if !f.createOK {
		return skill.Skill{ID: "sk-existing", Name: spec.Name, Status: skill.StatusDraft, Body: spec.Body}, false, nil
	}
	sk := skill.Skill{ID: "sk-new", Name: spec.Name, Status: skill.StatusDraft, Body: spec.Body, Triggers: spec.Triggers, Description: spec.Description, Version: "1.0.0", Metrics: skill.Metrics{}}
	f.skills = append(f.skills, sk)
	return sk, true, nil
}

func (f *covForge) Promote(_ string, id string) (skill.Status, error) {
	f.gotPromote = id
	if f.promoteErr != nil {
		return "", f.promoteErr
	}
	return f.promoteTo, nil
}

func (f *covForge) Quarantine(_ string, id string, _ string) error {
	f.quarantine = id
	return f.quarErr
}

func (f *covForge) Get(id string) (skill.Skill, bool, error) {
	f.gotGet = id
	if f.getErr != nil {
		return skill.Skill{}, false, f.getErr
	}
	for _, s := range f.skills {
		if s.ID == id {
			return s, true, nil
		}
	}
	return skill.Skill{}, false, nil
}

func (f *covForge) List() ([]skill.Skill, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.skills, nil
}

func (f *covForge) Bundles() *skill.BundleStore { return f.bundles }

func TestSkilltoolCoverageBindCurrentShortID(t *testing.T) {
	tool := New()
	if tool.current() != nil {
		t.Fatal("New should leave forge nil")
	}
	tool.Bind(nil)
	if tool.current() != nil {
		t.Fatal("Bind(nil) should not set forge")
	}
	f := &covForge{}
	tool.Bind(f)
	if tool.current() == nil {
		t.Fatal("Bind(covForge) should set forge")
	}

	if got := shortID(""); got != "" {
		t.Fatalf("shortID empty = %q", got)
	}
	if got := shortID("abc"); got != "abc" {
		t.Fatalf("shortID short = %q", got)
	}
	if got := shortID("abcdefghijklmnopqrst"); len(got) != 12 {
		t.Fatalf("shortID long = %q, want 12 chars", got)
	}
}

func TestSkilltoolCoverageSkillView(t *testing.T) {
	sk := skill.Skill{ID: "sk-1", Name: "name", Status: skill.StatusActive, Version: "3.0.0", Metrics: skill.Metrics{Uses: 5}}
	v := skillView(sk)
	if v["id"] != "sk-1" {
		t.Fatalf("id = %v", v["id"])
	}
	if v["name"] != "name" {
		t.Fatalf("name = %v", v["name"])
	}
	if v["status"] != "active" {
		t.Fatalf("status = %v", v["status"])
	}
	if v["version"] != "3.0.0" {
		t.Fatalf("version = %v", v["version"])
	}
	if v["uses"] != 5 {
		t.Fatalf("uses = %v", v["uses"])
	}
	if _, ok := v["description"]; ok {
		t.Fatal("empty description should be omitted")
	}
	if _, ok := v["triggers"]; ok {
		t.Fatal("empty triggers should be omitted")
	}

	sk.Description = "desc"
	sk.Triggers = []string{"a", "b"}
	v = skillView(sk)
	if v["description"] != "desc" {
		t.Fatalf("description = %v", v["description"])
	}
	if got, _ := v["triggers"].([]string); len(got) != 2 {
		t.Fatalf("triggers = %v", v["triggers"])
	}
}

func TestSkilltoolCoverageInvokeValidation(t *testing.T) {
	tool := New()
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke unbound: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("unbound = %+v", res)
	}

	tool.Bind(&covForge{})
	_, err = New().Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}

	res, err = tool.Invoke(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke empty: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "op required") {
		t.Fatalf("empty op = %+v", res)
	}

	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"wat"}`))
	if err != nil {
		t.Fatalf("Invoke unknown: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "unknown op") {
		t.Fatalf("unknown op = %+v", res)
	}
}

func TestSkilltoolCoverageLearnBranches(t *testing.T) {
	f := &covForge{}
	tool := New()
	tool.Bind(f)

	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"op":"learn","body":"x"}`))
	if !res.IsError || !strings.Contains(res.Output, `"name"`) {
		t.Fatalf("no name = %+v", res)
	}

	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"learn","name":"n"}`))
	if !res.IsError || !strings.Contains(res.Output, `"body"`) {
		t.Fatalf("no body = %+v", res)
	}

	f.createOK = true
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"learn","name":"new","body":"steps","description":"d","triggers":["a"],"tools":["t"]}`))
	if res.IsError {
		t.Fatalf("create success = %+v", res)
	}
	if !strings.Contains(res.Output, `"learned a new skill`) {
		t.Fatalf("message = %s", res.Output)
	}
	if len(f.skills) != 1 || f.skills[0].Name != "new" {
		t.Fatalf("fakeForge.skills = %+v", f.skills)
	}

	f.createOK = false
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"learn","name":"n","body":"x"}`))
	if res.IsError {
		t.Fatalf("refresh = %+v", res)
	}
	if !strings.Contains(res.Output, "you already knew this skill") {
		t.Fatalf("refresh message = %s", res.Output)
	}

	f.createErr = errors.New("disk full")
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"learn","name":"n","body":"x"}`))
	if !res.IsError || !strings.Contains(res.Output, "disk full") {
		t.Fatalf("create error = %+v", res)
	}
}

func TestSkilltoolCoverageResolveAndShow(t *testing.T) {
	f := &covForge{skills: []skill.Skill{
		{ID: "abc-1", Name: "first"},
		{ID: "abc-2", Name: "second"},
		{ID: "xyz-1", Name: "third"},
	}}
	tool := New()
	tool.Bind(f)

	sk, err := tool.resolve(f, "abc-1")
	if err != nil || sk.Name != "first" {
		t.Fatalf("exact resolve = %+v err %v", sk, err)
	}

	sk, err = tool.resolve(f, "xyz")
	if err != nil || sk.Name != "third" {
		t.Fatalf("prefix resolve = %+v err %v", sk, err)
	}

	_, err = tool.resolve(f, "abc")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous resolve = %v", err)
	}

	_, err = tool.resolve(f, "missing")
	if err == nil || !strings.Contains(err.Error(), "no skill with id") {
		t.Fatalf("missing resolve = %v", err)
	}

	_, err = tool.resolve(f, "")
	if err == nil || !strings.Contains(err.Error(), "id is required") {
		t.Fatalf("empty id = %v", err)
	}

	f.getErr = errors.New("io error")
	_, err = tool.resolve(f, "abc-1")
	if err == nil {
		t.Fatal("get error should propagate")
	}
	f.getErr = nil

	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"op":"show","id":"abc-1"}`))
	if res.IsError {
		t.Fatalf("show = %+v", res)
	}
	if !strings.Contains(res.Output, `"name": "first"`) || !strings.Contains(res.Output, `"body"`) {
		t.Fatalf("show output = %s", res.Output)
	}
	if !strings.Contains(res.Output, agent.DefaultContextRescueMarker) {
		t.Fatalf("show missing rescue marker: %s", res.Output)
	}
}

func TestSkilltoolCoveragePromoteRetire(t *testing.T) {
	f := &covForge{skills: []skill.Skill{{ID: "sk-1", Name: "n"}}, promoteTo: skill.StatusShadow}
	tool := New()
	tool.Bind(f)

	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"op":"promote","id":"sk-1"}`))
	if res.IsError {
		t.Fatalf("promote = %+v", res)
	}
	if !strings.Contains(res.Output, `"status": "shadow"`) {
		t.Fatalf("promote output = %s", res.Output)
	}

	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"retire","id":"sk-1","reason":"misbehavior"}`))
	if res.IsError {
		t.Fatalf("retire = %+v", res)
	}
	if f.quarantine != "sk-1" {
		t.Fatalf("quarantine id = %q", f.quarantine)
	}

	f.promoteErr = errors.New("forbidden")
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"promote","id":"sk-1"}`))
	if !res.IsError || !strings.Contains(res.Output, "forbidden") {
		t.Fatalf("promote err = %+v", res)
	}
	f.promoteErr = nil

	f.quarErr = errors.New("write failed")
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"retire","id":"sk-1"}`))
	if !res.IsError || !strings.Contains(res.Output, "write failed") {
		t.Fatalf("retire err = %+v", res)
	}
}

func TestSkilltoolCoverageListError(t *testing.T) {
	f := &covForge{listErr: errors.New("nope")}
	tool := New()
	tool.Bind(f)
	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if !res.IsError || !strings.Contains(res.Output, "nope") {
		t.Fatalf("list err = %+v", res)
	}
}

func TestSkilltoolCoverageReadFilesBundlesNil(t *testing.T) {
	f := &covForge{skills: []skill.Skill{{ID: "x", Name: "n"}}}
	tool := New()
	tool.Bind(f)

	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"op":"files","id":"x"}`))
	if !res.IsError || !strings.Contains(res.Output, "bundles are not available") {
		t.Fatalf("files nil bundles = %+v", res)
	}

	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"read","id":"x","path":"y"}`))
	if !res.IsError || !strings.Contains(res.Output, "bundles are not available") {
		t.Fatalf("read nil bundles = %+v", res)
	}

	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"read","id":"x"}`))
	if !res.IsError || !strings.Contains(res.Output, `"path"`) {
		t.Fatalf("read no path = %+v", res)
	}
}
