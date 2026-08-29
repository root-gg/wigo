package wigo

import (
	"testing"
	"time"
)

func setupTimelineTest(t *testing.T) {
	t.Helper()

	setupTestWigo(t, "databases")
	LocalWigo.config.Global.StatusHistoryDays = 30
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
