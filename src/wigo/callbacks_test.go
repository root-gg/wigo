package wigo

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeApprise installs a script standing in for the apprise binary. Every
// invocation writes its arguments to its own file so concurrent calls do not
// overwrite each other.
func fakeApprise(t *testing.T) (path string, calls func() []string) {
	t.Helper()

	directory := t.TempDir()
	script := filepath.Join(directory, "apprise")
	output := filepath.Join(directory, "calls")

	if err := os.Mkdir(output, 0755); err != nil {
		t.Fatalf("Fail to create the calls directory : %s", err)
	}

	content := "#!/bin/sh\necho \"$@\" > " + output + "/call-$$\n"
	if err := os.WriteFile(script, []byte(content), 0755); err != nil {
		t.Fatalf("Fail to write the fake apprise script : %s", err)
	}

	// Notifications are sent from goroutines, give them some time to land
	calls = func() []string {
		lines := make([]string, 0)

		for attempt := 0; attempt < 100; attempt++ {
			lines = lines[:0]

			files, err := os.ReadDir(output)
			if err != nil {
				t.Fatalf("Fail to list the calls directory : %s", err)
			}

			for _, file := range files {
				content, err := os.ReadFile(filepath.Join(output, file.Name()))
				if err != nil {
					continue
				}
				lines = append(lines, strings.TrimSpace(string(content)))
			}

			if len(lines) > 0 {
				return lines
			}

			time.Sleep(10 * time.Millisecond)
		}

		return lines
	}

	return script, calls
}

func TestSendApprise(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	script, calls := fakeApprise(t)

	notifications := wigo.GetConfig().Notifications
	notifications.AppriseEnabled = 1
	notifications.ApprisePath = script
	notifications.AppriseUrls = []string{"catchall://"}
	notifications.AppriseTargets = []AppriseTargetConfig{
		{Name: "dba team", Urls: []string{"dba://"}, Groups: []string{"databases"}},
		{Name: "web team", Urls: []string{"web://"}, Groups: []string{"frontend"}},
	}

	SendApprise(NewNotificationFromMessageForHost("Host db-1 DOWN", "db-1", "databases"))

	sent := calls()
	if len(sent) != 2 {
		t.Fatalf("Got %v, expected the catch all and the dba urls to be notified", sent)
	}

	urls := strings.Join(sent, "\n")
	if !strings.Contains(urls, "catchall://") || !strings.Contains(urls, "dba://") {
		t.Errorf("Got %v, expected catchall:// and dba://", sent)
	}
	if strings.Contains(urls, "web://") {
		t.Errorf("Got %v, the frontend target should not have been notified", sent)
	}

	// The message is the title and the summary is the body
	for _, call := range sent {
		if !strings.Contains(call, "-t Host db-1 DOWN") {
			t.Errorf("Got %q, expected the message as the title", call)
		}
	}
}

// Apprise refuses an empty title, the message is used as a fallback body.
func TestSendAppriseWithoutSummary(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	script, calls := fakeApprise(t)

	notifications := wigo.GetConfig().Notifications
	notifications.AppriseEnabled = 1
	notifications.ApprisePath = script
	notifications.AppriseUrls = []string{"catchall://"}

	SendApprise(NewNotificationFromMessageForHost("Host db-1 DOWN", "db-1", "databases"))

	sent := calls()
	if len(sent) != 1 {
		t.Fatalf("Got %v, expected one call", sent)
	}
	if !strings.Contains(sent[0], "-b Host db-1 DOWN") {
		t.Errorf("Got %q, expected the message to be used as the body", sent[0])
	}
}

func TestSendAppriseDisabled(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	script, calls := fakeApprise(t)

	notifications := wigo.GetConfig().Notifications
	notifications.AppriseEnabled = 0
	notifications.ApprisePath = script
	notifications.AppriseUrls = []string{"catchall://"}

	SendApprise(NewNotificationFromMessageForHost("Host db-1 DOWN", "db-1", "databases"))

	if sent := calls(); len(sent) != 0 {
		t.Errorf("Got %v, expected no call when apprise is disabled", sent)
	}
}

func TestSendAppriseWithoutMatchingTarget(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	script, calls := fakeApprise(t)

	notifications := wigo.GetConfig().Notifications
	notifications.AppriseEnabled = 1
	notifications.ApprisePath = script
	notifications.AppriseTargets = []AppriseTargetConfig{
		{Name: "web team", Urls: []string{"web://"}, Groups: []string{"frontend"}},
	}

	SendApprise(NewNotificationFromMessageForHost("Host db-1 DOWN", "db-1", "databases"))

	if sent := calls(); len(sent) != 0 {
		t.Errorf("Got %v, expected no call when no target matches", sent)
	}
}

// A probe notification is filtered on the group of the host that raised it.
func TestSendAppriseForAProbeNotification(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	script, calls := fakeApprise(t)

	notifications := wigo.GetConfig().Notifications
	notifications.AppriseEnabled = 1
	notifications.ApprisePath = script
	notifications.AppriseTargets = []AppriseTargetConfig{
		{Name: "dba team", Urls: []string{"dba://"}, Groups: []string{"databases"}},
	}

	host := wigo.GetLocalHost()
	notification := NewNotificationProbe(newTestProbe(host, "load", 100), newTestProbe(host, "load", 300))

	SendApprise(notification)

	sent := calls()
	if len(sent) != 1 || !strings.Contains(sent[0], "dba://") {
		t.Fatalf("Got %v, expected the dba url to be notified", sent)
	}
}

func TestCallbackHttp(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("Fail to parse the callback body : %s", err)
		}
		received <- r.PostFormValue("Notification")
	}))
	defer server.Close()

	wigo.GetConfig().Notifications.HttpUrl = server.URL

	notification := NewNotificationFromMessageForHost("Host db-1 DOWN", "db-1", "databases")
	payload, err := notification.ToJson()
	if err != nil {
		t.Fatalf("ToJson() returned an error : %s", err)
	}

	if err := CallbackHttp(string(payload)); err != nil {
		t.Fatalf("CallbackHttp() returned an error : %s", err)
	}

	select {
	case body := <-received:
		if !strings.Contains(body, "Host db-1 DOWN") {
			t.Errorf("Got %q, expected the notification payload", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("The callback has never been received")
	}
}

func TestCallbackHttpOnUnreachableUrl(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	// A server that is closed right away, so the port is free
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	wigo.GetConfig().Notifications.HttpUrl = url

	if err := CallbackHttp(`{"Message":"Host db-1 DOWN"}`); err == nil {
		t.Errorf("Expected an error when the callback url is unreachable")
	}
}

func TestNotificationSerialization(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	host := wigo.GetLocalHost()

	oldProbe := newTestProbe(host, "load", 100)
	newProbe := newTestProbe(host, "load", 300)

	notifications := []INotification{
		NewNotificationFromMessageForHost("Host db-1 DOWN", "db-1", "databases"),
		NewNotificationProbe(oldProbe, newProbe),
		&NotificationWigo{Notification: NewNotificationFromMessageForHost("wigo", "db-1", "databases")},
	}

	for _, notification := range notifications {
		payload, err := notification.ToJson()
		if err != nil {
			t.Fatalf("ToJson() returned an error : %s", err)
		}
		if !strings.Contains(string(payload), `"Group":"databases"`) {
			t.Errorf("Got %s, expected the group to be serialized", payload)
		}
		if notification.GetSummary() != "" && notification.GetMessage() == "" {
			t.Errorf("Got a summary without a message")
		}
	}
}
