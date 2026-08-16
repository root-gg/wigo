package wigo

import (
	"testing"
	"time"
)

func TestNewLog(t *testing.T) {

	setupTestWigo(t, "databases")

	entry := NewLog(CRITICAL, "everything is on fire")

	if entry.Level != CRITICAL {
		t.Errorf("Level = %d, expected CRITICAL", entry.Level)
	}
	if entry.Message != "everything is on fire" {
		t.Errorf("Message = %s", entry.Message)
	}
	// The log defaults to the local host identity
	if entry.Host != "test-host" || entry.Group != "databases" {
		t.Errorf("Got host %q and group %q, expected the local host ones", entry.Host, entry.Group)
	}
	if entry.Probe != "" {
		t.Errorf("Probe = %s, expected no probe", entry.Probe)
	}
	if entry.Timestamp == 0 || entry.Date == "" {
		t.Errorf("The log has not been dated")
	}
}

func TestLogSetters(t *testing.T) {

	setupTestWigo(t, "databases")

	entry := NewLog(INFO, "message")
	entry.SetHost("db-2")
	entry.SetGroup("frontend")

	if entry.Host != "db-2" {
		t.Errorf("Host = %s, expected db-2", entry.Host)
	}
	if entry.Group != "frontend" {
		t.Errorf("Group = %s, expected frontend", entry.Group)
	}
}

// Logs are written asynchronously, so this checks the row actually lands in the
// database.
func TestWigoAddLogForAProbe(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	host := wigo.GetLocalHost()
	probe := newTestProbe(host, "load", 300)

	if err := wigo.AddLog(probe, INFO, "load too high"); err != nil {
		t.Fatalf("AddLog() returned an error : %s", err)
	}

	row := waitForLog(t, "load too high")

	if row.probe != "load" {
		t.Errorf("Probe = %s, expected load", row.probe)
	}
	if row.host != "test-host" || row.group != "databases" {
		t.Errorf("Got host %q and group %q", row.host, row.group)
	}
	// The level is derived from the probe status, whatever the one given
	if row.level != CRITICAL {
		t.Errorf("Level = %d, expected CRITICAL for a probe in status 300", row.level)
	}
}

func TestWigoAddLogForAWigo(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	wigo.GlobalStatus = 200

	if err := wigo.AddLog(wigo, INFO, "wigo is degraded"); err != nil {
		t.Fatalf("AddLog() returned an error : %s", err)
	}

	row := waitForLog(t, "wigo is degraded")

	if row.host != "test-host" || row.group != "databases" {
		t.Errorf("Got host %q and group %q", row.host, row.group)
	}
	if row.level != WARNING {
		t.Errorf("Level = %d, expected WARNING for a wigo in status 200", row.level)
	}
}

func TestWigoSearchLogs(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	host := wigo.GetLocalHost()

	if err := wigo.AddLog(newTestProbe(host, "searchable", 300), INFO, "searchable probe log"); err != nil {
		t.Fatalf("AddLog() returned an error : %s", err)
	}
	waitForLog(t, "searchable probe log")

	logs := wigo.SearchLogs("searchable", "", "", 10, 0)
	if len(logs) != 1 {
		t.Fatalf("Got %d logs, expected the searchable one", len(logs))
	}
	if logs[0].Message != "searchable probe log" {
		t.Errorf("Message = %s", logs[0].Message)
	}
	// Dates are persisted as unix timestamps and rendered back
	if logs[0].Timestamp == 0 || logs[0].Date == "" {
		t.Errorf("Got timestamp %d and date %q, expected the stored date", logs[0].Timestamp, logs[0].Date)
	}

	// Every filter is applied
	if logs := wigo.SearchLogs("searchable", "test-host", "databases", 10, 0); len(logs) != 1 {
		t.Errorf("Got %d logs, expected the searchable one", len(logs))
	}
	if logs := wigo.SearchLogs("does-not-exist", "", "", 10, 0); len(logs) != 0 {
		t.Errorf("Got %d logs, expected none for an unknown probe", len(logs))
	}
	if logs := wigo.SearchLogs("searchable", "unknown-host", "", 10, 0); len(logs) != 0 {
		t.Errorf("Got %d logs, expected none for an unknown host", len(logs))
	}
	if logs := wigo.SearchLogs("searchable", "", "unknown-group", 10, 0); len(logs) != 0 {
		t.Errorf("Got %d logs, expected none for an unknown group", len(logs))
	}
}

type logRow struct {
	level   uint8
	group   string
	host    string
	probe   string
	message string
}

// waitForLog polls the database until the asynchronous write shows up.
func waitForLog(t *testing.T, message string) (row logRow) {
	t.Helper()

	for attempt := 0; attempt < 100; attempt++ {
		LocalWigo.sqlLiteLock.Lock()
		err := LocalWigo.sqlLiteConn.
			QueryRow(`SELECT level, grp, host, probe, message FROM logs WHERE message = ?`, message).
			Scan(&row.level, &row.group, &row.host, &row.probe, &row.message)
		LocalWigo.sqlLiteLock.Unlock()

		if err == nil {
			return row
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("The log %q has never been persisted", message)

	return
}
