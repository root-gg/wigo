package wigo

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func setupTokenTest(t *testing.T) {
	t.Helper()

	setupTestWigo(t, "databases")
	LocalWigo.config.Http.Login = "germain"
	LocalWigo.config.Http.Password = "s3cret"
	LocalWigo.config.Http.AllowWriteActions = true
}

// The secret is readable exactly once. Storing only its hash is what makes a
// stolen database useless against the api.
func TestATokenSecretIsNeverStored(t *testing.T) {
	setupTokenTest(t)

	token, secret, err := CreateApiToken("prometheus", RoleReadOnly, 0)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if !strings.HasPrefix(secret, "wigo_") {
		t.Errorf("Got %q, expected a recognisable prefix", secret)
	}

	// Nothing anywhere gives it back
	listed, err := ApiTokens()
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(listed) != 1 || listed[0].Name != "prometheus" || listed[0].Role != RoleReadOnly {
		t.Fatalf("Got %+v", listed)
	}

	var storedHash string
	LocalWigo.sqlLiteLock.Lock()
	err = LocalWigo.sqlLiteConn.QueryRow(`SELECT hash FROM api_tokens WHERE id = ?;`, token.Id).Scan(&storedHash)
	LocalWigo.sqlLiteLock.Unlock()
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if strings.Contains(storedHash, secret) || storedHash == secret {
		t.Errorf("The secret itself was stored")
	}

	// And it authenticates
	if recognised, ok := authenticateToken(secret); !ok || recognised.Id != token.Id {
		t.Errorf("The token should have been recognised")
	}
}

func TestARevokedTokenStopsWorking(t *testing.T) {
	setupTokenTest(t)

	token, secret, err := CreateApiToken("laptop", RoleOperator, 0)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if _, ok := authenticateToken(secret); !ok {
		t.Fatalf("It should work before being revoked")
	}

	if err := RevokeApiToken(token.Id); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if _, ok := authenticateToken(secret); ok {
		t.Errorf("A revoked token must not authenticate anything")
	}

	// The row stays : what it was called and when it was turned off is the
	// answer to a question somebody will ask later
	listed, _ := ApiTokens()
	if len(listed) != 1 || listed[0].RevokedAt == 0 {
		t.Errorf("Got %+v, expected the revoked token to still be listed", listed)
	}

	// Revoking it twice says so rather than pretending
	if err := RevokeApiToken(token.Id); err == nil {
		t.Errorf("Revoking an already revoked token should say so")
	}
}

func TestAnExpiredTokenStopsWorking(t *testing.T) {
	setupTokenTest(t)

	_, secret, err := CreateApiToken("afternoon", RoleOperator, time.Now().Unix()+3600)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if _, ok := authenticateToken(secret); !ok {
		t.Fatalf("It should work before expiring")
	}

	expireToken(t, "afternoon", time.Now().Unix()-1)

	if _, ok := authenticateToken(secret); ok {
		t.Errorf("An expired token must not authenticate anything")
	}
}

func TestCreateApiTokenRefusesWhatItCannotHonour(t *testing.T) {
	setupTokenTest(t)

	if _, _, err := CreateApiToken("", RoleOperator, 0); err == nil {
		t.Errorf("A token needs a name to be recognised and revoked later")
	}
	if _, _, err := CreateApiToken("x", "admin", 0); err == nil {
		t.Errorf("An unknown role should be refused")
	}
	if _, _, err := CreateApiToken("x", RoleOperator, time.Now().Unix()-1); err == nil {
		t.Errorf("A token cannot expire in the past")
	}

	if listed, _ := ApiTokens(); len(listed) != 0 {
		t.Errorf("Got %+v, expected nothing to have been stored", listed)
	}
}

// Handing somebody the dashboard so they can look at a graph must not also hand
// them the ability to switch the monitoring off.
func TestAReadOnlyTokenChangesNothing(t *testing.T) {
	setupTokenTest(t)
	root := newTestProbesDirectory(t, "60/check_load")
	LocalWigo.config.Global.ProbesDirectory = root

	_, readOnly, err := CreateApiToken("grafana", RoleReadOnly, 0)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	_, operator, err := CreateApiToken("laptop", RoleOperator, 0)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	// Reading is fine
	if status, _ := HttpProbesHandler(httptest.NewRecorder(), requestWithToken(t, readOnly)); status != 200 {
		t.Errorf("A read only token should be able to read, got %d", status)
	}

	// Changing is not
	status, message := HttpProbeDisableHandler(httptest.NewRecorder(), requestWithToken(t, readOnly))
	if status != 403 {
		t.Errorf("Got %d, expected a read only token to be refused", status)
	}
	if !strings.Contains(message, "read only") {
		t.Errorf("Got %q, expected it to say why", message)
	}
	if !probeIsIn(t, root, "60", "check_load") {
		t.Errorf("The probe should not have moved")
	}

	// And an operator is
	request := requestWithToken(t, operator)
	request.SetPathValue("probe", "check_load")
	if status, message := HttpProbeDisableHandler(httptest.NewRecorder(), request); status != 200 {
		t.Errorf("Got %d %q, expected an operator to be allowed", status, message)
	}
}

// Forwarding must not be a way around the check that just failed.
func TestAReadOnlyTokenCannotReachThroughToARemote(t *testing.T) {
	setupTokenTest(t)

	_, readOnly, err := CreateApiToken("grafana", RoleReadOnly, 0)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	request := requestWithToken(t, readOnly)
	request.SetPathValue("hostname", "db-1")
	request.SetPathValue("probe", "check_load")

	if status, _ := HttpHostProbeDisableHandler(httptest.NewRecorder(), request); status != 403 {
		t.Errorf("Got %d, expected a read only token to be refused before anything is forwarded", status)
	}
	if status, _ := HttpHostProbeRunHandler(httptest.NewRecorder(), request); status != 403 {
		t.Errorf("Got %d, expected a recheck to be refused too", status)
	}
}

