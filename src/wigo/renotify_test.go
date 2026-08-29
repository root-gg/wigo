package wigo

import (
	"testing"
	"time"
)

func setupRenotifyTest(t *testing.T) {
	t.Helper()

	setupTestWigo(t, "databases")
	LocalWigo.config.Notifications.OnProbeChange = true
	LocalWigo.config.Notifications.MinLevelToSend = 250
	LocalWigo.config.Notifications.RenotifyInterval = 600
	LocalWigo.config.Notifications.FlapDetection = false

	lastNotified.Lock()
	lastNotified.at = make(map[string]int64)
	lastNotified.since = make(map[string]int64)
	lastNotified.Unlock()
}

func addProbe(t *testing.T, name string, status int) {
	t.Helper()

	result := NewProbeResult(name, status, 0, name+" message", "")
	result.SetHost(LocalWigo.GetLocalHost())
	LocalWigo.GetLocalHost().Probes.Set(name, result)
}

func drainNotifications() []INotification {
	sent := make([]INotification, 0)
	for {
		select {
		case notification := <-Channels.ChanCallbacks:
			sent = append(sent, notification)
		default:
			return sent
		}
	}
}

// Six hours of silence look exactly like six hours of everything being fine.
func TestAProblemIsSaidAgain(t *testing.T) {
	setupRenotifyTest(t)
	addProbe(t, "check_load", 300)

	// Said once when it broke
	recordNotified("test-host", "check_load", time.Now().Unix())
	renotifyOpenProblems()
	if sent := drainNotifications(); len(sent) != 0 {
		t.Errorf("Got %d, expected nothing before the interval is up", len(sent))
	}

	backdateNotified(t, "test-host", "check_load", time.Now().Unix()-601)
	renotifyOpenProblems()

	sent := drainNotifications()
	if len(sent) != 1 {
		t.Fatalf("Got %d notifications, expected the problem to be said again", len(sent))
	}
	if sent[0].GetProbe() != "check_load" || sent[0].GetStatus() != 300 {
		t.Errorf("Got %+v", sent[0])
	}
}

// Nothing is said that a status change would not have said.
func TestWhatIsFineIsNotSaidAgain(t *testing.T) {
	setupRenotifyTest(t)
	addProbe(t, "check_load", 100)
	addProbe(t, "smart", 200)

	renotifyOpenProblems()

	if sent := drainNotifications(); len(sent) != 0 {
		t.Errorf("Got %+v, expected nothing below MinLevelToSend", sent)
	}
}

// Acknowledging something is precisely how you tell it to stop repeating.
func TestAnAckStopsTheRepeats(t *testing.T) {
	setupRenotifyTest(t)
	addProbe(t, "check_load", 300)
	ackOn(t, "test-host", "check_load", 300)

	backdateNotified(t, "test-host", "check_load", time.Now().Unix()-601)
	renotifyOpenProblems()

	if sent := drainNotifications(); len(sent) != 0 {
		t.Errorf("Got %+v, expected the ack to hold the repeat back", sent)
	}
}

// Saying the same unsteady thing every ten minutes is the same noise, slower.
func TestAFlappingProbeIsNotRepeatedAbout(t *testing.T) {
	setupRenotifyTest(t)
	LocalWigo.config.Notifications.FlapDetection = true
	LocalWigo.config.Notifications.FlapThreshold = 2
	LocalWigo.config.Notifications.FlapWindow = 3600

	flapping.Lock()
	flapping.transitions = make(map[string][]int64)
	flapping.since = make(map[string]int64)
	flapping.Unlock()

	addProbe(t, "check_load", 300)
	RecordStatusChange("test-host", "check_load")
	RecordStatusChange("test-host", "check_load")

	backdateNotified(t, "test-host", "check_load", time.Now().Unix()-601)
	renotifyOpenProblems()

	if sent := drainNotifications(); len(sent) != 0 {
		t.Errorf("Got %+v, expected a flapping probe not to be repeated about", sent)
	}
}

