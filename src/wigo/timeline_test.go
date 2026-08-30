package wigo

import (
	"testing"
	"time"
)

func setupTimelineTest(t *testing.T) {
	t.Helper()

	setupTestWigo(t, "databases")
	LocalWigo.config.Global.StatusHistoryDays = 30

	// Where each host was last seen is remembered for a whole process, so one
	// test would otherwise decide what the next one is allowed to record.
	forgetHostStatuses()
}

func changeAt(t *testing.T, host string, probe string, was int, now int, at int64) {
	t.Helper()

	RecordStatusTransition(StatusChange{
		Host: host, Probe: probe, Group: "databases",
		Was: was, Now: now, Message: "test", At: at,
	})
}

// The whole reason this exists : the logs table only carries the sentence, and
// building a timeline by parsing it would break the day somebody rewords it.
func TestATransitionIsKeptAsTwoStatuses(t *testing.T) {
	setupTimelineTest(t)

	now := time.Now().Unix()
	changeAt(t, "db1", "check_load", 100, 300, now-600)

	timeline, err := ProbeTimeline("db1", "check_load", now-3600, now)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(timeline.Changes) != 1 {
		t.Fatalf("Got %+v", timeline.Changes)
	}

	change := timeline.Changes[0]
	if change.Was != 100 || change.Now != 300 {
		t.Errorf("Got %d -> %d, expected 100 -> 300", change.Was, change.Now)
	}
	if change.Message != "test" || change.Group != "databases" {
		t.Errorf("Got %+v", change)
	}
}

// A host that has been critical for a week has no transition inside the last
// day. A timeline that started blank would draw that week as nothing at all.
func TestTheStatusWhenTheWindowOpensIsReported(t *testing.T) {
	setupTimelineTest(t)

	now := time.Now().Unix()
	changeAt(t, "db1", "check_load", 100, 300, now-86400*7)

	timeline, err := ProbeTimeline("db1", "check_load", now-3600, now)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(timeline.Changes) != 0 {
		t.Errorf("Got %+v, expected nothing inside the window", timeline.Changes)
	}
	if timeline.StatusAtStart != 300 {
		t.Errorf("Got %d, expected the window to open on the critical it was already at",
			timeline.StatusAtStart)
	}
}

// Never having been seen is not the same as having been fine.
func TestSomethingNeverSeenOpensAsAbsent(t *testing.T) {
	setupTimelineTest(t)

	now := time.Now().Unix()
	timeline, err := ProbeTimeline("db1", "check_load", now-3600, now)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if timeline.StatusAtStart != StatusAbsent {
		t.Errorf("Got %d, expected absent", timeline.StatusAtStart)
	}
	if len(timeline.Changes) != 0 {
		t.Errorf("Got %+v", timeline.Changes)
	}
}

// The last one before the window, not the first one : two changes before it and
// the window opens on the second.
func TestTheWindowOpensOnTheMostRecentEarlierChange(t *testing.T) {
	setupTimelineTest(t)

	now := time.Now().Unix()
	changeAt(t, "db1", "check_load", 100, 300, now-7200)
	changeAt(t, "db1", "check_load", 300, 200, now-5400)

	timeline, err := ProbeTimeline("db1", "check_load", now-3600, now)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if timeline.StatusAtStart != 200 {
		t.Errorf("Got %d, expected 200", timeline.StatusAtStart)
	}
}

// A host going up or down belongs to the host, not to any probe : it is what a
// master sees and the host itself cannot record.
func TestAHostTimelineIsSeparateFromItsProbes(t *testing.T) {
	setupTimelineTest(t)

	now := time.Now().Unix()
	changeAt(t, "db1", "", 100, 500, now-600)
	changeAt(t, "db1", "check_load", 100, 300, now-500)

	host, err := ProbeTimeline("db1", "", now-3600, now)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(host.Changes) != 1 || host.Changes[0].Now != 500 {
		t.Errorf("Got %+v, expected only the host change", host.Changes)
	}

	probe, err := ProbeTimeline("db1", "check_load", now-3600, now)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(probe.Changes) != 1 || probe.Changes[0].Now != 300 {
		t.Errorf("Got %+v, expected only the probe change", probe.Changes)
	}
}

