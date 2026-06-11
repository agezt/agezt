// SPDX-License-Identifier: MIT

package builtinskills

import (
	"testing"

	"github.com/agezt/agezt/kernel/skill"
)

// newForge builds a real Forge over a temp store + bundle store — the same wiring
// the daemon uses, so the seed test exercises Create + bundle materialization +
// promotion end to end.
func newForge(t *testing.T) *skill.Forge {
	t.Helper()
	dir := t.TempDir()
	store, err := skill.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	f := skill.NewForge(store, nil)
	bundles, err := skill.OpenBundles(dir)
	if err != nil {
		t.Fatalf("open bundles: %v", err)
	}
	f.SetBundles(bundles)
	return f
}

func TestSeedAll_InstallsActiveBrowserUse(t *testing.T) {
	f := newForge(t)
	seeded, err := SeedAll(f, "")
	if err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	if len(seeded) != len(builtinBundles) {
		t.Fatalf("seeded %d bundles, want %d", len(seeded), len(builtinBundles))
	}

	var bu *Seeded
	for i := range seeded {
		if seeded[i].Name == "browser-use" {
			bu = &seeded[i]
		}
	}
	if bu == nil {
		t.Fatalf("browser-use not seeded: %+v", seeded)
	}
	if !bu.Created {
		t.Errorf("first seed should create the skill")
	}
	if bu.Status != skill.StatusActive {
		t.Errorf("browser-use status = %q, want active (in the retrieval pool)", bu.Status)
	}

	// The bundle's scripts/reference must be materialized on disk.
	files, err := f.Bundles().List("browser-use")
	if err != nil {
		t.Fatalf("list bundle: %v", err)
	}
	wantFiles := map[string]bool{"scripts/browse.mjs": false, "scripts/setup.sh": false, "reference/actions.md": false}
	for _, rel := range files {
		if _, ok := wantFiles[rel]; ok {
			wantFiles[rel] = true
		}
	}
	for rel, found := range wantFiles {
		if !found {
			t.Errorf("bundle missing %q (got %v)", rel, files)
		}
	}

	// The driver script is real, non-empty.
	driver, err := f.Bundles().Read("browser-use", "scripts/browse.mjs")
	if err != nil || len(driver) == 0 {
		t.Errorf("browse.mjs unreadable/empty: %v", err)
	}
}

func TestSeedAll_InstallsComputerUse(t *testing.T) {
	f := newForge(t)
	if _, err := SeedAll(f, ""); err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	// The computer-use bundle's desktop driver must be materialized.
	driver, err := f.Bundles().Read("computer-use", "scripts/desktop.py")
	if err != nil || len(driver) == 0 {
		t.Fatalf("computer-use desktop.py unreadable/empty: %v", err)
	}
	files, _ := f.Bundles().List("computer-use")
	want := map[string]bool{"scripts/desktop.py": false, "scripts/setup.sh": false, "reference/patterns.md": false}
	for _, rel := range files {
		if _, ok := want[rel]; ok {
			want[rel] = true
		}
	}
	for rel, found := range want {
		if !found {
			t.Errorf("computer-use bundle missing %q (got %v)", rel, files)
		}
	}
}

func TestSeedAll_InstallsDataAnalysis(t *testing.T) {
	f := newForge(t)
	if _, err := SeedAll(f, ""); err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	driver, err := f.Bundles().Read("data-analysis", "scripts/analyze.py")
	if err != nil || len(driver) == 0 {
		t.Fatalf("data-analysis analyze.py unreadable/empty: %v", err)
	}
	files, _ := f.Bundles().List("data-analysis")
	want := map[string]bool{"scripts/analyze.py": false, "scripts/setup.sh": false, "reference/recipes.md": false}
	for _, rel := range files {
		if _, ok := want[rel]; ok {
			want[rel] = true
		}
	}
	for rel, found := range want {
		if !found {
			t.Errorf("data-analysis bundle missing %q (got %v)", rel, files)
		}
	}
}