// The dangerous version of quiet hours drops notifications. This one delays
// them : nothing is recorded as sent, so it goes out when the window closes.
func TestQuietHoursDelayRatherThanDrop(t *testing.T) {
	setupRenotifyTest(t)
	addProbe(t, "check_load", 300)

	// A window covering the whole day, so the test does not depend on the clock
	LocalWigo.config.Notifications.QuietHoursFrom = "00:00"
	LocalWigo.config.Notifications.QuietHoursTo = "23:59"

	SendNotification(NewNotificationFromMessageForHost("broken", "test-host", "databases").SetStatus(300))

	if sent := drainNotifications(); len(sent) != 0 {
		t.Fatalf("Got %+v, expected it to be held", sent)
	}
	if at, _ := notifiedState("test-host", ""); at != 0 {
		t.Errorf("A held notification must not be recorded as sent, or it would wait a full interval afterwards")
	}

	// Morning comes
	LocalWigo.config.Notifications.QuietHoursFrom = ""
	LocalWigo.config.Notifications.QuietHoursTo = ""

	renotifyOpenProblems()
	if sent := drainNotifications(); len(sent) != 1 {
		t.Errorf("Got %d, expected the held problem to be said once the window closed", len(sent))
	}
}

// Some things are worth being woken for, and the operator says which.
func TestQuietHoursLetTheWorstThrough(t *testing.T) {
	setupRenotifyTest(t)
	LocalWigo.config.Notifications.QuietHoursFrom = "00:00"
	LocalWigo.config.Notifications.QuietHoursTo = "23:59"
	LocalWigo.config.Notifications.QuietHoursMinLevelToSend = 500

	SendNotification(NewNotificationFromMessageForHost("bad", "test-host", "databases").SetStatus(300))
	if sent := drainNotifications(); len(sent) != 0 {
		t.Errorf("Got %+v, expected a merely critical one to wait", sent)
	}

	SendNotification(NewNotificationFromMessageForHost("worse", "test-host", "databases").SetStatus(500))
	if sent := drainNotifications(); len(sent) != 1 {
		t.Errorf("Got %d, expected the worst to get through", len(sent))
	}
}

func TestQuietHoursWindow(t *testing.T) {
	setupRenotifyTest(t)

	// A window that crosses midnight, which is the usual shape
	LocalWigo.config.Notifications.QuietHoursFrom = "22:00"
	LocalWigo.config.Notifications.QuietHoursTo = "08:00"

	quiet := []string{"22:00", "23:30", "00:00", "03:00", "07:59"}
	loud := []string{"08:00", "12:00", "21:59"}

	for _, at := range quiet {
		if !inQuietHours(atTime(t, at)) {
			t.Errorf("%s should be inside the window", at)
		}
	}
	for _, at := range loud {
		if inQuietHours(atTime(t, at)) {
			t.Errorf("%s should be outside the window", at)
		}
	}

	// And one that does not cross it
	LocalWigo.config.Notifications.QuietHoursFrom = "09:00"
	LocalWigo.config.Notifications.QuietHoursTo = "17:00"
	if !inQuietHours(atTime(t, "12:00")) || inQuietHours(atTime(t, "18:00")) {
		t.Errorf("A window inside the day should work too")
	}

	// A malformed or empty one is simply off, rather than quiet for ever
	for _, from := range []string{"", "nope", "25:00", "22"} {
		LocalWigo.config.Notifications.QuietHoursFrom = from
		LocalWigo.config.Notifications.QuietHoursTo = "08:00"
		if inQuietHours(atTime(t, "23:00")) {
			t.Errorf("%q should turn quiet hours off, not on", from)
		}
	}
}

