package wigo

import (
	"testing"
	"time"
)

// setupDisableTest points the wigo at a fresh probes directory and empties the
// table, so a test only ever sees what it put there itself.
func setupDisableTest(t *testing.T, probes ...string) string {
	t.Helper()

	setupTestWigo(t, "databases")
	root := newTestProbesDirectory(t, probes...)
	LocalWigo.config.Global.ProbesDirectory = root

	return root
}

// The record says who turned a probe off and why. It is metadata : the probe is
// disabled because nothing schedules it, not because a row says so.
func TestRecordAndReadADisable(t *testing.T) {
	setupDisableTest(t, "60/check_load")

	err := DisableProbeWithReason("check_load", "noisy while we migrate",
		"germain from 10.0.0.2", 0)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	records := ProbeDisableRecords()
	if len(records) != 1 {
		t.Fatalf("Got %+v, expected one record", records)
	}

	record := records[0]
	if record.Probe != "check_load" || record.Reason != "noisy while we migrate" {
		t.Errorf("Got %+v", record)
	}
	if record.Author != "germain from 10.0.0.2" {
		t.Errorf("Got author %q", record.Author)
	}
	// Captured before the probe stopped, otherwise there would be nothing left
	// to read it from
	if record.Interval != 60 {
		t.Errorf("Got interval %d, expected the 60 it was running at", record.Interval)
	}
	if record.Until != 0 {
		t.Errorf("Got until %d, expected no end date", record.Until)
	}
	if record.CreatedAt == 0 {
		t.Errorf("The record should be dated")
	}
}

// Most disabled probes were never turned off by anyone, they were simply never
// turned on. Attributing those to somebody would be a lie.
func TestOnlyDeliberateDisablesAreRecorded(t *testing.T) {
	root := setupDisableTest(t, "60/check_load")
	shipProbe(t, root, "hbase-master")

	if records := ProbeDisableRecords(); len(records) != 0 {
		t.Errorf("Got %+v, expected nothing to be attributed to anyone", records)
	}

	// hbase-master is disabled, and no more explained than before
	locations, err := probeLocationsIn(root)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(locations) != 2 {
		t.Fatalf("Got %+v", locations)
	}
}

// Enabling a probe makes whatever was said about it being off untrue.
func TestEnablingForgetsTheRecord(t *testing.T) {
	setupDisableTest(t, "60/check_load")

	if err := DisableProbeWithReason("check_load", "because", "someone", 0); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if err := ScheduleProbe("check_load", 300); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	// ScheduleProbe is the filesystem operation, the record is dropped by
	// whoever called it -- but reading must not report it either way
	if records := ProbeDisableRecords(); len(records) != 0 {
		t.Errorf("Got %+v, expected the record to be gone", records)
	}
}

// A probe put back by hand, by a package upgrade or by anything not going
// through the API leaves the record behind. The directory is what is true.
func TestARecordThatNoLongerDescribesRealityIsDropped(t *testing.T) {
	root := setupDisableTest(t, "60/check_load")

	if err := DisableProbeWithReason("check_load", "because", "someone", 0); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	// Straight to the filesystem, as an administrator would
	if err := scheduleProbeIn(root, "check_load", 60); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	if records := ProbeDisableRecords(); len(records) != 0 {
		t.Errorf("Got %+v, expected the stale record to be dropped", records)
	}

	// And dropped for good, not just hidden
	stored, err := readProbeDisableRecords()
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(stored) != 0 {
		t.Errorf("Got %+v, expected the row to be deleted", stored)
	}
}

// Disabling something for an afternoon must not leave it off for eight months.
func TestAnExpiredDisableBringsTheProbeBack(t *testing.T) {
	root := setupDisableTest(t, "300/check_load")

	if err := DisableProbeWithReason("check_load", "migration", "someone", 3600); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if probeIsScheduledIn(root, "check_load") {
		t.Fatalf("The probe should be disabled")
	}

	// Nothing happens while the deadline is ahead
	ExpireDisabledProbes()
	if probeIsScheduledIn(root, "check_load") {
		t.Errorf("The probe came back before its time")
	}

	backdateDisable(t, "check_load", time.Now().Unix()-1)

	ExpireDisabledProbes()

	if !probeIsIn(t, root, "300", "check_load") {
		t.Errorf("The probe should run every 300 seconds again, as it did")
	}
	if records := ProbeDisableRecords(); len(records) != 0 {
		t.Errorf("Got %+v, expected the record to be gone with the disable", records)
	}
}