func TestSeedAll_InstallsDockerServices(t *testing.T) {
	f := newForge(t)
	seeded, err := SeedAll(f, "")
	if err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	var got *Seeded
	for i := range seeded {
		if seeded[i].Name == "docker-services" {
			got = &seeded[i]
		}
	}
	if got == nil {
		t.Fatalf("docker-services not seeded: %+v", seeded)
	}
	if got.Status != skill.StatusActive {
		t.Errorf("docker-services status = %q, want active", got.Status)
	}
	svc, err := f.Bundles().Read("docker-services", "scripts/svc.sh")
	if err != nil || len(svc) == 0 {
		t.Fatalf("docker-services svc.sh unreadable/empty: %v", err)
	}
	files, _ := f.Bundles().List("docker-services")
	want := map[string]bool{"scripts/svc.sh": false, "reference/services.md": false}
	for _, rel := range files {
		if _, ok := want[rel]; ok {
			want[rel] = true
		}
	}
	for rel, found := range want {
		if !found {
			t.Errorf("docker-services bundle missing %q (got %v)", rel, files)
		}
	}
}

func TestSeedAll_InstallsGitOps(t *testing.T) {
	f := newForge(t)
	seeded, err := SeedAll(f, "")
	if err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	var got *Seeded
	for i := range seeded {
		if seeded[i].Name == "git-ops" {
			got = &seeded[i]
		}
	}
	if got == nil {
		t.Fatalf("git-ops not seeded: %+v", seeded)
	}
	if got.Status != skill.StatusActive {
		t.Errorf("git-ops status = %q, want active", got.Status)
	}
	flow, err := f.Bundles().Read("git-ops", "scripts/gitflow.sh")
	if err != nil || len(flow) == 0 {
		t.Fatalf("git-ops gitflow.sh unreadable/empty: %v", err)
	}
	files, _ := f.Bundles().List("git-ops")
	want := map[string]bool{"scripts/gitflow.sh": false, "reference/recipes.md": false}
	for _, rel := range files {
		if _, ok := want[rel]; ok {
			want[rel] = true
		}
	}
	for rel, found := range want {
		if !found {
			t.Errorf("git-ops bundle missing %q (got %v)", rel, files)
		}
	}
}

func TestSeedAll_InstallsWebResearch(t *testing.T) {
	f := newForge(t)
	seeded, err := SeedAll(f, "")
	if err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	var got *Seeded
	for i := range seeded {
		if seeded[i].Name == "web-research" {
			got = &seeded[i]
		}
	}
	if got == nil {
		t.Fatalf("web-research not seeded: %+v", seeded)
	}
	if got.Status != skill.StatusActive {
		t.Errorf("web-research status = %q, want active", got.Status)
	}
	ex, err := f.Bundles().Read("web-research", "scripts/extract.py")
	if err != nil || len(ex) == 0 {
		t.Fatalf("web-research extract.py unreadable/empty: %v", err)
	}
	files, _ := f.Bundles().List("web-research")
	want := map[string]bool{"scripts/extract.py": false, "scripts/setup.sh": false, "reference/recipes.md": false}
	for _, rel := range files {
		if _, ok := want[rel]; ok {
			want[rel] = true
		}
	}
	for rel, found := range want {
		if !found {
			t.Errorf("web-research bundle missing %q (got %v)", rel, files)
		}
	}
}

func TestSeedAll_InstallsPDFTools(t *testing.T) {
	f := newForge(t)
	seeded, err := SeedAll(f, "")
	if err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	var got *Seeded
	for i := range seeded {
		if seeded[i].Name == "pdf-tools" {
			got = &seeded[i]
		}
	}
	if got == nil {
		t.Fatalf("pdf-tools not seeded: %+v", seeded)
	}
	if got.Status != skill.StatusActive {
		t.Errorf("pdf-tools status = %q, want active", got.Status)
	}
	drv, err := f.Bundles().Read("pdf-tools", "scripts/pdf.py")
	if err != nil || len(drv) == 0 {
		t.Fatalf("pdf-tools pdf.py unreadable/empty: %v", err)
	}
	files, _ := f.Bundles().List("pdf-tools")
	want := map[string]bool{"scripts/pdf.py": false, "scripts/setup.sh": false, "reference/recipes.md": false}
	for _, rel := range files {
		if _, ok := want[rel]; ok {
			want[rel] = true
		}
	}
	for rel, found := range want {
		if !found {
			t.Errorf("pdf-tools bundle missing %q (got %v)", rel, files)
		}
	}
}

