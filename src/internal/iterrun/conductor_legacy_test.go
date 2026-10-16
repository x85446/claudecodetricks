package iterrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TESTMASTER: id=indigobird-web-migration tier=? parallel=no
func TestConductorLegacyImportPreviewApplyIdempotence(t *testing.T) {
	home := nightlyHome(t)
	t.Setenv("ITERATE_CONDUCTOR_DIR", filepath.Join(home, "state"))
	p := mkNightlyProject(t, home, "one", enrolledConductor, queuedPlan)
	q := mkNightlyProject(t, home, "two", plainConductor, queuedPlan)
	source := filepath.Join(p, ".claude", "iterate", "conductor.md")
	before, _ := os.ReadFile(source)
	service := NewConductorService("")
	preview, err := ImportLegacyConductor(service, []string{p, q}, false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || len(preview.Projects) != 2 {
		t.Fatalf("preview: %+v", preview)
	}
	if _, err = os.Stat(filepath.Join(service.Dir, "config.json")); !os.IsNotExist(err) {
		t.Fatal("preview persisted configuration")
	}
	report, err := ImportLegacyConductor(service, []string{p, q}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Applied || !report.Projects[0].Enabled || !report.Projects[1].Paused {
		t.Fatalf("flags/ambiguity: %+v", report)
	}
	cfg, err := service.Config()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || len(cfg.Automations) != 0 || len(cfg.Projects) != 2 {
		t.Fatalf("import must not infer dispatch: %+v", cfg)
	}
	repeat, err := ImportLegacyConductor(service, []string{p, q, p}, true)
	if err != nil {
		t.Fatal(err)
	}
	if repeat.Applied || repeat.Revision != report.Revision {
		t.Fatalf("repeated import changed configuration: %+v", repeat)
	}
	after, _ := os.ReadFile(source)
	if string(before) != string(after) {
		t.Fatal("legacy source changed")
	}
	backups, err := filepath.Glob(filepath.Join(service.Dir, "migration-backups", "*", "*", "conductor.md"))
	if err != nil || len(backups) != 2 {
		t.Fatalf("protected source backups: %v %v", backups, err)
	}
	for _, p := range backups {
		info, err := os.Stat(p)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("backup permissions: %s %v", p, err)
		}
	}
}

// TESTMASTER: id=indigobird-single-server-recovery tier=? parallel=no
func TestConductorLegacyImportLiveOwnerAndPartialBackup(t *testing.T) {
	home := nightlyHome(t)
	t.Setenv("ITERATE_CONDUCTOR_DIR", filepath.Join(home, "state"))
	p := mkNightlyProject(t, home, "owned", enrolledConductor, queuedPlan)
	unlock, err := tryNightlyLock(p)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	service := NewConductorService("")
	preview, err := ImportLegacyConductor(service, []string{p}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Projects[0].Paused || !strings.Contains(strings.Join(preview.Projects[0].Review, " "), "worker owns") {
		t.Fatalf("live owner: %+v", preview)
	}
	// A failed backup cannot publish a partially imported configuration.
	if err = os.MkdirAll(service.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(service.Dir, "migration-backups"), []byte("collision"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = ImportLegacyConductor(service, []string{p}, true); err == nil {
		t.Fatal("expected backup failure")
	}
	if _, err = os.Stat(filepath.Join(service.Dir, "config.json")); !os.IsNotExist(err) {
		t.Fatal("failed import published config")
	}
	if err = os.Remove(filepath.Join(service.Dir, "migration-backups")); err != nil {
		t.Fatal(err)
	}
	if _, err = ImportLegacyConductor(service, []string{p}, true); err != nil {
		t.Fatal(err)
	}
	if !nightlyLockHeld(p) {
		t.Fatal("import disturbed existing worker ownership")
	}
}