// A probe brought back before the deadline must not be reported as still off.
func TestAnExpiredRecordOfAProbeThatRunsIsJustDropped(t *testing.T) {
	root := setupDisableTest(t, "60/check_load")

	if err := DisableProbeWithReason("check_load", "migration", "someone", 3600); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if err := scheduleProbeIn(root, "check_load", 120); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	backdateDisable(t, "check_load", time.Now().Unix()-1)
	ExpireDisabledProbes()

	// Put back at 120 by hand, and expiry must not drag it to the 60 it was at
	if !probeIsIn(t, root, "120", "check_load") || probeIsIn(t, root, "60", "check_load") {
		t.Errorf("Expiry should not have moved a probe that already runs")
	}
}

// A deadline needs somewhere to put the probe back to. Accepting one on a probe
// that was not running would promise a return that could never happen.
func TestADeadlineNeedsAnIntervalToReturnTo(t *testing.T) {
	root := setupDisableTest(t)
	shipProbe(t, root, "hbase-master")

	err := DisableProbeWithReason("hbase-master", "not applicable", "someone", 3600)
	if err == nil {
		t.Fatalf("A deadline on a probe that is not running should be refused")
	}

	// Without a deadline it is fine : it records why a probe nobody enabled is
	// deliberately left alone
	if err := DisableProbeWithReason("hbase-master", "no hbase here", "someone", 0); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	records := ProbeDisableRecords()
	if len(records) != 1 || records[0].Reason != "no hbase here" {
		t.Errorf("Got %+v", records)
	}
}

// The table can only ever start a probe. A row that cannot be acted on must not
// be retried every minute for ever either.
func TestAnExpiredRecordWithNoIntervalStopsAsking(t *testing.T) {
	root := setupDisableTest(t, "60/check_load")

	if err := DisableProbeWithReason("check_load", "because", "someone", 0); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	// A deadline on a record that has no interval to return to, which the API
	// refuses but a hand written row could hold
	forceDisableRecord(t, ProbeDisableRecord{
		Probe:     "check_load",
		Reason:    "because",
		Interval:  0,
		CreatedAt: time.Now().Unix() - 10,
		Until:     time.Now().Unix() - 1,
	})

	ExpireDisabledProbes()

	if probeIsScheduledIn(root, "check_load") {
		t.Errorf("Nothing should have been started")
	}

	stored, err := readProbeDisableRecords()
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(stored) != 1 || stored[0].Until != 0 {
		t.Errorf("Got %+v, expected the deadline to have been cleared", stored)
	}
}

// Disabling a probe twice replaces the note rather than failing on the key.
func TestDisablingTwiceReplacesTheRecord(t *testing.T) {
	setupDisableTest(t, "60/check_load")

	if err := DisableProbeWithReason("check_load", "first", "a", 0); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if err := DisableProbeWithReason("check_load", "second", "b", 0); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	records := ProbeDisableRecords()
	if len(records) != 1 || records[0].Reason != "second" || records[0].Author != "b" {
		t.Errorf("Got %+v", records)
	}
}

func TestParseDisableDuration(t *testing.T) {
	if duration, err := parseDisableDuration(""); err != nil || duration != 0 {
		t.Errorf("Got %d %v, expected no end date", duration, err)
	}
	if duration, err := parseDisableDuration("1h"); err != nil || duration != 3600 {
		t.Errorf("Got %d %v, expected 3600", duration, err)
	}

	// A few seconds is not a maintenance window, and a decade is not temporary
	for _, value := range []string{"30s", "0s", "-1h", "8760000h", "soon", "1"} {
		if _, err := parseDisableDuration(value); err == nil {
			t.Errorf("%q should be refused as a duration", value)
		}
	}
}

// backdateDisable moves a record's deadline into the past, which is the only
// way to test an expiry without waiting for one.
func backdateDisable(t *testing.T, probe string, until int64) {
	t.Helper()

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	if _, err := LocalWigo.sqlLiteConn.Exec(
		`UPDATE disabled_probes SET until = ? WHERE probe = ?;`, until, probe); err != nil {
		t.Fatalf("Fail to backdate the record : %s", err)
	}
}

func forceDisableRecord(t *testing.T, record ProbeDisableRecord) {
	t.Helper()

	if err := recordProbeDisabled(record); err != nil {
		t.Fatalf("Fail to write the record : %s", err)
	}
}