func TestChangesComeBackInOrder(t *testing.T) {
	setupTimelineTest(t)

	now := time.Now().Unix()
	changeAt(t, "db1", "check_load", 300, 100, now-100)
	changeAt(t, "db1", "check_load", 100, 300, now-300)
	changeAt(t, "db1", "check_load", 200, 100, now-200)

	timeline, err := ProbeTimeline("db1", "check_load", now-3600, now)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(timeline.Changes) != 3 {
		t.Fatalf("Got %+v", timeline.Changes)
	}
	for i := 1; i < len(timeline.Changes); i++ {
		if timeline.Changes[i].At < timeline.Changes[i-1].At {
			t.Errorf("Got them out of order : %+v", timeline.Changes)
		}
	}
}

func TestAgedStatusChangesAreDropped(t *testing.T) {
	setupTimelineTest(t)
	LocalWigo.config.Global.StatusHistoryDays = 1

	now := time.Now().Unix()
	changeAt(t, "db1", "check_load", 100, 300, now-86400*3)
	changeAt(t, "db1", "check_load", 300, 100, now-600)

	dropAgedStatusChanges()

	timeline, err := ProbeTimeline("db1", "check_load", now-86400*7, now)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(timeline.Changes) != 1 || timeline.Changes[0].Now != 100 {
		t.Errorf("Got %+v, expected only the recent one", timeline.Changes)
	}
}

// An empty timeline on a host that keeps nothing is not the same as one on a
// host where nothing happened, so the retention travels with the answer.
func TestNothingIsKeptWhenTheHistoryIsOff(t *testing.T) {
	setupTimelineTest(t)
	LocalWigo.config.Global.StatusHistoryDays = 0

	now := time.Now().Unix()
	changeAt(t, "db1", "check_load", 100, 300, now-600)

	timeline, err := ProbeTimeline("db1", "check_load", now-3600, now)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(timeline.Changes) != 0 {
		t.Errorf("Got %+v, expected nothing recorded", timeline.Changes)
	}
	if timeline.HistoryDays != 0 {
		t.Errorf("The answer has to say it keeps nothing")
	}
}

func TestProbeTimelineRefusesWhatItCannotAnswer(t *testing.T) {
	setupTimelineTest(t)

	now := time.Now().Unix()

	if _, err := ProbeTimeline("", "check_load", now-3600, now); err == nil {
		t.Errorf("A timeline needs a host")
	}
	if _, err := ProbeTimeline("db1", "../../etc/passwd", now-3600, now); err == nil {
		t.Errorf("A traversing probe name should be refused")
	}
	if _, err := ProbeTimeline("db1", "check_load", now, now-3600); err == nil {
		t.Errorf("A window that ends before it starts should be refused")
	}
}

// A local wigo does not notify about its own probes appearing, so the arrival
// has to be written down where the result lands. Otherwise a probe that has
// been fine since it was installed has no row at all, and its whole band reads
// as never watched.
func TestALocalProbeArrivingStartsItsBand(t *testing.T) {
	setupTimelineTest(t)

	now := time.Now().Unix()
	host := LocalWigo.GetLocalHost()
	host.AddOrUpdateProbe(newTestProbe(nil, "check_load", 100))

	timeline, err := ProbeTimeline(LocalWigo.GetHostname(), "check_load", now-3600, now+60)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(timeline.Changes) != 1 {
		t.Fatalf("Got %+v, expected its arrival", timeline.Changes)
	}
	if timeline.Changes[0].Was != StatusAbsent || timeline.Changes[0].Now != 100 {
		t.Errorf("Got %+v, expected it to have arrived at 100", timeline.Changes[0])
	}

	// And updating it with the same status writes nothing more : the band is
	// already running, only a change ends it.
	host.AddOrUpdateProbe(newTestProbe(host, "check_load", 100))

	timeline, err = ProbeTimeline(LocalWigo.GetHostname(), "check_load", now-3600, now+60)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(timeline.Changes) != 1 {
		t.Errorf("Got %+v, expected nothing to have been added", timeline.Changes)
	}
}

