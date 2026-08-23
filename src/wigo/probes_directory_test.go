package wigo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestProbesDirectory builds a probes tree with the given probes already
// installed, expressed as "directory/name".
func newTestProbesDirectory(t *testing.T, probes ...string) string {
	t.Helper()

	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "examples"), 0755); err != nil {
		t.Fatalf("Fail to create the examples directory : %s", err)
	}

	for _, probe := range probes {
		directory, name, found := strings.Cut(probe, "/")
		if !found {
			t.Fatalf("Malformed test probe %q", probe)
		}

		target := filepath.Join(root, "examples", name)
		if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0755); err != nil {
			t.Fatalf("Fail to create the probe %s : %s", name, err)
		}

		if err := os.MkdirAll(filepath.Join(root, directory), 0755); err != nil {
			t.Fatalf("Fail to create the directory %s : %s", directory, err)
		}

		if err := os.Symlink(filepath.Join("..", "examples", name), filepath.Join(root, directory, name)); err != nil {
			t.Fatalf("Fail to link the probe %s : %s", name, err)
		}
	}

	return root
}

func probeIsIn(t *testing.T, root string, directory string, name string) bool {
	t.Helper()

	_, err := os.Lstat(filepath.Join(root, directory, name))
	return err == nil
}

func TestIsValidProbeName(t *testing.T) {
	valid := []string{"check_http", "hardware_load_average", "os-release", "packages-apt", "a", "probe.sh", "A1"}
	for _, name := range valid {
		if !IsValidProbeName(name) {
			t.Errorf("%q should be a valid probe name", name)
		}
	}

	// A name is turned into a path, so anything that could escape the probes
	// directory or reach an unexpected file must be refused outright.
	invalid := []string{
		"",
		"..",
		".",
		".hidden",
		"../../etc/passwd",
		"check/../../root",
		"sub/probe",
		"probe name",
		"probe;rm -rf /",
		"probe\x00",
		"probe\n",
		"-probe",
		"_probe",
		strings.Repeat("a", 256),
	}
	for _, name := range invalid {
		if IsValidProbeName(name) {
			t.Errorf("%q should be refused as a probe name", name)
		}
	}
}

func TestProbePathRefusesEscapes(t *testing.T) {
	root := "/usr/local/wigo/probes"

	if _, err := probePath(root, "60", "../../../etc/passwd"); err == nil {
		t.Errorf("A traversing name should not produce a path")
	}
	if _, err := probePath(root, "../../etc", "passwd"); err == nil {
		t.Errorf("A traversing directory should not produce a path")
	}
	if _, err := probePath(root, "examples", "check_http"); err == nil {
		t.Errorf("examples is not a probe location and should be refused")
	}

	path, err := probePath(root, "60", "check_http")
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if want := "/usr/local/wigo/probes/60/check_http"; path != want {
		t.Errorf("Got %q, expected %q", path, want)
	}
}

func TestProbeDirectoryInterval(t *testing.T) {
	if interval, ok := ProbeDirectoryInterval("300"); !ok || interval != 300 {
		t.Errorf("Got %d %v, expected 300 true", interval, ok)
	}
	for _, directory := range []string{"examples", "disabled", "", "-60", "0", "60s"} {
		if _, ok := ProbeDirectoryInterval(directory); ok {
			t.Errorf("%q should not be read as a check interval", directory)
		}
	}
}

func TestScheduleProbeChangesInterval(t *testing.T) {
	root := newTestProbesDirectory(t, "60/check_load")

	if err := scheduleProbeIn(root, "check_load", 300); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	if probeIsIn(t, root, "60", "check_load") {
		t.Errorf("The probe is still scheduled every 60 seconds")
	}
	if !probeIsIn(t, root, "300", "check_load") {
		t.Errorf("The probe has not been moved to the 300 directory")
	}
}

func TestScheduleProbeCreatesMissingDirectory(t *testing.T) {
	root := newTestProbesDirectory(t, "60/check_load")

	if err := scheduleProbeIn(root, "check_load", 900); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if !probeIsIn(t, root, "900", "check_load") {
		t.Errorf("The 900 directory has not been created")
	}
}