func TestSeedAll_InstallsImageTools(t *testing.T) {
	f := newForge(t)
	seeded, err := SeedAll(f, "")
	if err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	var got *Seeded
	for i := range seeded {
		if seeded[i].Name == "image-tools" {
			got = &seeded[i]
		}
	}
	if got == nil {
		t.Fatalf("image-tools not seeded: %+v", seeded)
	}
	if got.Status != skill.StatusActive {
		t.Errorf("image-tools status = %q, want active", got.Status)
	}
	drv, err := f.Bundles().Read("image-tools", "scripts/img.py")
	if err != nil || len(drv) == 0 {
		t.Fatalf("image-tools img.py unreadable/empty: %v", err)
	}
	files, _ := f.Bundles().List("image-tools")
	want := map[string]bool{"scripts/img.py": false, "scripts/setup.sh": false, "reference/recipes.md": false}
	for _, rel := range files {
		if _, ok := want[rel]; ok {
			want[rel] = true
		}
	}
	for rel, found := range want {
		if !found {
			t.Errorf("image-tools bundle missing %q (got %v)", rel, files)
		}
	}
}

func TestSeedAll_InstallsSQLDB(t *testing.T) {
	f := newForge(t)
	seeded, err := SeedAll(f, "")
	if err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	var got *Seeded
	for i := range seeded {
		if seeded[i].Name == "sql-db" {
			got = &seeded[i]
		}
	}
	if got == nil {
		t.Fatalf("sql-db not seeded: %+v", seeded)
	}
	if got.Status != skill.StatusActive {
		t.Errorf("sql-db status = %q, want active", got.Status)
	}
	drv, err := f.Bundles().Read("sql-db", "scripts/db.py")
	if err != nil || len(drv) == 0 {
		t.Fatalf("sql-db db.py unreadable/empty: %v", err)
	}
	files, _ := f.Bundles().List("sql-db")
	want := map[string]bool{"scripts/db.py": false, "scripts/setup.sh": false, "reference/recipes.md": false}
	for _, rel := range files {
		if _, ok := want[rel]; ok {
			want[rel] = true
		}
	}
	for rel, found := range want {
		if !found {
			t.Errorf("sql-db bundle missing %q (got %v)", rel, files)
		}
	}
}

func TestSeedAll_InstallsArchiveTools(t *testing.T) {
	f := newForge(t)
	seeded, err := SeedAll(f, "")
	if err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	var got *Seeded
	for i := range seeded {
		if seeded[i].Name == "archive-tools" {
			got = &seeded[i]
		}
	}
	if got == nil {
		t.Fatalf("archive-tools not seeded: %+v", seeded)
	}
	if got.Status != skill.StatusActive {
		t.Errorf("archive-tools status = %q, want active", got.Status)
	}
	drv, err := f.Bundles().Read("archive-tools", "scripts/arc.py")
	if err != nil || len(drv) == 0 {
		t.Fatalf("archive-tools arc.py unreadable/empty: %v", err)
	}
	files, _ := f.Bundles().List("archive-tools")
	want := map[string]bool{"scripts/arc.py": false, "reference/recipes.md": false}
	for _, rel := range files {
		if _, ok := want[rel]; ok {
			want[rel] = true
		}
	}
	for rel, found := range want {
		if !found {
			t.Errorf("archive-tools bundle missing %q (got %v)", rel, files)
		}
	}
}

func TestSeedAll_InstallsHTTPAPI(t *testing.T) {
	f := newForge(t)
	seeded, err := SeedAll(f, "")
	if err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	var got *Seeded
	for i := range seeded {
		if seeded[i].Name == "http-api-client" {
			got = &seeded[i]
		}
	}
	if got == nil {
		t.Fatalf("http-api-client not seeded: %+v", seeded)
	}
	if got.Status != skill.StatusActive {
		t.Errorf("http-api-client status = %q, want active", got.Status)
	}
	drv, err := f.Bundles().Read("http-api-client", "scripts/api.py")
	if err != nil || len(drv) == 0 {
		t.Fatalf("http-api-client api.py unreadable/empty: %v", err)
	}
	files, _ := f.Bundles().List("http-api-client")
	want := map[string]bool{"scripts/api.py": false, "scripts/setup.sh": false, "reference/recipes.md": false}
	for _, rel := range files {
		if _, ok := want[rel]; ok {
			want[rel] = true
		}
	}
	for rel, found := range want {
		if !found {
			t.Errorf("http-api-client bundle missing %q (got %v)", rel, files)
		}
	}
}

