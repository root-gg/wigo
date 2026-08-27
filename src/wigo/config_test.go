package wigo

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAppriseTargetMatches(t *testing.T) {

	tests := []struct {
		name     string
		target   AppriseTargetConfig
		hostname string
		group    string
		expected bool
	}{
		{
			name:     "group match",
			target:   AppriseTargetConfig{Groups: []string{"databases"}},
			hostname: "db-1",
			group:    "databases",
			expected: true,
		},
		{
			name:     "group match is case insensitive",
			target:   AppriseTargetConfig{Groups: []string{"DataBases"}},
			hostname: "db-1",
			group:    "databases",
			expected: true,
		},
		{
			name:     "group mismatch",
			target:   AppriseTargetConfig{Groups: []string{"databases"}},
			hostname: "web-1",
			group:    "frontend",
			expected: false,
		},
		{
			name:     "host match",
			target:   AppriseTargetConfig{Hosts: []string{"db-1"}},
			hostname: "db-1",
			group:    "frontend",
			expected: true,
		},
		{
			name:     "host match is case insensitive",
			target:   AppriseTargetConfig{Hosts: []string{"DB-1"}},
			hostname: "db-1",
			group:    "",
			expected: true,
		},
		{
			name:     "groups and hosts are or'ed",
			target:   AppriseTargetConfig{Groups: []string{"databases"}, Hosts: []string{"web-1"}},
			hostname: "web-1",
			group:    "frontend",
			expected: true,
		},
		{
			name:     "group wildcard matches everything",
			target:   AppriseTargetConfig{Groups: []string{"*"}},
			hostname: "web-1",
			group:    "frontend",
			expected: true,
		},
		{
			name:     "host wildcard matches an unknown group",
			target:   AppriseTargetConfig{Hosts: []string{"*"}},
			hostname: "web-1",
			group:    "",
			expected: true,
		},
		{
			name:     "empty group does not match a filter",
			target:   AppriseTargetConfig{Groups: []string{"databases"}},
			hostname: "db-1",
			group:    "",
			expected: false,
		},
		{
			name:     "empty hostname does not match a filter",
			target:   AppriseTargetConfig{Hosts: []string{"db-1"}},
			hostname: "",
			group:    "databases",
			expected: false,
		},
		{
			name:     "target without any filter never matches",
			target:   AppriseTargetConfig{Urls: []string{"noop://"}},
			hostname: "db-1",
			group:    "databases",
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.target.Matches(test.hostname, test.group); got != test.expected {
				t.Errorf("Matches(%q, %q) = %v, expected %v", test.hostname, test.group, got, test.expected)
			}
		})
	}
}

func TestAppriseTargetGetName(t *testing.T) {

	named := AppriseTargetConfig{Name: "dba team"}
	if got := named.GetName(0); got != `"dba team"` {
		t.Errorf("GetName() = %s, expected \"dba team\" between quotes", got)
	}

	// Without a name the target falls back on its position in the file
	unnamed := AppriseTargetConfig{}
	if got := unnamed.GetName(2); got != "#3" {
		t.Errorf("GetName() = %s, expected #3", got)
	}
}

func TestAppriseUrlOrigin(t *testing.T) {

	fromTarget := AppriseUrl{Url: "dba://", Target: `"dba team"`}
	if got := fromTarget.Origin(); got != ` (target "dba team")` {
		t.Errorf("Origin() = %q", got)
	}

	// Urls coming from the plain AppriseUrls list have no target
	catchAll := AppriseUrl{Url: "catchall://"}
	if got := catchAll.Origin(); got != "" {
		t.Errorf("Origin() = %q, expected an empty string", got)
	}
}

