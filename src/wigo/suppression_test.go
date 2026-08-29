package wigo

import (
	"testing"
	"time"
)

func ackOn(t *testing.T, target string, probe string, status int) {
	t.Helper()

	err := AddSuppression(Suppression{
		Kind: SuppressionAck, Scope: SuppressionScopeHost,
		Target: target, Probe: probe, Status: status,
		Author: "germain", CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("Fail to ack : %s", err)
	}
}

func silenceOn(t *testing.T, scope string, target string, probe string, seconds int64) {
	t.Helper()

	err := AddSuppression(Suppression{
		Kind: SuppressionSilence, Scope: scope,
		Target: target, Probe: probe,
		Author: "germain", CreatedAt: time.Now().Unix(),
		Until: time.Now().Unix() + seconds,
	})
	if err != nil {
		t.Fatalf("Fail to silence : %s", err)
	}
}

// The whole point : the notification stops, the monitoring does not.
func TestASilenceHoldsBackANotification(t *testing.T) {
	setupTestWigo(t, "databases")
	silenceOn(t, SuppressionScopeHost, "db1", "", 3600)

	if _, held := suppressionFor("db1", "databases", "check_load", 300); !held {
		t.Errorf("The notification should have been held back")
	}
	if _, held := suppressionFor("db2", "databases", "check_load", 300); held {
		t.Errorf("Another host should not have been silenced with it")
	}
}

// Acknowledging a WARNING must not swallow its turn to CRITICAL : what was
// acknowledged is not what is happening any more.
func TestAnAckDoesNotSwallowThingsGettingWorse(t *testing.T) {
	setupTestWigo(t, "databases")
	ackOn(t, "db1", "check_load", 250)

	if _, held := suppressionFor("db1", "databases", "check_load", 250); !held {
		t.Errorf("The acknowledged status should be held back")
	}
	if _, held := suppressionFor("db1", "databases", "check_load", 200); !held {
		t.Errorf("Something less bad should be held back too")
	}
	if _, held := suppressionFor("db1", "databases", "check_load", 300); held {
		t.Errorf("A worse status must get through, that is the point of an ack")
	}
}

// An ack left behind would silently hold back the next problem, and one taken
// at WARNING would go on covering a host critical for a week.
func TestAnAckIsClearedWhenTheSituationChanges(t *testing.T) {
	setupTestWigo(t, "databases")

	ackOn(t, "db1", "check_load", 300)
	clearAckOn("db1", "databases", "check_load", 100)

	if len(Suppressions()) != 0 {
		t.Errorf("Recovering should have cleared the ack")
	}

	ackOn(t, "db1", "check_load", 250)
	clearAckOn("db1", "databases", "check_load", 500)

	if len(Suppressions()) != 0 {
		t.Errorf("Getting worse should have cleared the ack")
	}

	// Still the same situation, still acknowledged
	ackOn(t, "db1", "check_load", 300)
	clearAckOn("db1", "databases", "check_load", 300)

	if len(Suppressions()) != 1 {
		t.Errorf("An unchanged situation should stay acknowledged")
	}
}

// A silence is a window, and a window that never closes is an unmonitored host.
func TestASilenceStopsApplyingWhenItRunsOut(t *testing.T) {
	setupTestWigo(t, "databases")
	silenceOn(t, SuppressionScopeHost, "db1", "", 3600)

	backdateSilence(t, "db1", time.Now().Unix()-1)

	if _, held := suppressionFor("db1", "databases", "check_load", 300); held {
		t.Errorf("An expired silence should not hold anything back")
	}

	// And it is deleted rather than listed for ever
	if len(Suppressions()) != 0 {
		t.Errorf("The expired silence should have been forgotten")
	}
}

func TestASilenceMustEndInTheFuture(t *testing.T) {
	setupTestWigo(t, "databases")

	err := AddSuppression(Suppression{
		Kind: SuppressionSilence, Scope: SuppressionScopeHost,
		Target: "db1", CreatedAt: time.Now().Unix(), Until: time.Now().Unix() - 1,
	})
	if err == nil {
		t.Errorf("A silence that is already over should be refused")
	}
}

// A silence on one probe is a finer statement than one on its whole host.
func TestTheMostSpecificSuppressionWins(t *testing.T) {
	setupTestWigo(t, "databases")

	silenceOn(t, SuppressionScopeGroup, "databases", "", 3600)
	silenceOn(t, SuppressionScopeHost, "db1", "", 3600)
	ackOn(t, "db1", "check_load", 500)

	suppression, held := suppressionFor("db1", "databases", "check_load", 300)
	if !held {
		t.Fatalf("It should have been held back")
	}
	if suppression.Probe != "check_load" {
		t.Errorf("Got %+v, expected the one about that very probe", suppression)
	}

	// Another probe of the same host falls back to the host wide silence
	suppression, held = suppressionFor("db1", "databases", "smart", 300)
	if !held || suppression.Scope != SuppressionScopeHost || suppression.Probe != "" {
		t.Errorf("Got %+v, expected the host wide silence", suppression)
	}

	// And another host of the group to the group wide one
	suppression, held = suppressionFor("db2", "databases", "smart", 300)
	if !held || suppression.Scope != SuppressionScopeGroup {
		t.Errorf("Got %+v, expected the group wide silence", suppression)
	}
}

// A group scoped suppression must not reach a host that belongs to no group,
// which would quietly cover everything nobody has sorted yet.
func TestAGroupSuppressionNeverCoversAHostWithoutAGroup(t *testing.T) {
	setupTestWigo(t, "databases")
	silenceOn(t, SuppressionScopeGroup, "databases", "", 3600)

	if _, held := suppressionFor("db1", "", "check_load", 300); held {
		t.Errorf("A host with no group should not be silenced by a group")
	}

	// Not even by one that somehow ended up with an empty target
	empty := Suppression{Kind: SuppressionSilence, Scope: SuppressionScopeGroup, Target: ""}
	if empty.Covers("db1", "", "check_load") {
		t.Errorf("An empty group should cover nothing")
	}

	// A target with no group of its own is refused outright
	if err := AddSuppression(Suppression{
		Kind: SuppressionSilence, Scope: SuppressionScopeGroup, Target: "",
		CreatedAt: time.Now().Unix(), Until: time.Now().Unix() + 3600,
	}); err == nil {
		t.Errorf("A suppression with nothing to be about should be refused")
	}
}

// Re-acknowledging means "this, now", not "this as well as what I said before".
func TestAddingReplacesTheOneOnTheSameTarget(t *testing.T) {
	setupTestWigo(t, "databases")

	ackOn(t, "db1", "check_load", 250)
	ackOn(t, "db1", "check_load", 300)

	suppressions := Suppressions()
	if len(suppressions) != 1 || suppressions[0].Status != 300 {
		t.Errorf("Got %+v, expected the second ack alone", suppressions)
	}
}

func TestRemoveSuppression(t *testing.T) {
	setupTestWigo(t, "databases")

	silenceOn(t, SuppressionScopeHost, "db1", "check_load", 3600)
	silenceOn(t, SuppressionScopeHost, "db1", "", 3600)

	if err := RemoveSuppression(SuppressionScopeHost, "db1", "check_load"); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	suppressions := Suppressions()
	if len(suppressions) != 1 || suppressions[0].Probe != "" {
		t.Errorf("Got %+v, expected only the host wide one left", suppressions)
	}
}

func TestAddSuppressionRefusesWhatItCannotStore(t *testing.T) {
	setupTestWigo(t, "databases")

	cases := []Suppression{
		{Kind: SuppressionAck, Scope: SuppressionScopeHost, Target: ""},
		{Kind: "mute", Scope: SuppressionScopeHost, Target: "db1"},
		{Kind: SuppressionAck, Scope: "fleet", Target: "db1"},
		{Kind: SuppressionAck, Scope: SuppressionScopeHost, Target: "db1", Probe: "../../etc/passwd"},
	}
	for _, suppression := range cases {
		if err := AddSuppression(suppression); err == nil {
			t.Errorf("%+v should have been refused", suppression)
		}
	}
	if len(Suppressions()) != 0 {
		t.Errorf("Nothing should have been stored")
	}
}

func TestParseSilenceDuration(t *testing.T) {
	if duration, err := parseSilenceDuration("2h"); err != nil || duration != 7200 {
		t.Errorf("Got %d %v, expected 7200", duration, err)
	}

	// Required, unlike a disable : a silence with no end is an unmonitored host
	for _, value := range []string{"", "30s", "-1h", "8760000h", "soon"} {
		if _, err := parseSilenceDuration(value); err == nil {
			t.Errorf("%q should be refused as a silence duration", value)
		}
	}
}

// The notification path is the only thing that matters here : held back means
// nothing reaches the callbacks channel.
func TestSendNotificationRespectsASilence(t *testing.T) {
	setupTestWigo(t, "databases")
	silenceOn(t, SuppressionScopeHost, "test-host", "", 3600)

	SendNotification(NewNotificationFromMessageForHost("Host test-host DOWN", "test-host", "databases").SetStatus(500))

	select {
	case notification := <-Channels.ChanCallbacks:
		t.Errorf("Got %q, expected nothing to be sent", notification.GetMessage())
	default:
	}
}

func TestSendNotificationLetsThroughWhatIsNotSuppressed(t *testing.T) {
	setupTestWigo(t, "databases")
	silenceOn(t, SuppressionScopeHost, "another-host", "", 3600)

	SendNotification(NewNotificationFromMessageForHost("Host test-host DOWN", "test-host", "databases").SetStatus(500))

	select {
	case notification := <-Channels.ChanCallbacks:
		if notification.GetMessage() != "Host test-host DOWN" {
			t.Errorf("Got %q", notification.GetMessage())
		}
	default:
		t.Errorf("The notification should have been sent")
	}
}

// A host coming back up is what clears the ack taken while it was down.
func TestSendNotificationClearsTheAckOnRecovery(t *testing.T) {
	setupTestWigo(t, "databases")
	ackOn(t, "test-host", "", 500)

	SendNotification(NewNotificationFromMessageForHost("Host test-host UP", "test-host", "databases").SetStatus(100))

	if len(Suppressions()) != 0 {
		t.Errorf("Coming back up should have cleared the ack")
	}
	select {
	case <-Channels.ChanCallbacks:
	default:
		t.Errorf("The recovery itself should have been sent")
	}
}

func backdateSilence(t *testing.T, target string, until int64) {
	t.Helper()

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	if _, err := LocalWigo.sqlLiteConn.Exec(
		`UPDATE suppressions SET until = ? WHERE target = ?;`, until, target); err != nil {
		t.Fatalf("Fail to backdate the silence : %s", err)
	}
}