func TestScheduleProbeRefusesOutOfRangeInterval(t *testing.T) {
	root := newTestProbesDirectory(t, "60/check_load")

	// One second would leave a zero timeout and kill every run
	for _, interval := range []int{0, 1, -60, MaxProbeInterval + 1} {
		if err := scheduleProbeIn(root, "check_load", interval); err == nil {
			t.Errorf("An interval of %d should be refused", interval)
		}
	}

	if !probeIsIn(t, root, "60", "check_load") {
		t.Errorf("The probe should not have moved")
	}
}

func TestUnscheduleAndRescheduleProbe(t *testing.T) {
	root := newTestProbesDirectory(t, "60/check_load")

	if err := unscheduleProbeIn(root, "check_load"); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if probeIsIn(t, root, "60", "check_load") {
		t.Errorf("The probe is still scheduled")
	}
	if probeIsScheduledIn(root, "check_load") {
		t.Errorf("The probe should be reported as disabled")
	}

	if err := scheduleProbeIn(root, "check_load", 120); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if !probeIsIn(t, root, "120", "check_load") {
		t.Errorf("The probe has not been scheduled again")
	}
	if !probeIsScheduledIn(root, "check_load") {
		t.Errorf("The probe should be running again")
	}
}

// The probe has to actually be there : acting on a name that is not installed
// must fail loudly rather than create something.
func TestMoveUnknownProbeFails(t *testing.T) {
	root := newTestProbesDirectory(t, "60/check_load")

	err := scheduleProbeIn(root, "check_mdadm", 300)
	if err == nil {
		t.Fatalf("Scheduling a probe that is not installed should fail")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Got %q, expected a not found error", err)
	}
	if probeIsIn(t, root, "300", "check_mdadm") {
		t.Errorf("A probe has been created out of thin air")
	}
}

// shipProbe installs a probe without scheduling it, which is to say disabled :
// the state of the fifteen probes wigo ships and the packaging does not link.
func shipProbe(t *testing.T, root string, name string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(root, "examples", name), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("Fail to create the example %s : %s", name, err)
	}
}

// A disabled probe is scheduled from nowhere, so it is not one of the places
// anything may be moved out of.
func TestDisabledProbeIsScheduledNowhere(t *testing.T) {
	root := newTestProbesDirectory(t)
	shipProbe(t, root, "check_ntp")

	locations, err := findProbeLocationsIn(root, "check_ntp")
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(locations) != 0 {
		t.Errorf("Got %v, expected the probe not to be installed", locations)
	}

	if probeIsScheduledIn(root, "check_ntp") {
		t.Errorf("A probe that is merely shipped is not scheduled")
	}
}

// Half of the probes wigo ships are never linked by the packaging, and they are
// not running any more than one somebody turned off. Being disabled is the
// absence of a schedule, so both are the same state and both are listed.
func TestListingIncludesEveryProbeNothingSchedules(t *testing.T) {
	root := newTestProbesDirectory(t, "60/check_load")
	shipProbe(t, root, "hbase-master")

	locations, err := probeLocationsIn(root)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(locations) != 2 {
		t.Fatalf("Got %+v, expected the two probes", locations)
	}

	byName := make(map[string]ProbeLocation)
	for _, location := range locations {
		byName[location.Name] = location
	}

	disabled := byName["hbase-master"]
	if disabled.Enabled || disabled.Directory != ExampleProbesDirectory || disabled.Interval != 0 {
		t.Errorf("Got %+v, expected hbase-master to be listed as disabled", disabled)
	}
	if !byName["check_load"].Enabled || byName["check_load"].Interval != 60 {
		t.Errorf("Got %+v for check_load", byName["check_load"])
	}
}

// A probe that runs must not also be listed as disabled from the file its
// schedule links to.
func TestListingDoesNotRepeatAScheduledProbe(t *testing.T) {
	root := newTestProbesDirectory(t, "60/check_load")

	locations, err := probeLocationsIn(root)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(locations) != 1 {
		t.Errorf("Got %+v, expected check_load once", locations)
	}
}

