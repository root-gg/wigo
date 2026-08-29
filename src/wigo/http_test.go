package wigo

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestHttpRemotesHandler(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	wigo.RemoteWigos.Set("db-2", newTestRemoteWigo("uuid-2", "db-2", "databases"))

	// Without a hostname the handler lists the known wigos
	code, body := HttpRemotesHandler(nil, testRequest(t))
	if code != 200 {
		t.Fatalf("Code = %d, expected 200", code)
	}

	names := make([]string, 0)
	if err := json.Unmarshal([]byte(body), &names); err != nil {
		t.Fatalf("The body is not valid json : %s", err)
	}
	if !IsStringInArray("db-2", names) {
		t.Errorf("Got %v, expected it to contain db-2", names)
	}

	// With a hostname it returns the whole wigo
	code, body = HttpRemotesHandler(nil, testRequest(t, "hostname", "db-2"))
	if code != 200 {
		t.Fatalf("Code = %d, expected 200", code)
	}
	decoded, err := NewWigoFromJson([]byte(body), 0)
	if err != nil {
		t.Fatalf("The body is not a valid wigo : %s", err)
	}
	if decoded.GetHostname() != "db-2" {
		t.Errorf("Hostname = %s, expected db-2", decoded.GetHostname())
	}

	if code, _ := HttpRemotesHandler(nil, testRequest(t, "hostname", "unknown")); code != 404 {
		t.Errorf("Code = %d, expected 404 for an unknown host", code)
	}
}

func TestHttpRemotesProbesHandler(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	remote := newTestRemoteWigo("uuid-2", "db-2", "databases")
	remote.LocalHost.Probes.Set("load", newTestProbe(remote.LocalHost, "load", 300))
	wigo.RemoteWigos.Set("db-2", remote)

	// Probes list
	code, body := HttpRemotesProbesHandler(nil, testRequest(t, "hostname", "db-2"))
	if code != 200 {
		t.Fatalf("Code = %d, expected 200", code)
	}
	probes := make([]string, 0)
	if err := json.Unmarshal([]byte(body), &probes); err != nil {
		t.Fatalf("The body is not valid json : %s", err)
	}
	if len(probes) != 1 || probes[0] != "load" {
		t.Errorf("Got %v, expected the load probe", probes)
	}

	// A single probe
	code, body = HttpRemotesProbesHandler(nil, testRequest(t, "hostname", "db-2", "probe", "load"))
	if code != 200 {
		t.Fatalf("Code = %d, expected 200", code)
	}
	probe := new(ProbeResult)
	if err := json.Unmarshal([]byte(body), probe); err != nil {
		t.Fatalf("The body is not a valid probe : %s", err)
	}
	if probe.Status != 300 {
		t.Errorf("Status = %d, expected 300", probe.Status)
	}

	if code, _ := HttpRemotesProbesHandler(nil, testRequest(t)); code != 404 {
		t.Errorf("Code = %d, expected 404 without a hostname", code)
	}
	if code, _ := HttpRemotesProbesHandler(nil, testRequest(t, "hostname", "unknown")); code != 404 {
		t.Errorf("Code = %d, expected 404 for an unknown host", code)
	}
}

func TestHttpRemotesStatusHandler(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	remote := newTestRemoteWigo("uuid-2", "db-2", "databases")
	remote.GlobalStatus = 300
	wigo.RemoteWigos.Set("db-2", remote)

	code, body := HttpRemotesStatusHandler(nil, testRequest(t, "hostname", "db-2"))
	if code != 200 || body != "300" {
		t.Errorf("Got code %d and body %q, expected 200 and 300", code, body)
	}

	if code, _ := HttpRemotesStatusHandler(nil, testRequest(t)); code != 404 {
		t.Errorf("Code = %d, expected 404 without a hostname", code)
	}
	if code, _ := HttpRemotesStatusHandler(nil, testRequest(t, "hostname", "unknown")); code != 404 {
		t.Errorf("Code = %d, expected 404 for an unknown host", code)
	}
}

