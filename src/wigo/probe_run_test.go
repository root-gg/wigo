package wigo

import (
	"os"
	"path/filepath"
	"testing"
)

// recordingRunner stands in for the binary, which is what really executes a
// probe. What is worth testing here is which probe gets run, with what, and
// when the request is refused before anything runs at all.
type recordingRunner struct {
	calls    int
	path     string
	interval int
	timeout  int
}

func (runner *recordingRunner) install(t *testing.T) {
	t.Helper()

	SetProbeRunner(func(path string, interval int, timeout int) {
		runner.calls++
		runner.path = path
		runner.interval = interval
		runner.timeout = timeout
	})
	t.Cleanup(func() { SetProbeRunner(nil) })
}

func setupRunTest(t *testing.T, probes ...string) (string, *recordingRunner) {
	t.Helper()

	setupTestWigo(t, "databases")
	root := newTestProbesDirectory(t, probes...)
	LocalWigo.config.Global.ProbesDirectory = root

	runner := &recordingRunner{}
	runner.install(t)

	return root, runner
}

func TestRunProbeNowRunsTheScheduledCopy(t *testing.T) {
	root, runner := setupRunTest(t, "300/check_load")

	if err := RunProbeNow("check_load"); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	if runner.calls != 1 {
		t.Fatalf("Got %d runs, expected one", runner.calls)
	}
	if want := filepath.Join(root, "300", "check_load"); runner.path != want {
		t.Errorf("Got %q, expected %q", runner.path, want)
	}
	// The result is stamped with the interval it really runs at, not with
	// whatever the on demand run was bounded to
	if runner.interval != 300 {
		t.Errorf("Got interval %d, expected 300", runner.interval)
	}
}

// A probe gets its whole interval minus a second when it is scheduled. On
// demand an http request is waiting on it, so the wait is capped.
func TestOnDemandRunIsBounded(t *testing.T) {
	_, runner := setupRunTest(t, "3600/check_load", "10/smart")

	if err := RunProbeNow("check_load"); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if runner.timeout != maxOnDemandProbeTimeout {
		t.Errorf("Got timeout %d, expected it capped at %d", runner.timeout, maxOnDemandProbeTimeout)
	}

	// A short interval keeps its own, shorter, timeout
	if err := RunProbeNow("smart"); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if runner.timeout != 9 {
		t.Errorf("Got timeout %d, expected 9", runner.timeout)
	}
}

// Running a disabled probe would put a fresh result on screen for a check that
// is not happening, which is exactly what being disabled has to stay visible as.
func TestRunProbeNowRefusesADisabledProbe(t *testing.T) {
	root, runner := setupRunTest(t)
	shipProbe(t, root, "hbase-master")

	err := RunProbeNow("hbase-master")
	if err == nil {
		t.Fatalf("Running a disabled probe should be refused")
	}
	if runner.calls != 0 {
		t.Errorf("Nothing should have been run")
	}
}

func TestRunProbeNowRefusesAnInvalidName(t *testing.T) {
	_, runner := setupRunTest(t, "60/check_load")

	for _, name := range []string{"", "..", "../../etc/passwd", "sub/probe"} {
		if err := RunProbeNow(name); err == nil {
			t.Errorf("Running %q should be refused", name)
		}
	}
	if runner.calls != 0 {
		t.Errorf("Nothing should have been run")
	}
}

// A probe that exited 13 is skipped by the scheduler, and until now a restart
// was the only way back. Asking for it explicitly is that way back.
func TestRunProbeNowGivesASkippedProbeItsChance(t *testing.T) {
	_, runner := setupRunTest(t, "60/check_mdadm")

	GetLocalWigo().DisableProbe("check_mdadm")
	if !GetLocalWigo().IsProbeDisabled("check_mdadm") {
		t.Fatalf("The probe should be skipped")
	}

	if err := RunProbeNow("check_mdadm"); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	if runner.calls != 1 {
		t.Errorf("The probe should have been run anyway")
	}
	if GetLocalWigo().IsProbeDisabled("check_mdadm") {
		t.Errorf("It should be back in the rotation, and exit 13 again on its own if nothing changed")
	}
}

// A probe scheduled at several intervals runs at the shortest of them, so that
// is the one a recheck reproduces.
func TestRunProbeNowUsesTheShortestInterval(t *testing.T) {
	root, runner := setupRunTest(t, "900/iostat", "60/iostat")

	if err := RunProbeNow("iostat"); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if runner.interval != 60 {
		t.Errorf("Got interval %d, expected 60", runner.interval)
	}
	if want := filepath.Join(root, "60", "iostat"); runner.path != want {
		t.Errorf("Got %q, expected %q", runner.path, want)
	}
}

// A wigo whose binary never registered a runner must say so rather than
// silently answer that everything went well.
func TestRunProbeNowWithoutARunner(t *testing.T) {
	setupTestWigo(t, "databases")
	LocalWigo.config.Global.ProbesDirectory = newTestProbesDirectory(t, "60/check_load")
	SetProbeRunner(nil)

	if err := RunProbeNow("check_load"); err == nil {
		t.Errorf("It should refuse rather than pretend the probe ran")
	}
}

// An order arriving from a master goes through the same checks as anything
// arriving over http.
func TestApplyRunProbeCommandValidatesItsInput(t *testing.T) {
	root, runner := setupRunTest(t, "60/check_load")
	shipProbe(t, root, "hbase-master")

	if err := ApplyProbeCommand(ProbeCommand{Action: CommandRunProbe, Probe: "../../etc/passwd"}); err == nil {
		t.Errorf("A traversing probe name should be refused")
	}
	if err := ApplyProbeCommand(ProbeCommand{Action: CommandRunProbe, Probe: "hbase-master"}); err == nil {
		t.Errorf("A disabled probe should be refused")
	}
	if runner.calls != 0 {
		t.Errorf("Nothing should have been run")
	}

	if err := ApplyProbeCommand(ProbeCommand{Action: CommandRunProbe, Probe: "check_load"}); err != nil {
		t.Errorf("Unexpected error : %s", err)
	}
	if runner.calls != 1 {
		t.Errorf("The probe should have been run")
	}
}

// The runner is read while orders are applied from another goroutine, so it is
// guarded. Nothing here should race.
func TestSetProbeRunnerIsSafeToReadWhileWriting(t *testing.T) {
	_, _ = setupRunTest(t, "60/check_load")

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			SetProbeRunner(func(string, int, int) {})
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		_ = RunProbeNow("check_load")
	}
	<-done
}

// A probe file that is gone leaves the name in the directory listing but the
// path unusable, and that has to surface rather than run something else.
func TestRunProbeNowOnAProbeThatVanished(t *testing.T) {
	root, runner := setupRunTest(t, "60/check_load")

	if err := os.Remove(filepath.Join(root, "60", "check_load")); err != nil {
		t.Fatalf("Fail to remove the probe : %s", err)
	}

	if err := RunProbeNow("check_load"); err == nil {
		t.Errorf("Running a probe that is no longer scheduled should be refused")
	}
	if runner.calls != 0 {
		t.Errorf("Nothing should have been run")
	}
}