// Enabling a disabled probe is creating the link the packaging would have
// created. The probe itself never moves.
func TestScheduleLinksADisabledProbe(t *testing.T) {
	root := newTestProbesDirectory(t)
	shipProbe(t, root, "hbase-master")

	if err := scheduleProbeIn(root, "hbase-master", 300); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	if !probeIsIn(t, root, "300", "hbase-master") {
		t.Fatalf("The probe has not been scheduled")
	}
	if _, err := os.Lstat(filepath.Join(root, "examples", "hbase-master")); err != nil {
		t.Errorf("The probe itself should not have moved : %s", err)
	}

	target, err := os.Readlink(filepath.Join(root, "300", "hbase-master"))
	if err != nil {
		t.Fatalf("The schedule should be a symlink : %s", err)
	}
	if want := filepath.Join("..", "examples", "hbase-master"); target != want {
		t.Errorf("Got a link to %q, expected %q", target, want)
	}

	// And it behaves like any other scheduled probe from there on
	if err := scheduleProbeIn(root, "hbase-master", 60); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if probeIsIn(t, root, "300", "hbase-master") || !probeIsIn(t, root, "60", "hbase-master") {
		t.Errorf("The freshly scheduled probe could not be repitched")
	}
}

// Disabling means no interval directory links to the probe any more. Nothing is
// parked anywhere : there is no such place.
func TestDisableRemovesEverySchedule(t *testing.T) {
	root := newTestProbesDirectory(t, "60/check_load")

	if err := unscheduleProbeIn(root, "check_load"); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	if probeIsIn(t, root, "60", "check_load") {
		t.Errorf("The probe is still scheduled")
	}
	if probeIsIn(t, root, DisabledProbesDirectory, "check_load") {
		t.Errorf("Nothing should be parked in a disabled directory, there is no such state")
	}
	if !probeIsIn(t, root, ExampleProbesDirectory, "check_load") {
		t.Errorf("The probe itself should still be installed")
	}
	if probeIsScheduledIn(root, "check_load") {
		t.Errorf("The probe should no longer be scheduled")
	}
}

// A probe scheduled at several intervals is disabled once, everywhere.
func TestDisableRemovesEveryScheduleOfAnAmbiguousProbe(t *testing.T) {
	root := newTestProbesDirectory(t, "60/iostat", "300/iostat", "900/iostat")

	if err := unscheduleProbeIn(root, "iostat"); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	for _, directory := range []string{"60", "300", "900"} {
		if probeIsIn(t, root, directory, "iostat") {
			t.Errorf("The probe is still scheduled in %s", directory)
		}
	}
	if !probeIsIn(t, root, ExampleProbesDirectory, "iostat") {
		t.Errorf("The probe itself should still be installed")
	}
}

// Disabling a probe nothing schedules is asking for the state it is already in.
// Refusing would make an interface unable to agree with itself.
func TestDisableAnAlreadyDisabledProbeIsANoop(t *testing.T) {
	root := newTestProbesDirectory(t)
	shipProbe(t, root, "hbase-master")

	if err := unscheduleProbeIn(root, "hbase-master"); err != nil {
		t.Errorf("Disabling an already disabled probe should succeed : %s", err)
	}
	if !probeIsIn(t, root, ExampleProbesDirectory, "hbase-master") {
		t.Errorf("The probe should have been left alone")
	}
}