// A probe appearing already critical has no transition of its own. Without
// recording its arrival, a timeline would draw it as nothing.
func TestAProbeAppearingIsRecorded(t *testing.T) {
	setupTimelineTest(t)

	now := time.Now().Unix()
	changeAt(t, "db1", "check_load", StatusAbsent, 300, now-600)

	timeline, err := ProbeTimeline("db1", "check_load", now-3600, now)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(timeline.Changes) != 1 || timeline.Changes[0].Was != StatusAbsent {
		t.Errorf("Got %+v", timeline.Changes)
	}
	if timeline.Changes[0].Now != 300 {
		t.Errorf("Got %d, expected it to have appeared critical", timeline.Changes[0].Now)
	}
}

// A host that never stopped answering had nothing at all on its band, and the
// screen said "nothing known over that window" about a machine it had been
// watching for a month. Its timeline is its health, and health is the worst of
// its probes rather than whether it replied.
func TestAHostsOwnStatusIsRecorded(t *testing.T) {
	setupTimelineTest(t)

	now := time.Now().Unix()
	local := LocalWigo.GetLocalHost()

	// It starts fine, then one probe turns critical
	local.AddOrUpdateProbe(newTestProbe(local, "check_load", 100))
	local.AddOrUpdateProbe(newTestProbe(local, "check_ntp", 300))

	timeline, err := ProbeTimeline(LocalWigo.GetHostname(), "", now-3600, now+60)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(timeline.Changes) == 0 {
		t.Fatalf("The host's own status has to reach its timeline")
	}

	last := timeline.Changes[len(timeline.Changes)-1]
	if last.Now != 300 {
		t.Errorf("Got %d, expected the host to be worth its worst probe", last.Now)
	}
}

// Down and Up own reachability : recording the same moment from both would put
// two transitions at the same instant.
func TestReachabilityIsLeftToDownAndUp(t *testing.T) {
	setupTimelineTest(t)

	const down = 999

	RecordHostStatusChange("db1", "databases", down, 100, "back")
	RecordHostStatusChange("db1", "databases", 100, down, "gone")

	now := time.Now().Unix()
	timeline, err := ProbeTimeline("db1", "", now-3600, now+60)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(timeline.Changes) != 0 {
		t.Errorf("Got %+v, expected the sentinel to be left alone", timeline.Changes)
	}

	// And a real change is still written
	RecordHostStatusChange("db1", "databases", 100, 300, "a probe went critical")

	timeline, err = ProbeTimeline("db1", "", now-3600, now+60)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(timeline.Changes) != 1 || timeline.Changes[0].Now != 300 {
		t.Errorf("Got %+v", timeline.Changes)
	}
}

// Writing a row every time a probe reports the same thing would fill the table
// with a status that never changed.
func TestAStatusThatDidNotChangeIsNotWritten(t *testing.T) {
	setupTimelineTest(t)

	RecordHostStatusChange("db1", "databases", 100, 100, "still fine")

	now := time.Now().Unix()
	timeline, err := ProbeTimeline("db1", "", now-3600, now+60)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(timeline.Changes) != 0 {
		t.Errorf("Got %+v, expected nothing", timeline.Changes)
	}
}