func TestGetAppriseUrls(t *testing.T) {

	config := &NotificationConfig{
		AppriseUrls: []string{"catchall://"},
		AppriseTargets: []AppriseTargetConfig{
			{
				Name:   "dba team",
				Urls:   []string{"dba://", "catchall://", ""},
				Groups: []string{"databases"},
			},
			{
				Urls:  []string{"onduty://"},
				Hosts: []string{"db-1"},
			},
			{
				Name:   "web team",
				Urls:   []string{"web://"},
				Groups: []string{"frontend", "backend"},
			},
		},
	}

	tests := []struct {
		name     string
		hostname string
		group    string
		expected []AppriseUrl
	}{
		{
			name:     "a matching group and host cumulate their targets",
			hostname: "db-1",
			group:    "databases",
			expected: []AppriseUrl{
				{Url: "catchall://"},
				{Url: "dba://", Target: `"dba team"`},
				{Url: "onduty://", Target: "#2"},
			},
		},
		{
			name:     "only the host matches",
			hostname: "db-1",
			group:    "none",
			expected: []AppriseUrl{
				{Url: "catchall://"},
				{Url: "onduty://", Target: "#2"},
			},
		},
		{
			name:     "second group of a target",
			hostname: "web-1",
			group:    "backend",
			expected: []AppriseUrl{
				{Url: "catchall://"},
				{Url: "web://", Target: `"web team"`},
			},
		},
		{
			name:     "nothing matches but the catch all list is still notified",
			hostname: "unknown",
			group:    "unknown",
			expected: []AppriseUrl{
				{Url: "catchall://"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := config.GetAppriseUrls(test.hostname, test.group, false)
			if !reflect.DeepEqual(got, test.expected) {
				t.Errorf("GetAppriseUrls(%q, %q) =\n%v\nexpected\n%v", test.hostname, test.group, got, test.expected)
			}
		})
	}
}

// An url listed in both the catch all list and a matching target must only be
// notified once, keeping the first origin it was seen with.
func TestGetAppriseUrlsDeduplication(t *testing.T) {

	config := &NotificationConfig{
		AppriseUrls: []string{"shared://"},
		AppriseTargets: []AppriseTargetConfig{
			{Name: "first", Urls: []string{"shared://", "first://"}, Groups: []string{"prod"}},
			{Name: "second", Urls: []string{"first://", "second://"}, Groups: []string{"prod"}},
		},
	}

	expected := []AppriseUrl{
		{Url: "shared://"},
		{Url: "first://", Target: `"first"`},
		{Url: "second://", Target: `"second"`},
	}

	if got := config.GetAppriseUrls("web-1", "prod", false); !reflect.DeepEqual(got, expected) {
		t.Errorf("GetAppriseUrls() =\n%v\nexpected\n%v", got, expected)
	}
}

func TestGetAppriseUrlsWithoutAnyConfiguration(t *testing.T) {

	config := &NotificationConfig{}

	if got := config.GetAppriseUrls("db-1", "databases", false); len(got) != 0 {
		t.Errorf("GetAppriseUrls() = %v, expected no url", got)
	}
}

// Apprise targets are declared as an array of tables and must survive a real
// configuration file round trip.
func TestNewConfigAppriseTargets(t *testing.T) {

	content := `
[Notifications]
AppriseEnabled = 1
ApprisePath = "/opt/apprise"
AppriseUrls = ["catchall://"]

[[Notifications.AppriseTargets]]
Name = "dba team"
Urls = ["dba://"]
Groups = ["databases"]
Hosts = ["db-master"]

[[Notifications.AppriseTargets]]
Urls = ["web://"]
Groups = ["frontend", "backend"]
`

	configFile := filepath.Join(t.TempDir(), "wigo.conf")
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("Fail to write the test configuration file : %s", err)
	}

	config := NewConfig(configFile)

	notifications := config.Notifications
	if notifications.AppriseEnabled != 1 {
		t.Errorf("AppriseEnabled = %d, expected 1", notifications.AppriseEnabled)
	}
	if notifications.ApprisePath != "/opt/apprise" {
		t.Errorf("ApprisePath = %s", notifications.ApprisePath)
	}

	if len(notifications.AppriseTargets) != 2 {
		t.Fatalf("Got %d apprise targets, expected 2", len(notifications.AppriseTargets))
	}

	first := notifications.AppriseTargets[0]
	if first.Name != "dba team" {
		t.Errorf("First target name = %s", first.Name)
	}
	if !reflect.DeepEqual(first.Groups, []string{"databases"}) {
		t.Errorf("First target groups = %v", first.Groups)
	}
	if !reflect.DeepEqual(first.Hosts, []string{"db-master"}) {
		t.Errorf("First target hosts = %v", first.Hosts)
	}

	second := notifications.AppriseTargets[1]
	if !reflect.DeepEqual(second.Groups, []string{"frontend", "backend"}) {
		t.Errorf("Second target groups = %v", second.Groups)
	}

	expected := []AppriseUrl{
		{Url: "catchall://"},
		{Url: "dba://", Target: `"dba team"`},
	}
	if got := notifications.GetAppriseUrls("db-master", "none", false); !reflect.DeepEqual(got, expected) {
		t.Errorf("GetAppriseUrls() =\n%v\nexpected\n%v", got, expected)
	}
}

// The old flat RemoteWigos list is still supported and converted into the
// advanced one, taking the http port as a default.
func TestNewConfigLegacyRemoteWigosList(t *testing.T) {

	content := `
[Http]
Port = 4000

[RemoteWigos]
List = ["db-1", "db-2:4002"]
`

	configFile := filepath.Join(t.TempDir(), "wigo.conf")
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("Fail to write the test configuration file : %s", err)
	}

	config := NewConfig(configFile)

	advanced := config.RemoteWigos.AdvancedList
	if len(advanced) != 2 {
		t.Fatalf("Got %d remote wigos, expected 2", len(advanced))
	}

	if advanced[0].Hostname != "db-1" || advanced[0].Port != 4000 {
		t.Errorf("Got %s:%d, expected db-1 on the default http port", advanced[0].Hostname, advanced[0].Port)
	}
	if advanced[1].Hostname != "db-2" || advanced[1].Port != 4002 {
		t.Errorf("Got %s:%d, expected db-2:4002", advanced[1].Hostname, advanced[1].Port)
	}

	// The temporary list used during the conversion is emptied
	if config.AdvancedList != nil {
		t.Errorf("AdvancedList = %v, expected it to be cleared", config.AdvancedList)
	}
}