// Disabling must not destroy anything. An administrator may drop a script
// straight into an interval directory rather than link to one : deleting it
// would be the only copy gone, and the probe would vanish from the listing with
// no way to turn it back on.
func TestDisableKeepsAProbeThatIsNotALink(t *testing.T) {
	root := newTestProbesDirectory(t)
	if err := os.MkdirAll(filepath.Join(root, "60"), 0755); err != nil {
		t.Fatalf("Fail to create the directory : %s", err)
	}
	script := "#!/bin/sh\necho mine\n"
	if err := os.WriteFile(filepath.Join(root, "60", "mycheck"), []byte(script), 0755); err != nil {
		t.Fatalf("Fail to create the probe : %s", err)
	}

	if err := unscheduleProbeIn(root, "mycheck"); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	if probeIsIn(t, root, "60", "mycheck") {
		t.Errorf("The probe is still scheduled")
	}

	kept, err := os.ReadFile(filepath.Join(root, ExampleProbesDirectory, "mycheck"))
	if err != nil {
		t.Fatalf("The probe has been destroyed instead of kept : %s", err)
	}
	if string(kept) != script {
		t.Errorf("Got %q, the probe was not kept as it was", kept)
	}

	// And it is still listed, as disabled, so it can be turned back on
	locations, err := probeLocationsIn(root)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(locations) != 1 || locations[0].Name != "mycheck" || locations[0].Enabled {
		t.Errorf("Got %+v, expected mycheck listed as disabled", locations)
	}
}

// A schedule pointing outside the probes tree is the same case : the link is
// the only record of where that probe lives.
func TestDisableKeepsALinkPointingOutside(t *testing.T) {
	root := newTestProbesDirectory(t)
	outside := filepath.Join(t.TempDir(), "custom_check")
	if err := os.WriteFile(outside, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("Fail to create the probe : %s", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "60"), 0755); err != nil {
		t.Fatalf("Fail to create the directory : %s", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "60", "custom_check")); err != nil {
		t.Fatalf("Fail to link the probe : %s", err)
	}

	if err := unscheduleProbeIn(root, "custom_check"); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	target, err := os.Readlink(filepath.Join(root, ExampleProbesDirectory, "custom_check"))
	if err != nil {
		t.Fatalf("The link has been destroyed instead of kept : %s", err)
	}
	if target != outside {
		t.Errorf("Got a link to %q, expected %q", target, outside)
	}
}

// A probe parked in the disabled directory by an earlier version of this
// feature is still disabled, and must stay visible rather than quietly vanish.
func TestLegacyDisabledDirectoryIsStillRead(t *testing.T) {
	root := newTestProbesDirectory(t, "disabled/smart")

	locations, err := probeLocationsIn(root)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(locations) != 1 || locations[0].Name != "smart" || locations[0].Enabled {
		t.Fatalf("Got %+v, expected smart listed as disabled", locations)
	}

	// And scheduling it moves it out of there for good
	if err := scheduleProbeIn(root, "smart", 300); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if probeIsIn(t, root, DisabledProbesDirectory, "smart") {
		t.Errorf("The probe should have left the disabled directory")
	}
	if !probeIsIn(t, root, "300", "smart") {
		t.Errorf("The probe has not been scheduled")
	}
}

// Disabling one parked there by the earlier version empties it too, rather than
// leaving the probe in a directory that no longer means anything.
func TestLegacyDisabledDirectoryIsEmptiedOnDisable(t *testing.T) {
	root := newTestProbesDirectory(t, "disabled/smart")

	if err := unscheduleProbeIn(root, "smart"); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if probeIsIn(t, root, DisabledProbesDirectory, "smart") {
		t.Errorf("The probe should have left the disabled directory")
	}
	if !probeIsIn(t, root, ExampleProbesDirectory, "smart") {
		t.Errorf("The probe itself should still be installed")
	}
}

// A probe that is not installed at all is still refused, and no link may be
// created for one.
func TestScheduleStillRefusesAProbeThatDoesNotExist(t *testing.T) {
	root := newTestProbesDirectory(t, "60/check_load")

	if err := scheduleProbeIn(root, "check_absent", 300); err == nil {
		t.Errorf("Scheduling a probe that is not installed should fail")
	}
	if probeIsIn(t, root, "300", "check_absent") {
		t.Errorf("A link has been created for a probe that does not exist")
	}
}