// A master of masters sees the hosts two levels down only inside their trees,
// and those never pass through AddOrUpdateRemoteWigo. Hooking that alone left
// them with an empty band, which is the shape of the bug this fixes.
func TestEveryHostOfTheTreeGetsItsOwnBand(t *testing.T) {
	setupTimelineTest(t)
	forgetHostStatuses()

	local := setupTestWigo(t, "databases")
	LocalWigo.config.Global.StatusHistoryDays = 30

	middle := newTestRemoteWigo("uuid-middle", "middle", "frontend")
	middle.LocalHost.Status = 100

	deep := newTestRemoteWigo("uuid-deep", "two-levels-down", "frontend")
	deep.LocalHost.Status = 300
	middle.RemoteWigos.Set("uuid-deep", deep)

	local.RemoteWigos.Set("uuid-middle", middle)
	RecordHostStatuses(middle)

	now := time.Now().Unix()
	for name, expected := range map[string]int{"middle": 100, "two-levels-down": 300} {
		timeline, err := ProbeTimeline(name, "", now-3600, now+60)
		if err != nil {
			t.Fatalf("%s : unexpected error %s", name, err)
		}
		if len(timeline.Changes) != 1 {
			t.Fatalf("%s : got %+v, expected its band to have started", name, timeline.Changes)
		}
		if timeline.Changes[0].Was != StatusAbsent || timeline.Changes[0].Now != expected {
			t.Errorf("%s : got %+v", name, timeline.Changes[0])
		}
	}

	// Seen again unchanged, nothing more is written
	RecordHostStatuses(middle)

	timeline, err := ProbeTimeline("two-levels-down", "", now-3600, now+60)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(timeline.Changes) != 1 {
		t.Errorf("Got %+v, expected nothing to have been added", timeline.Changes)
	}

	// And a real change is
	deep.LocalHost.Status = 100
	RecordHostStatuses(middle)

	timeline, err = ProbeTimeline("two-levels-down", "", now-3600, now+60)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(timeline.Changes) != 2 || timeline.Changes[1].Now != 100 {
		t.Errorf("Got %+v", timeline.Changes)
	}
}

func forgetHostStatuses() {
	lastHostStatus.Lock()
	lastHostStatus.at = make(map[string]int)
	lastHostStatus.Unlock()
}

// A restart used to write a fresh "never watched" transition for every host,
// cutting a band that never stopped and putting a gap on the screen where
// nothing had happened.
func TestARestartDoesNotStartTheBandAgain(t *testing.T) {
	setupTimelineTest(t)

	remote := newTestRemoteWigo("uuid-1", "db1", "databases")
	remote.LocalHost.Status = 100

	RecordHostStatuses(remote)

	// The process goes away and comes back : the memory of where each host
	// stood goes with it, the database does not.
	forgetHostStatuses()
	RecordHostStatuses(remote)

	now := time.Now().Unix()
	timeline, err := ProbeTimeline("db1", "", now-3600, now+60)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(timeline.Changes) != 1 {
		t.Errorf("Got %+v, expected the band not to have been started twice", timeline.Changes)
	}

	// And a change across the restart is still written, once
	forgetHostStatuses()
	remote.LocalHost.Status = 300
	RecordHostStatuses(remote)

	timeline, err = ProbeTimeline("db1", "", now-3600, now+60)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(timeline.Changes) != 2 || timeline.Changes[1].Was != 100 || timeline.Changes[1].Now != 300 {
		t.Errorf("Got %+v", timeline.Changes)
	}
}

// Zeroing the field and building it back up in place let another goroutine read
// the nought in between, and nought is ERROR on this scale -- it reached a
// timeline as a real transition to 0. The value is built aside and assigned
// once now, so a reader sees the old one or the new one.
//
// The concurrent case is deliberately not exercised here : Host.Status is a
// plain field with no lock, so a test that reads it while it is written is a
// data race of its own and the detector would fail the suite for the wrong
// reason. What is checked is that the function never has a reason to hold a
// nought.
func TestRecomputingAStatusNeverSettlesOnANought(t *testing.T) {
	setupTimelineTest(t)

	host := NewHost()
	host.Name = "db1"

	// No probe at all : not an error, and not a nought
	host.RecomputeStatus()
	if host.Status != 100 {
		t.Errorf("Got %d, expected a host with no probe to be fine", host.Status)
	}

	host.Probes.Set("check_load", newTestProbe(host, "check_load", 300))
	host.Probes.Set("check_ntp", newTestProbe(host, "check_ntp", 100))
	host.RecomputeStatus()
	if host.Status != 300 {
		t.Errorf("Got %d, expected the worst of its probes", host.Status)
	}

	// And back down when the bad one goes away
	host.Probes.Remove("check_load")
	host.RecomputeStatus()
	if host.Status != 100 {
		t.Errorf("Got %d, expected it to follow what is left", host.Status)
	}
}