func TestHttpRemotesProbesStatusHandler(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	remote := newTestRemoteWigo("uuid-2", "db-2", "databases")
	remote.LocalHost.Probes.Set("load", newTestProbe(remote.LocalHost, "load", 250))
	wigo.RemoteWigos.Set("db-2", remote)

	code, body := HttpRemotesProbesStatusHandler(nil, testRequest(t, "hostname", "db-2", "probe", "load"))
	if code != 200 || body != "250" {
		t.Errorf("Got code %d and body %q, expected 200 and 250", code, body)
	}

	if code, _ := HttpRemotesProbesStatusHandler(nil, testRequest(t, "hostname", "db-2")); code != 404 {
		t.Errorf("Code = %d, expected 404 without a probe name", code)
	}
	if code, _ := HttpRemotesProbesStatusHandler(nil, testRequest(t, "hostname", "db-2", "probe", "unknown")); code != 404 {
		t.Errorf("Code = %d, expected 404 for an unknown probe", code)
	}
}

func TestHttpGroupsHandler(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	remote := newTestRemoteWigo("uuid-2", "web-1", "frontend")
	remote.GlobalStatus = 300
	wigo.RemoteWigos.Set("uuid-2", remote)

	// Groups list
	code, body := HttpGroupsHandler(nil, testRequest(t))
	if code != 200 {
		t.Fatalf("Code = %d, expected 200", code)
	}
	groups := make([]string, 0)
	if err := json.Unmarshal([]byte(body), &groups); err != nil {
		t.Fatalf("The body is not valid json : %s", err)
	}
	if !IsStringInArray("databases", groups) || !IsStringInArray("frontend", groups) {
		t.Errorf("Got %v, expected databases and frontend", groups)
	}

	// Group summary
	code, body = HttpGroupsHandler(nil, testRequest(t, "group", "frontend"))
	if code != 200 {
		t.Fatalf("Code = %d, expected 200", code)
	}

	summary := make(map[string]interface{})
	if err := json.Unmarshal([]byte(body), &summary); err != nil {
		t.Fatalf("The body is not valid json : %s", err)
	}
	if summary["Name"] != "frontend" {
		t.Errorf("Name = %v, expected frontend", summary["Name"])
	}
	if summary["Status"] != float64(300) {
		t.Errorf("Status = %v, expected 300", summary["Status"])
	}
	if hosts, ok := summary["Hosts"].([]interface{}); !ok || len(hosts) != 1 {
		t.Errorf("Got %v, expected a single host in the group", summary["Hosts"])
	}
}

func TestHttpLogsHandler(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	host := wigo.GetLocalHost()

	if err := wigo.AddLog(newTestProbe(host, "http-logs", 300), INFO, "http logs test"); err != nil {
		t.Fatalf("AddLog() returned an error : %s", err)
	}
	waitForLog(t, "http logs test")

	request, err := http.NewRequest("GET", "http://localhost/api/logs?probe=http-logs&limit=10", nil)
	if err != nil {
		t.Fatalf("Fail to build the request : %s", err)
	}

	code, body := HttpLogsHandler(nil, request)
	if code != 200 {
		t.Fatalf("Code = %d, expected 200", code)
	}

	logs := make([]*Log, 0)
	if err := json.Unmarshal([]byte(body), &logs); err != nil {
		t.Fatalf("The body is not valid json : %s", err)
	}
	if len(logs) != 1 || logs[0].Message != "http logs test" {
		t.Fatalf("Got %v, expected the log we just wrote", logs)
	}
}

func TestHttpLogsHandlerWithUnknownHost(t *testing.T) {

	setupTestWigo(t, "databases")

	request, err := http.NewRequest("GET", "http://localhost/api/logs?hostname=unknown", nil)
	if err != nil {
		t.Fatalf("Fail to build the request : %s", err)
	}

	if code, _ := HttpLogsHandler(nil, request); code != 404 {
		t.Errorf("Code = %d, expected 404 for an unknown host", code)
	}
}