func TestExamplePathRefusesEscapes(t *testing.T) {
	root := "/usr/local/wigo/probes"

	for _, name := range []string{"../../etc/passwd", "", "..", "sub/probe", ".hidden"} {
		if _, err := examplePath(root, name); err == nil {
			t.Errorf("%q should not produce an example path", name)
		}
	}

	path, err := examplePath(root, "hbase-master")
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if want := "/usr/local/wigo/probes/examples/hbase-master"; path != want {
		t.Errorf("Got %q, expected %q", path, want)
	}
}

// A directory in examples/ is not a probe, and neither is a name we would never
// accept from a request.
func TestShippedListingSkipsWhatIsNotAProbe(t *testing.T) {
	root := newTestProbesDirectory(t)
	shipProbe(t, root, "hbase-master")
	if err := os.MkdirAll(filepath.Join(root, "examples", "lib"), 0755); err != nil {
		t.Fatalf("Fail to create the directory : %s", err)
	}
	shipProbe(t, root, ".gitkeep")

	locations, err := probeLocationsIn(root)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(locations) != 1 || locations[0].Name != "hbase-master" {
		t.Errorf("Got %+v, expected only hbase-master", locations)
	}
}

func TestMoveRefusesInvalidName(t *testing.T) {
	root := newTestProbesDirectory(t, "60/check_load")

	for _, name := range []string{"../../etc/passwd", "", "sub/probe", ".."} {
		if err := scheduleProbeIn(root, name, 300); err == nil {
			t.Errorf("Scheduling %q should be refused", name)
		}
		if err := unscheduleProbeIn(root, name); err == nil {
			t.Errorf("Unscheduling %q should be refused", name)
		}
	}
}

// A probe installed at two intervals runs twice per cycle. Asking for an
// interval means asking for it to run once, so the extra copies go rather than
// being left behind : moving one of them would leave it running from the other.
func TestMoveConsolidatesAnAmbiguousProbe(t *testing.T) {
	root := newTestProbesDirectory(t, "60/iostat", "300/iostat")

	if err := scheduleProbeIn(root, "iostat", 900); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	if probeIsIn(t, root, "60", "iostat") || probeIsIn(t, root, "300", "iostat") {
		t.Errorf("The extra copies should be gone")
	}
	if !probeIsIn(t, root, "900", "iostat") {
		t.Errorf("The probe has not been moved to the 900 directory")
	}

	locations, err := findProbeLocationsIn(root, "iostat")
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(locations) != 1 {
		t.Errorf("Got %+v, expected the probe to be installed exactly once", locations)
	}
}

// One copy already in the target directory is the one kept, so nothing is moved
// and the others simply go.
func TestMoveKeepsTheCopyAlreadyInPlace(t *testing.T) {
	root := newTestProbesDirectory(t, "60/iostat", "300/iostat")

	if err := scheduleProbeIn(root, "iostat", 300); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	if probeIsIn(t, root, "60", "iostat") {
		t.Errorf("The extra copy should be gone")
	}
	if !probeIsIn(t, root, "300", "iostat") {
		t.Errorf("The copy already in place should have been kept")
	}
}

// A probe left in the old disabled directory by an earlier version while also
// being scheduled was not disabled at all : it ran. Disabling it empties both.
func TestDisableEmptiesTheLegacyDirectoryAndTheSchedule(t *testing.T) {
	root := newTestProbesDirectory(t, "60/iostat", "disabled/iostat")

	if err := unscheduleProbeIn(root, "iostat"); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	if probeIsIn(t, root, "60", "iostat") {
		t.Errorf("The probe is still scheduled every 60 seconds")
	}
	if probeIsIn(t, root, DisabledProbesDirectory, "iostat") {
		t.Errorf("The probe should have left the disabled directory")
	}
	if !probeIsIn(t, root, ExampleProbesDirectory, "iostat") {
		t.Errorf("The probe itself should still be installed")
	}
}