func TestNewConfigAdvancedRemoteWigosList(t *testing.T) {

	content := `
[[AdvancedList]]
Hostname = "db-1"
Port = 4002
SslEnabled = true
Login = "wigo"
Password = "secret"
CheckRemotesDepth = 2
`

	configFile := filepath.Join(t.TempDir(), "wigo.conf")
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("Fail to write the test configuration file : %s", err)
	}

	config := NewConfig(configFile)

	advanced := config.RemoteWigos.AdvancedList
	if len(advanced) != 1 {
		t.Fatalf("Got %d remote wigos, expected 1", len(advanced))
	}
	if advanced[0].Hostname != "db-1" || advanced[0].Port != 4002 {
		t.Errorf("Got %s:%d, expected db-1:4002", advanced[0].Hostname, advanced[0].Port)
	}
	if !advanced[0].SslEnabled || advanced[0].Login != "wigo" || advanced[0].Password != "secret" {
		t.Errorf("Got %+v, expected the ssl and credentials settings", advanced[0])
	}
	if advanced[0].CheckRemotesDepth != 2 {
		t.Errorf("CheckRemotesDepth = %d, expected 2", advanced[0].CheckRemotesDepth)
	}
}

func TestNewConfigDefaults(t *testing.T) {

	config := NewConfig(filepath.Join(t.TempDir(), "does-not-exist.conf"))

	if config.Global.Group != "none" {
		t.Errorf("Group = %s, expected none", config.Global.Group)
	}
	if config.Global.AliveTimeout != 60 {
		t.Errorf("AliveTimeout = %d, expected 60", config.Global.AliveTimeout)
	}
	if !config.Http.Enabled || config.Http.Port != 4000 {
		t.Errorf("Got enabled %v on port %d, expected the http server on 4000", config.Http.Enabled, config.Http.Port)
	}
	if config.PushServer.Enabled || config.PushClient.Enabled {
		t.Errorf("The push server and client must be disabled by default")
	}
	if config.Notifications.MinLevelToSend != 101 {
		t.Errorf("MinLevelToSend = %d, expected 101", config.Notifications.MinLevelToSend)
	}
	if config.Notifications.OnHostChange || config.Notifications.OnProbeChange {
		t.Errorf("Notifications must be disabled by default")
	}

	// The probes configuration directory is exported for the probes to read
	if os.Getenv("WIGO_PROBE_CONFIG_ROOT") != config.Global.ProbesConfigDirectory {
		t.Errorf("WIGO_PROBE_CONFIG_ROOT = %s, expected %s", os.Getenv("WIGO_PROBE_CONFIG_ROOT"), config.Global.ProbesConfigDirectory)
	}
}

// Both gates that let something change a host must stay closed until an
// administrator opens them, so upgrading an existing install never widens what
// it exposes.
func TestNewConfigWriteActionsAreClosedByDefault(t *testing.T) {

	config := NewConfig(filepath.Join(t.TempDir(), "does-not-exist.conf"))

	if config.Http.AllowWriteActions {
		t.Errorf("Http.AllowWriteActions must be off by default")
	}
	if config.PushClient.AllowRemoteControl {
		t.Errorf("PushClient.AllowRemoteControl must be off by default")
	}
}

// A configuration file that predates these options must keep them closed.
func TestNewConfigWriteActionsStayClosedOnAnOldConfigFile(t *testing.T) {

	path := filepath.Join(t.TempDir(), "wigo.conf")
	old := `
[Global]
Hostname = "legacy"

[Http]
Enabled = true
Port = 4000
Login = "admin"
Password = "secret"

[PushClient]
Enabled = true
Address = "master.domain.tld"
PushInterval = 10
`
	if err := os.WriteFile(path, []byte(old), 0644); err != nil {
		t.Fatalf("Fail to write the configuration file : %s", err)
	}

	config := NewConfig(path)

	if config.Global.Hostname != "legacy" {
		t.Fatalf("Hostname = %s, the file has not been read", config.Global.Hostname)
	}
	if config.Http.AllowWriteActions {
		t.Errorf("An old configuration file must not enable Http.AllowWriteActions")
	}
	if config.PushClient.AllowRemoteControl {
		t.Errorf("An old configuration file must not enable PushClient.AllowRemoteControl")
	}
}

// Without any apprise configuration the defaults must keep apprise disabled.
func TestNewConfigAppriseDefaults(t *testing.T) {

	config := NewConfig(filepath.Join(t.TempDir(), "does-not-exist.conf"))

	if config.Notifications.AppriseEnabled != 0 {
		t.Errorf("AppriseEnabled = %d, expected apprise to be disabled by default", config.Notifications.AppriseEnabled)
	}
	if config.Notifications.ApprisePath != "/usr/local/bin/apprise" {
		t.Errorf("ApprisePath = %s", config.Notifications.ApprisePath)
	}
	if config.Notifications.AppriseTargets != nil {
		t.Errorf("AppriseTargets = %v, expected no target", config.Notifications.AppriseTargets)
	}
}