func TestHttpLogsIndexesHandler(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	host := wigo.GetLocalHost()

	if err := wigo.AddLog(newTestProbe(host, "indexed", 300), INFO, "indexes test"); err != nil {
		t.Fatalf("AddLog() returned an error : %s", err)
	}
	waitForLog(t, "indexes test")

	code, body := HttpLogsIndexesHandler(nil, testRequest(t))
	if code != 200 {
		t.Fatalf("Code = %d, expected 200", code)
	}

	indexes := make(map[string][]string)
	if err := json.Unmarshal([]byte(body), &indexes); err != nil {
		t.Fatalf("The body is not valid json : %s", err)
	}
	if !IsStringInArray("indexed", indexes["probes"]) {
		t.Errorf("Got %v, expected the indexed probe", indexes["probes"])
	}
	if !IsStringInArray("test-host", indexes["hosts"]) {
		t.Errorf("Got %v, expected the test host", indexes["hosts"])
	}
	if !IsStringInArray("databases", indexes["groups"]) {
		t.Errorf("Got %v, expected the databases group", indexes["groups"])
	}
}

// The authority endpoints are only available when the push server runs.
func TestHttpAuthorityHandlersWithoutPushServer(t *testing.T) {

	setupTestWigo(t, "databases")

	if code, _ := HttpAuthorityListHandler(nil, testRequest(t)); code != 500 {
		t.Errorf("Code = %d, expected 500", code)
	}
	if code, _ := HttpAuthorityAllowHandler(nil, testRequest(t, "uuid", testClientUuid)); code != 500 {
		t.Errorf("Code = %d, expected 500", code)
	}
	if code, _ := HttpAuthorityRevokeHandler(nil, testRequest(t, "uuid", testClientUuid)); code != 500 {
		t.Errorf("Code = %d, expected 500", code)
	}
}

func TestHttpAuthorityHandlers(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	wigo.push = newTestPushServer(t)

	wigo.push.authority.AddClientToWaitingList(testClientUuid, "db-1")

	code, body := HttpAuthorityListHandler(nil, testRequest(t))
	if code != 200 {
		t.Fatalf("Code = %d, expected 200", code)
	}

	lists := make(map[string]map[string]string)
	if err := json.Unmarshal([]byte(body), &lists); err != nil {
		t.Fatalf("The body is not valid json : %s", err)
	}
	if lists["waiting"][testClientUuid] != "db-1" {
		t.Errorf("Got %v, expected the client in the waiting list", lists["waiting"])
	}

	// Allow it
	if code, _ := HttpAuthorityAllowHandler(nil, testRequest(t, "uuid", testClientUuid)); code != 200 {
		t.Errorf("Code = %d, expected 200", code)
	}
	if !wigo.push.authority.IsAllowed(testClientUuid) {
		t.Errorf("The client has not been allowed")
	}

	// Then revoke it
	if code, _ := HttpAuthorityRevokeHandler(nil, testRequest(t, "uuid", testClientUuid)); code != 200 {
		t.Errorf("Code = %d, expected 200", code)
	}
	if wigo.push.authority.IsAllowed(testClientUuid) {
		t.Errorf("The client has not been revoked")
	}

	// Allowing an unknown client fails
	if code, _ := HttpAuthorityAllowHandler(nil, testRequest(t, "uuid", "unknown")); code != 500 {
		t.Errorf("Code = %d, expected 500 for an unknown client", code)
	}
}

// testRequest builds a request carrying the route values a handler reads, which
// is what the router gives it in production.
func testRequest(t *testing.T, keyValues ...string) *http.Request {
	t.Helper()

	request, err := http.NewRequest("GET", "http://localhost/", nil)
	if err != nil {
		t.Fatalf("Fail to build the request : %s", err)
	}

	for i := 0; i+1 < len(keyValues); i += 2 {
		request.SetPathValue(keyValues[i], keyValues[i+1])
	}

	return request
}