// The same the other way round : scheduling has to empty the legacy directory,
// otherwise the probe would look disabled while running.
func TestScheduleEmptiesTheLegacyDisabledDirectory(t *testing.T) {
	root := newTestProbesDirectory(t, "60/iostat", "disabled/iostat")

	if err := scheduleProbeIn(root, "iostat", 60); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	if probeIsIn(t, root, DisabledProbesDirectory, "iostat") {
		t.Errorf("The disabled copy should be gone")
	}
	if !probeIsIn(t, root, "60", "iostat") {
		t.Errorf("The probe should still run every 60 seconds")
	}
}

func TestMoveToSameDirectoryIsANoop(t *testing.T) {
	root := newTestProbesDirectory(t, "60/check_load")

	if err := scheduleProbeIn(root, "check_load", 60); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if !probeIsIn(t, root, "60", "check_load") {
		t.Errorf("The probe should still be there")
	}
}

func TestFindProbeLocations(t *testing.T) {
	root := newTestProbesDirectory(t, "60/check_load", "disabled/check_mdadm")

	locations, err := findProbeLocationsIn(root, "check_load")
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(locations) != 1 || locations[0].Interval != 60 || !locations[0].Enabled {
		t.Errorf("Got %+v, expected check_load enabled every 60 seconds", locations)
	}

	locations, err = findProbeLocationsIn(root, "check_mdadm")
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(locations) != 1 || locations[0].Enabled || locations[0].Interval != 0 {
		t.Errorf("Got %+v, expected check_mdadm disabled", locations)
	}
}

func TestProbeLocationsListsEverything(t *testing.T) {
	root := newTestProbesDirectory(t, "60/check_load", "300/smart", "disabled/check_mdadm")

	locations, err := probeLocationsIn(root)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(locations) != 3 {
		t.Fatalf("Got %+v, expected three probes", locations)
	}

	byName := make(map[string]ProbeLocation)
	for _, location := range locations {
		byName[location.Name] = location
	}

	if byName["check_load"].Interval != 60 || !byName["check_load"].Enabled {
		t.Errorf("Got %+v for check_load", byName["check_load"])
	}
	if byName["smart"].Interval != 300 {
		t.Errorf("Got %+v for smart", byName["smart"])
	}
	if byName["check_mdadm"].Enabled {
		t.Errorf("check_mdadm should be reported as disabled")
	}
}

// A probe leaving an interval directory has not necessarily gone away : it may
// have been repitched. Its result belongs to whichever directory holds it now,
// and the one it left must not drop it.
func TestProbeIsScheduled(t *testing.T) {
	root := newTestProbesDirectory(t, "60/check_load", "disabled/check_mdadm")

	if !probeIsScheduledIn(root, "check_load") {
		t.Errorf("check_load runs every 60 seconds and should be reported as scheduled")
	}
	if probeIsScheduledIn(root, "check_mdadm") {
		t.Errorf("check_mdadm is disabled and should not be reported as scheduled")
	}
	if probeIsScheduledIn(root, "check_ntp") {
		t.Errorf("check_ntp is not installed and should not be reported as scheduled")
	}
	if probeIsScheduledIn(root, "../../etc/passwd") {
		t.Errorf("An invalid name must never be reported as scheduled")
	}
}

func TestProbeIsScheduledAfterBeingRepitched(t *testing.T) {
	root := newTestProbesDirectory(t, "60/check_load")

	if err := scheduleProbeIn(root, "check_load", 900); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	// The 60 directory no longer holds it, but 900 does : the result must stay
	if !probeIsScheduledIn(root, "check_load") {
		t.Errorf("A repitched probe must still be reported as scheduled")
	}

	// Once disabled there is nothing left to run it
	if err := unscheduleProbeIn(root, "check_load"); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if probeIsScheduledIn(root, "check_load") {
		t.Errorf("A disabled probe must not be reported as scheduled")
	}
}

// A missing probes directory must not be read as "everything is scheduled"
func TestProbeIsScheduledWithoutProbesDirectory(t *testing.T) {
	if probeIsScheduledIn(filepath.Join(t.TempDir(), "gone"), "check_load") {
		t.Errorf("An unreadable probes directory must not report a probe as scheduled")
	}
}