// A wigo with no credential is open, and a token there would look like
// protection while every request without one still walks through.
func TestATokenIsRefusedOnAnUnguardedWigo(t *testing.T) {
	setupTokenTest(t)
	LocalWigo.config.Http.Login = ""

	status, message := HttpTokenCreateHandler(httptest.NewRecorder(),
		httptest.NewRequest("POST", "/api/tokens?name=x&role=operator", nil))

	if status != 400 {
		t.Errorf("Got %d, expected it to be refused", status)
	}
	if !strings.Contains(message, "Login and Password") {
		t.Errorf("Got %q, expected it to say what to do first", message)
	}
}

// A token that was presented and refused must not fall back on the shared
// credential : a revoked token would otherwise still work for whoever also
// knows the password.
func TestARefusedTokenDoesNotFallBackOnTheCredential(t *testing.T) {
	setupTokenTest(t)

	handler := Chain(Handler(func(w http.ResponseWriter, r *http.Request) (int, string) {
		return 200, "ok"
	}), Authenticating("germain", "s3cret"))

	request := httptest.NewRequest("GET", "/api/probes", nil)
	request.SetBasicAuth("germain", "s3cret")
	request.Header.Set("Authorization", "Bearer wigo_revoked")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != 401 {
		t.Errorf("Code = %d, expected 401", recorder.Code)
	}
}

// The shared credential keeps working, and keeps being an operator : an upgrade
// must not lock an administrator out of their own install.
func TestTheSharedCredentialIsStillAnOperator(t *testing.T) {
	setupTokenTest(t)

	handler := Chain(Handler(func(w http.ResponseWriter, r *http.Request) (int, string) {
		caller := CallerOf(r)
		if !caller.May(RoleOperator) {
			return 403, "not an operator"
		}
		return 200, caller.Name
	}), Authenticating("germain", "s3cret"))

	request := httptest.NewRequest("GET", "/api/probes", nil)
	request.SetBasicAuth("germain", "s3cret")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != 200 || recorder.Body.String() != "germain" {
		t.Errorf("Got %d %q", recorder.Code, recorder.Body.String())
	}
}

// An install with nothing configured stays open, or upgrading would lock every
// one of them out.
func TestAnUnguardedWigoAllowsEverything(t *testing.T) {
	setupTokenTest(t)

	caller := CallerOf(httptest.NewRequest("GET", "/api/probes", nil))
	if !caller.May(RoleOperator) {
		t.Errorf("Got %+v, expected an unguarded request to be allowed", caller)
	}
}

func TestCallerMay(t *testing.T) {
	operator := Caller{Role: RoleOperator}
	readOnly := Caller{Role: RoleReadOnly}

	if !operator.May(RoleOperator) || !operator.May(RoleReadOnly) {
		t.Errorf("An operator may do both")
	}
	if readOnly.May(RoleOperator) {
		t.Errorf("A read only caller may not change anything")
	}
	if !readOnly.May(RoleReadOnly) {
		t.Errorf("A read only caller may read")
	}
}

// Both header shapes, because a scraper and a script do not use the same one.
func TestATokenIsReadFromEitherHeader(t *testing.T) {
	bearer := httptest.NewRequest("GET", "/", nil)
	bearer.Header.Set("Authorization", "Bearer wigo_abc")
	if got := presentedToken(bearer); got != "wigo_abc" {
		t.Errorf("Got %q from an Authorization header", got)
	}

	custom := httptest.NewRequest("GET", "/", nil)
	custom.Header.Set("X-Wigo-Token", "wigo_def")
	if got := presentedToken(custom); got != "wigo_def" {
		t.Errorf("Got %q from an X-Wigo-Token header", got)
	}

	// Basic auth is not a token
	basic := httptest.NewRequest("GET", "/", nil)
	basic.SetBasicAuth("germain", "s3cret")
	if got := presentedToken(basic); got != "" {
		t.Errorf("Got %q, expected basic auth not to be read as a token", got)
	}
}

func TestParseTokenExpiry(t *testing.T) {
	if expiry, err := parseTokenExpiry(""); err != nil || expiry != 0 {
		t.Errorf("Got %d %v, expected a token that does not expire", expiry, err)
	}
	if expiry, err := parseTokenExpiry("24h"); err != nil || expiry <= time.Now().Unix() {
		t.Errorf("Got %d %v", expiry, err)
	}
	for _, value := range []string{"10s", "soon", "-1h"} {
		if _, err := parseTokenExpiry(value); err == nil {
			t.Errorf("%q should be refused", value)
		}
	}
}

func requestWithToken(t *testing.T, secret string) *http.Request {
	t.Helper()

	request := httptest.NewRequest("POST", "/api/probes/check_load/disable", nil)
	request.SetPathValue("probe", "check_load")

	token, ok := authenticateToken(secret)
	if !ok {
		t.Fatalf("The token should have been recognised")
	}

	return withCaller(request, Caller{Role: token.Role, Name: token.Name})
}

func expireToken(t *testing.T, name string, at int64) {
	t.Helper()

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	if _, err := LocalWigo.sqlLiteConn.Exec(
		`UPDATE api_tokens SET expires_at = ? WHERE name = ?;`, at, name); err != nil {
		t.Fatalf("Fail to expire the token : %s", err)
	}
}