// Escalation counts from when the problem appeared, not from the last repeat.
func TestEscalationAfterAProblemGoesUnattended(t *testing.T) {
	setupRenotifyTest(t)
	LocalWigo.config.Notifications.EscalateAfter = 1800
	addProbe(t, "check_load", 300)

	now := time.Now().Unix()
	recordNotified("test-host", "check_load", now)
	backdateNotified(t, "test-host", "check_load", now-601)

	renotifyOpenProblems()
	sent := drainNotifications()
	if len(sent) != 1 {
		t.Fatalf("Got %d, expected one repeat", len(sent))
	}
	if notificationIsEscalated(sent[0]) {
		t.Errorf("Ten minutes in is too early to wake anybody else")
	}

	backdateSince(t, "test-host", "check_load", now-3600)
	backdateNotified(t, "test-host", "check_load", now-601)

	renotifyOpenProblems()
	sent = drainNotifications()
	if len(sent) != 1 || !notificationIsEscalated(sent[0]) {
		t.Errorf("Got %+v, expected it to have escalated after an hour", sent)
	}
}

// Escalation targets stay out of the normal path : being second in line means
// not being woken first.
func TestEscalationTargetsAreOnlyUsedWhenEscalating(t *testing.T) {
	config := new(NotificationConfig)
	config.AppriseTargets = []AppriseTargetConfig{
		{Name: "oncall", Urls: []string{"mailto://oncall"}, Hosts: []string{"*"}},
		{Name: "manager", Urls: []string{"mailto://manager"}, Hosts: []string{"*"}, Escalation: true},
	}

	normal := config.GetAppriseUrls("db1", "databases", nil, false)
	if len(normal) != 1 || normal[0].Url != "mailto://oncall" {
		t.Errorf("Got %+v, expected only the first line", normal)
	}

	escalated := config.GetAppriseUrls("db1", "databases", nil, true)
	if len(escalated) != 2 {
		t.Errorf("Got %+v, expected both", escalated)
	}
}

// A recovery must not leave the clock running, or the next problem would look
// as though it had been open for hours the moment it appeared.
func TestRecoveringForgetsHowLongItWasBroken(t *testing.T) {
	setupRenotifyTest(t)
	addProbe(t, "check_load", 300)

	SendNotification(NewNotificationFromMessageForHost("broken", "test-host", "databases").SetStatus(300))
	drainNotifications()
	if at, _ := notifiedState("test-host", ""); at == 0 {
		t.Fatalf("It should have been recorded")
	}

	SendNotification(NewNotificationFromMessageForHost("fixed", "test-host", "databases").SetStatus(100))
	drainNotifications()

	at, since := notifiedState("test-host", "")
	if at != 0 || since != 0 {
		t.Errorf("Got %d %d, expected a clean slate after recovery", at, since)
	}
}

// A problem whose first notification was held has never been recorded, so it is
// overdue rather than not yet due.
func TestAProblemNeverNotifiedIsOverdue(t *testing.T) {
	setupRenotifyTest(t)
	addProbe(t, "check_load", 300)

	renotifyOpenProblems()

	if sent := drainNotifications(); len(sent) != 1 {
		t.Errorf("Got %d, expected a problem nobody has been told about to be said", len(sent))
	}
}

func TestRenotifyIsOffByDefault(t *testing.T) {
	setupRenotifyTest(t)
	LocalWigo.config.Notifications.RenotifyInterval = 0
	addProbe(t, "check_load", 300)

	renotifyOpenProblems()

	if sent := drainNotifications(); len(sent) != 0 {
		t.Errorf("Got %+v, expected nothing without an interval", sent)
	}
}

func atTime(t *testing.T, hourMinute string) time.Time {
	t.Helper()

	parsed, err := time.Parse("15:04", hourMinute)
	if err != nil {
		t.Fatalf("Fail to parse %q : %s", hourMinute, err)
	}

	return parsed
}

func backdateNotified(t *testing.T, hostname string, probe string, at int64) {
	t.Helper()

	lastNotified.Lock()
	defer lastNotified.Unlock()

	lastNotified.at[notifyKey(hostname, probe)] = at
	if _, known := lastNotified.since[notifyKey(hostname, probe)]; !known {
		lastNotified.since[notifyKey(hostname, probe)] = at
	}
}

func backdateSince(t *testing.T, hostname string, probe string, at int64) {
	t.Helper()

	lastNotified.Lock()
	defer lastNotified.Unlock()

	lastNotified.since[notifyKey(hostname, probe)] = at
}
