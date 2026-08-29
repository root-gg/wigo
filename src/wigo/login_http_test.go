package wigo

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// A wigo that lets anonymous callers read never sends a 401 on its own, so this
// is the only thing that can ask a browser for the credential.
func TestSigningInChallengesAnAnonymousCaller(t *testing.T) {
	setupTestWigo(t, "databases")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/login", nil)
	request = withCaller(request, Caller{Role: RoleReadOnly, Name: "anonymous"})

	status, _ := HttpLoginHandler(recorder, request)
	if status != 401 {
		t.Errorf("Got %d, expected a challenge", status)
	}
	if recorder.Header().Get("WWW-Authenticate") == "" {
		t.Errorf("A challenge has to say how to answer it")
	}
}

// Already signed in : send them back to what they were looking at rather than
// asking again for something they have already given.
func TestSigningInSendsAnOperatorBack(t *testing.T) {
	setupTestWigo(t, "databases")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/login?next=%2F%23%2Fhost%3Fname%3Ddb1", nil)
	request = withCaller(request, Caller{Role: RoleOperator, Name: "germain"})

	status, _ := HttpLoginHandler(recorder, request)
	if status != 302 {
		t.Errorf("Got %d, expected a redirect", status)
	}
	if got := recorder.Header().Get("Location"); got != "/#/host?name=db1" {
		t.Errorf("Got %q, expected to be sent back where I was", got)
	}
}

// An open redirect on the page somebody just typed a password into is the one
// place it really matters.
func TestTheReturnPathCannotLeaveThisHost(t *testing.T) {
	cases := map[string]string{
		"":                          "/",
		"/":                         "/",
		"/#/host?name=db1":          "/#/host?name=db1",
		"//evil.example.com":        "/",
		"https://evil.example.com":  "/",
		"http://evil.example.com/x": "/",
		"javascript:alert(1)":       "/",
		"evil.example.com":          "/",
	}

	for next, expected := range cases {
		if got := safeReturnPath(next); got != expected {
			t.Errorf("safeReturnPath(%q) = %q, expected %q", next, got, expected)
		}
	}
}

// There is no way to tell a browser to drop a credential, so the only thing
// that works is refusing until it stops offering one.
func TestSigningOutAlwaysRefuses(t *testing.T) {
	setupTestWigo(t, "databases")

	for _, caller := range []Caller{
		{Role: RoleOperator, Name: "germain"},
		{Role: RoleReadOnly, Name: "anonymous"},
	} {
		recorder := httptest.NewRecorder()
		request := withCaller(httptest.NewRequest("GET", "/api/logout", nil), caller)

		if status, _ := HttpLogoutHandler(recorder, request); status != 401 {
			t.Errorf("%s got %d, expected 401", caller.Name, status)
		}
		if recorder.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("%s : the browser has to be asked again", caller.Name)
		}
	}
}

// Offering a sign-in button on a host with no Login configured would be
// offering a door with no key behind it.
func TestWhoamiSaysWhetherSigningInIsPossible(t *testing.T) {
	setupTestWigo(t, "databases")

	recorder := httptest.NewRecorder()
	_, body := HttpWhoamiHandler(recorder, httptest.NewRequest("GET", "/api/whoami", nil))
	if strings.Contains(body, `"CanSignIn":true`) {
		t.Errorf("Got %s, expected no credential to sign in with", body)
	}

	LocalWigo.config.Http.Login = "germain"
	LocalWigo.config.Http.Password = "s3cret"

	recorder = httptest.NewRecorder()
	_, body = HttpWhoamiHandler(recorder, httptest.NewRequest("GET", "/api/whoami", nil))
	if !strings.Contains(body, `"CanSignIn":true`) {
		t.Errorf("Got %s, expected a credential to be offered", body)
	}
}