func TestSeedAll_InstallsEmailTools(t *testing.T) {
	f := newForge(t)
	seeded, err := SeedAll(f, "")
	if err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	var got *Seeded
	for i := range seeded {
		if seeded[i].Name == "email-tools" {
			got = &seeded[i]
		}
	}
	if got == nil {
		t.Fatalf("email-tools not seeded: %+v", seeded)
	}
	if got.Status != skill.StatusActive {
		t.Errorf("email-tools status = %q, want active", got.Status)
	}
	drv, err := f.Bundles().Read("email-tools", "scripts/mail.py")
	if err != nil || len(drv) == 0 {
		t.Fatalf("email-tools mail.py unreadable/empty: %v", err)
	}
	files, _ := f.Bundles().List("email-tools")
	want := map[string]bool{"scripts/mail.py": false, "reference/recipes.md": false}
	for _, rel := range files {
		if _, ok := want[rel]; ok {
			want[rel] = true
		}
	}
	for rel, found := range want {
		if !found {
			t.Errorf("email-tools bundle missing %q (got %v)", rel, files)
		}
	}
}

func TestSeedAll_InstallsSSHRemote(t *testing.T) {
	f := newForge(t)
	seeded, err := SeedAll(f, "")
	if err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	var got *Seeded
	for i := range seeded {
		if seeded[i].Name == "ssh-remote" {
			got = &seeded[i]
		}
	}
	if got == nil {
		t.Fatalf("ssh-remote not seeded: %+v", seeded)
	}
	if got.Status != skill.StatusActive {
		t.Errorf("ssh-remote status = %q, want active", got.Status)
	}
	drv, err := f.Bundles().Read("ssh-remote", "scripts/ssh.py")
	if err != nil || len(drv) == 0 {
		t.Fatalf("ssh-remote ssh.py unreadable/empty: %v", err)
	}
	files, _ := f.Bundles().List("ssh-remote")
	want := map[string]bool{"scripts/ssh.py": false, "scripts/setup.sh": false, "reference/recipes.md": false}
	for _, rel := range files {
		if _, ok := want[rel]; ok {
			want[rel] = true
		}
	}
	for rel, found := range want {
		if !found {
			t.Errorf("ssh-remote bundle missing %q (got %v)", rel, files)
		}
	}
}

func TestSeedAll_InstallsCryptoTools(t *testing.T) {
	f := newForge(t)
	seeded, err := SeedAll(f, "")
	if err != nil {
		t.Fatalf("SeedAll: %v", err)
	}
	var got *Seeded
	for i := range seeded {
		if seeded[i].Name == "crypto-tools" {
			got = &seeded[i]
		}
	}
	if got == nil {
		t.Fatalf("crypto-tools not seeded: %+v", seeded)
	}
	if got.Status != skill.StatusActive {
		t.Errorf("crypto-tools status = %q, want active", got.Status)
	}
	drv, err := f.Bundles().Read("crypto-tools", "scripts/crypto.py")
	if err != nil || len(drv) == 0 {
		t.Fatalf("crypto-tools crypto.py unreadable/empty: %v", err)
	}
	files, _ := f.Bundles().List("crypto-tools")
	want := map[string]bool{"scripts/crypto.py": false, "reference/recipes.md": false}
	for _, rel := range files {
		if _, ok := want[rel]; ok {
			want[rel] = true
		}
	}
	for rel, found := range want {
		if !found {
			t.Errorf("crypto-tools bundle missing %q (got %v)", rel, files)
		}
	}
}

func TestSeedAll_Idempotent(t *testing.T) {
	f := newForge(t)
	if _, err := SeedAll(f, ""); err != nil {
		t.Fatalf("first SeedAll: %v", err)
	}
	seeded, err := SeedAll(f, "")
	if err != nil {
		t.Fatalf("second SeedAll: %v", err)
	}
	for _, s := range seeded {
		if s.Created {
			t.Errorf("re-seed created %q again (should dedupe on content address)", s.Name)
		}
		if s.Status != skill.StatusActive {
			t.Errorf("re-seed left %q at %q, want active", s.Name, s.Status)
		}
	}
}
