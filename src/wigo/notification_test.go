package wigo

import (
	"database/sql"
	"log"
	"os"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

// Logs are persisted asynchronously through the LocalWigo global, so it is
// installed once for the whole test binary instead of being swapped by each
// test, which would let a pending log write dereference a nil wigo.
func TestMain(m *testing.M) {

	config := new(Config)
	config.Global = new(GeneralConfig)
	config.Notifications = new(NotificationConfig)
	config.OpenTSDB = new(OpenTSDBConfig)

	LocalWigo = new(Wigo)
	LocalWigo.config = config
	LocalWigo.Uuid = "00000000-0000-4000-8000-000000000000"
	LocalWigo.Version = "test"
	LocalWigo.Hostname = "test-host"
	LocalWigo.IsAlive = true
	LocalWigo.locker = new(sync.RWMutex)
	LocalWigo.LocalHost = NewHost()
	LocalWigo.LocalHost.Name = "test-host"
	LocalWigo.LocalHost.SetParentWigo(LocalWigo)
	LocalWigo.RemoteWigos = NewConcurrentMapWigos()
	LocalWigo.disabledProbes = make(map[string]bool)
	LocalWigo.disabledProbesLock = new(sync.RWMutex)

	// Give the asynchronous log writes a real database to work on
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		log.Fatalf("Fail to open the in memory database : %s", err)
	}
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS logs (id integer not null primary key, date timestamp, level int, grp text, host text, probe text, message text)`); err != nil {
		log.Fatalf("Fail to create the logs table : %s", err)
	}
	if _, err := conn.Exec(createDisabledProbesTable); err != nil {
		log.Fatalf("Fail to create the disabled probes table : %s", err)
	}
	if _, err := conn.Exec(createSuppressionsTable); err != nil {
		log.Fatalf("Fail to create the suppressions table : %s", err)
	}
	if _, err := conn.Exec(createApiTokensTable); err != nil {
		log.Fatalf("Fail to create the api tokens table : %s", err)
	}
	if _, err := conn.Exec(createMetricsTable); err != nil {
		log.Fatalf("Fail to create the metrics table : %s", err)
	}
	if _, err := conn.Exec(createStatusChangesTable); err != nil {
		log.Fatalf("Fail to create the status changes table : %s", err)
	}
	LocalWigo.sqlLiteLock = new(sync.Mutex)
	LocalWigo.sqlLiteConn = conn

	os.Exit(m.Run())
}

// setupTestWigo resets the shared LocalWigo for a single test. Callbacks are
// collected in a buffered channel instead of the unbuffered one used in
// production, so sending a notification never blocks.
func setupTestWigo(t *testing.T, group string) *Wigo {
	t.Helper()

	// Every section a real configuration has : anything reading one that was
	// left nil here would panic in a test while working perfectly in
	// production, which says nothing useful.
	config := new(Config)
	config.Global = new(GeneralConfig)
	config.Http = new(HttpConfig)
	config.PushServer = new(PushServerConfig)
	config.PushClient = new(PushClientConfig)
	config.RemoteWigos = new(RemoteWigoConfig)
	config.Notifications = new(NotificationConfig)
	config.Notifications.MinLevelToSend = 250
	config.OpenTSDB = new(OpenTSDBConfig)
	LocalWigo.config = config

	LocalWigo.GlobalStatus = 0
	LocalWigo.GlobalMessage = ""
	LocalWigo.IsAlive = true
	LocalWigo.Hostname = "test-host"
	LocalWigo.LocalHost = NewHost()
	LocalWigo.LocalHost.Name = "test-host"
	LocalWigo.LocalHost.Group = group
	LocalWigo.LocalHost.SetParentWigo(LocalWigo)
	LocalWigo.RemoteWigos = NewConcurrentMapWigos()
	LocalWigo.disabledProbes = make(map[string]bool)
	LocalWigo.disabledProbesLock = new(sync.RWMutex)
	LocalWigo.push = nil

	Channels = new(Chans)
	Channels.ChanCallbacks = make(chan INotification, 100)

	// The database is shared by the whole test binary, start from a clean slate
	LocalWigo.sqlLiteLock.Lock()
	if _, err := LocalWigo.sqlLiteConn.Exec(`DELETE FROM logs`); err != nil {
		t.Fatalf("Fail to clean the logs table : %s", err)
	}
	if _, err := LocalWigo.sqlLiteConn.Exec(`DELETE FROM disabled_probes`); err != nil {
		t.Fatalf("Fail to clean the disabled probes table : %s", err)
	}
	if _, err := LocalWigo.sqlLiteConn.Exec(`DELETE FROM suppressions`); err != nil {
		t.Fatalf("Fail to clean the suppressions table : %s", err)
	}
	if _, err := LocalWigo.sqlLiteConn.Exec(`DELETE FROM api_tokens`); err != nil {
		t.Fatalf("Fail to clean the api tokens table : %s", err)
	}
	if _, err := LocalWigo.sqlLiteConn.Exec(`DELETE FROM metrics`); err != nil {
		t.Fatalf("Fail to clean the metrics table : %s", err)
	}
	if _, err := LocalWigo.sqlLiteConn.Exec(`DELETE FROM status_changes`); err != nil {
		t.Fatalf("Fail to clean the status changes table : %s", err)
	}
	LocalWigo.sqlLiteLock.Unlock()

	return LocalWigo
}

// newTestRemoteWigo builds a remote wigo as it would be received from the
// network, ready to be added to the local one.
func newTestRemoteWigo(uuid string, hostname string, group string) *Wigo {
	this := new(Wigo)
	this.Uuid = uuid
	this.Version = "test"
	this.Hostname = hostname
	this.IsAlive = true
	this.locker = new(sync.RWMutex)
	this.LocalHost = NewHost()
	this.LocalHost.Name = hostname
	this.LocalHost.Group = group
	this.LocalHost.SetParentWigo(this)
	this.RemoteWigos = NewConcurrentMapWigos()
	this.disabledProbes = make(map[string]bool)
	this.disabledProbesLock = new(sync.RWMutex)

	return this
}

func newTestProbe(host *Host, name string, status int) *ProbeResult {
	probe := new(ProbeResult)
	probe.Name = name
	probe.Status = status
	probe.Message = name + " message"
	probe.SetHost(host)

	return probe
}

func TestNewNotificationFromMessageForHost(t *testing.T) {

	notification := NewNotificationFromMessageForHost("Host db-1 DOWN", "db-1", "databases")

	if notification.GetMessage() != "Host db-1 DOWN" {
		t.Errorf("GetMessage() = %s", notification.GetMessage())
	}
	if notification.GetHostname() != "db-1" {
		t.Errorf("GetHostname() = %s, expected db-1", notification.GetHostname())
	}
	if notification.GetGroup() != "databases" {
		t.Errorf("GetGroup() = %s, expected databases", notification.GetGroup())
	}
}

// The hostname and the group are what apprise targets are filtered on, they
// have to be set whatever the kind of probe notification.
func TestNewNotificationProbeSetsHostAndGroup(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	host := wigo.GetLocalHost()

	tests := []struct {
		name     string
		oldProbe *ProbeResult
		newProbe *ProbeResult
	}{
		{
			name:     "new probe",
			newProbe: newTestProbe(host, "probe.new", 100),
		},
		{
			name:     "deleted probe",
			oldProbe: newTestProbe(host, "probe.deleted", 100),
		},
		{
			name:     "status change",
			oldProbe: newTestProbe(host, "probe.changed", 100),
			newProbe: newTestProbe(host, "probe.changed", 300),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			notification := NewNotificationProbe(test.oldProbe, test.newProbe)

			if notification.GetHostname() != "test-host" {
				t.Errorf("GetHostname() = %s, expected test-host", notification.GetHostname())
			}
			if notification.GetGroup() != "databases" {
				t.Errorf("GetGroup() = %s, expected databases", notification.GetGroup())
			}
		})
	}
}

// A probe keeping the same status is not a change, nothing has to be filled in.
func TestNewNotificationProbeWithoutStatusChange(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	host := wigo.GetLocalHost()

	notification := NewNotificationProbe(newTestProbe(host, "probe", 100), newTestProbe(host, "probe", 100))

	if notification.GetMessage() != "" {
		t.Errorf("GetMessage() = %s, expected no message", notification.GetMessage())
	}
	if len(Channels.ChanCallbacks) != 0 {
		t.Errorf("A notification has been sent for an unchanged probe")
	}
}

func TestNewNotificationProbeSending(t *testing.T) {

	tests := []struct {
		name          string
		onProbeChange bool
		oldStatus     int
		newStatus     int
		expected      bool
	}{
		{
			name:          "notifications disabled",
			onProbeChange: false,
			oldStatus:     100,
			newStatus:     300,
			expected:      false,
		},
		{
			name:          "down above the minimum level",
			onProbeChange: true,
			oldStatus:     100,
			newStatus:     300,
			expected:      true,
		},
		{
			name:          "down below the minimum level",
			onProbeChange: true,
			oldStatus:     100,
			newStatus:     200,
			expected:      false,
		},
		{
			name:          "up from a status above the minimum level",
			onProbeChange: true,
			oldStatus:     300,
			newStatus:     100,
			expected:      true,
		},
		{
			name:          "up from a status below the minimum level",
			onProbeChange: true,
			oldStatus:     200,
			newStatus:     100,
			expected:      false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wigo := setupTestWigo(t, "databases")
			wigo.GetConfig().Notifications.OnProbeChange = test.onProbeChange
			host := wigo.GetLocalHost()

			NewNotificationProbe(newTestProbe(host, "probe", test.oldStatus), newTestProbe(host, "probe", test.newStatus))

			sent := len(Channels.ChanCallbacks) == 1
			if sent != test.expected {
				t.Errorf("Notification sent = %v, expected %v", sent, test.expected)
			}
		})
	}
}

func TestWigoGetGroup(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	if wigo.GetGroup() != "databases" {
		t.Errorf("GetGroup() = %s, expected databases", wigo.GetGroup())
	}

	// Remote wigos that never answered have no local host yet
	unknown := new(Wigo)
	if unknown.GetGroup() != "" {
		t.Errorf("GetGroup() = %s, expected an empty string", unknown.GetGroup())
	}
}
