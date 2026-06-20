// SPDX-License-Identifier: MIT

package settings

import (
	"os"
	"reflect"
	"testing"
)

func TestValidAccountLabel(t *testing.T) {
	ok := []string{"work", "a", "acct-1", "team_a", "x0123456789012345678901234567890"} // 32
	for _, s := range ok {
		if !ValidAccountLabel(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	bad := []string{"", "_x", "-x", "Work", "a b", "a#b", "toolongtoolongtoolongtoolongtoolong", "a.b"}
	for _, s := range bad {
		if ValidAccountLabel(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func TestSuffixEnv(t *testing.T) {
	if SuffixEnv("AGEZT_EMAIL_FROM", "") != "AGEZT_EMAIL_FROM" {
		t.Fatal("default instance must use the bare name")
	}
	if SuffixEnv("AGEZT_EMAIL_FROM", "work") != "AGEZT_EMAIL_FROM#work" {
		t.Fatal("labelled instance must use #label")
	}
}

func TestAccountLabels(t *testing.T) {
	base := []string{"AGEZT_EMAIL_SMTP_ADDR", "AGEZT_EMAIL_FROM", "AGEZT_EMAIL_PASSWORD"}
	keys := []string{
		"AGEZT_EMAIL_SMTP_ADDR",       // default instance — excluded
		"AGEZT_EMAIL_SMTP_ADDR#work",  // work
		"AGEZT_EMAIL_FROM#work",       // work (dup label)
		"AGEZT_EMAIL_PASSWORD#alerts", // alerts (a vault key)
		"AGEZT_TELEGRAM_TOKEN#bot2",   // different kind — excluded
		"AGEZT_EMAIL_FROM#Bad Label",  // invalid label — excluded
		"AGEZT_EMAIL_FROM#",           // empty label — excluded
	}
	got := AccountLabels(keys, base)
	want := []string{"alerts", "work"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AccountLabels = %v, want %v", got, want)
	}
}

func TestFieldGetter(t *testing.T) {
	t.Setenv("AGEZT_X_TOKEN", "default-val")
	t.Setenv("AGEZT_X_TOKEN#work", "work-val")
	if FieldGetter("")("AGEZT_X_TOKEN") != "default-val" {
		t.Fatal("default getter should read the bare name")
	}
	if FieldGetter("work")("AGEZT_X_TOKEN") != "work-val" {
		t.Fatal("labelled getter should read the suffixed name")
	}
	if FieldGetter("missing")("AGEZT_X_TOKEN") != "" {
		t.Fatal("unknown instance should be empty")
	}
	_ = os.Unsetenv
}

func TestSectionEnvs(t *testing.T) {
	envs := SectionEnvs("email")
	if len(envs) == 0 {
		t.Fatal("email section should have env fields")
	}
	found := false
	for _, e := range envs {
		if e == "AGEZT_EMAIL_SMTP_ADDR" {
			found = true
		}
	}
	if !found {
		t.Fatalf("email section envs missing SMTP addr: %v", envs)
	}
	if SectionEnvs("nonexistent-section") != nil {
		t.Fatal("unknown section should return nil")
	}
}
