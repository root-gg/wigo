package wigo

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A monitoring tool going down because one handler panicked is the worst
// possible failure : nobody is watching, and nobody is told.
func TestAPanicIsCaught(t *testing.T) {
	handler := Chain(Handler(func(w http.ResponseWriter, r *http.Request) (int, string) {
		panic("something unexpected")
	}), Recovering())

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/boom", nil))

	if recorder.Code != 500 {
		t.Errorf("Code = %d, expected 500", recorder.Code)
	}
}

// The api says it speaks json, unless the handler already decided otherwise.
func TestContentTypeIsSetForTheApiOnly(t *testing.T) {
	json := Handler(func(w http.ResponseWriter, r *http.Request) (int, string) { return 200, "{}" })

	recorder := httptest.NewRecorder()
	json.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/probes", nil))
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Got %q for an api path", got)
	}

	recorder = httptest.NewRecorder()
	json.ServeHTTP(recorder, httptest.NewRequest("GET", "/index.html", nil))
	if got := recorder.Header().Get("Content-Type"); got == "application/json" {
		t.Errorf("A page is not json")
	}

	// /metrics sets its own and must keep it
	metrics := Handler(func(w http.ResponseWriter, r *http.Request) (int, string) {
		w.Header().Set("Content-Type", "text/plain")
		return 200, "wigo_up 1"
	})
	recorder = httptest.NewRecorder()
	metrics.ServeHTTP(recorder, httptest.NewRequest("GET", "/metrics", nil))
	if got := recorder.Header().Get("Content-Type"); got != "text/plain" {
		t.Errorf("Got %q, expected the handler's own choice to stand", got)
	}
}

// Wrong login, wrong password, or neither : all refused, and the credential is
// the only thing standing in front of an api that can disable probes.
func TestAuthenticatingRefusesEverythingButTheCredential(t *testing.T) {
	handler := Chain(Handler(func(w http.ResponseWriter, r *http.Request) (int, string) {
		return 200, "ok"
	}), Authenticating("germain", "s3cret", RoleNone))

	cases := []struct {
		login, password string
		expected        int
	}{
		{"germain", "s3cret", 200},
		{"germain", "wrong", 401},
		{"wrong", "s3cret", 401},
		{"", "", 401},
		{"germain", "s3cre", 401},
		{"germain", "s3crett", 401},
	}

	for _, test := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest("GET", "/api/probes", nil)
		if test.login != "" || test.password != "" {
			request.SetBasicAuth(test.login, test.password)
		}

		handler.ServeHTTP(recorder, request)
		if recorder.Code != test.expected {
			t.Errorf("%q/%q got %d, expected %d", test.login, test.password, recorder.Code, test.expected)
		}
	}

	// A request with no Authorization header at all
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/probes", nil))
	if recorder.Header().Get("WWW-Authenticate") == "" {
		t.Errorf("A 401 has to say how to authenticate")
	}
}

// Compressed or not, the body has to be the same.
func TestGzipOnlyWhenAsked(t *testing.T) {
	handler := Chain(Handler(func(w http.ResponseWriter, r *http.Request) (int, string) {
		return 200, strings.Repeat("wigo ", 200)
	}), Gzip())

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/probes", nil))
	if recorder.Header().Get("Content-Encoding") == "gzip" {
		t.Errorf("Nothing asked for gzip")
	}

	recorder = httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/probes", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	handler.ServeHTTP(recorder, request)

	if recorder.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("It was asked for")
	}
	if recorder.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("A cache has to know the answer depends on the header")
	}
	// A Content-Length computed on the uncompressed body would be a lie
	if recorder.Header().Get("Content-Length") != "" {
		t.Errorf("Got a Content-Length alongside a compressed body")
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := Chain(Handler(func(w http.ResponseWriter, r *http.Request) (int, string) {
		return 200, "ok"
	}), SecurityHeaders())

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/", nil))

	for header, expected := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "same-origin",
	} {
		if got := recorder.Header().Get(header); got != expected {
			t.Errorf("%s = %q, expected %q", header, got, expected)
		}
	}
}

// The outermost middleware listed has to be the outermost applied, or the
// credential would be checked after the handler has already run.
func TestChainAppliesInReadingOrder(t *testing.T) {
	order := make([]string, 0)

	mark := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}

	handler := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
	}), mark("first"), mark("second"))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	if strings.Join(order, ",") != "first,second,handler" {
		t.Errorf("Got %v", order)
	}
}

// The role of the caller, as the chain saw them.
func callerServedBy(t *testing.T, middleware Middleware, decorate func(*http.Request)) (int, Caller) {
	t.Helper()

	seen := Caller{}
	handler := Chain(Handler(func(w http.ResponseWriter, r *http.Request) (int, string) {
		seen = CallerOf(r)
		return 200, "ok"
	}), middleware)

	request := httptest.NewRequest("GET", "/api/probes", nil)
	if decorate != nil {
		decorate(request)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return recorder.Code, seen
}

// The setting people actually want : the dashboard is open to whoever can reach
// it, and changing anything still needs the credential.
func TestAnonymousCanBeLetInReadOnly(t *testing.T) {
	middleware := Authenticating("germain", "s3cret", RoleReadOnly)

	status, caller := callerServedBy(t, middleware, nil)
	if status != 200 {
		t.Fatalf("Got %d, expected an anonymous caller to be served", status)
	}
	if caller.Role != RoleReadOnly {
		t.Errorf("Got role %q, expected read only", caller.Role)
	}
	if caller.May(RoleOperator) {
		t.Errorf("An anonymous reader must not be able to change anything")
	}

	// And the credential still identifies the administrator
	status, caller = callerServedBy(t, middleware, func(r *http.Request) {
		r.SetBasicAuth("germain", "s3cret")
	})
	if status != 200 || !caller.May(RoleOperator) {
		t.Errorf("Got %d and %+v, expected the credential to still grant operator", status, caller)
	}
}

// Trying to say who you are and getting it wrong is not the same as not saying:
// serving the typo as anonymous would hide it behind a dashboard that half
// works.
func TestAWrongPasswordIsRefusedEvenWhenAnonymousIsWelcome(t *testing.T) {
	status, _ := callerServedBy(t, Authenticating("germain", "s3cret", RoleReadOnly),
		func(r *http.Request) { r.SetBasicAuth("germain", "wrong") })

	if status != 401 {
		t.Errorf("Got %d, expected 401", status)
	}
}

// Nothing to compare against, so nothing a presented password could unlock.
// Refusing it would lock a client out of a wigo that guards nothing.
func TestCredentialsAreIgnoredWhenNoneAreConfigured(t *testing.T) {
	status, caller := callerServedBy(t, Authenticating("", "", RoleReadOnly),
		func(r *http.Request) { r.SetBasicAuth("someone", "anything") })

	if status != 200 || caller.Role != RoleReadOnly {
		t.Errorf("Got %d and %+v, expected an anonymous read only caller", status, caller)
	}
}

// Both historical behaviours, which an upgrade may not change: no Login served
// everybody as an operator, a Login refused everybody without it.
func TestAnUnsetAnonymousRoleKeepsWhatTheInstallDid(t *testing.T) {
	open := &HttpConfig{}
	if role := ResolvedAnonymousRole(open); role != RoleOperator {
		t.Errorf("Got %q, expected a wigo with no Login to stay open as an operator", role)
	}

	guarded := &HttpConfig{Login: "germain", Password: "s3cret"}
	if role := ResolvedAnonymousRole(guarded); role != RoleNone {
		t.Errorf("Got %q, expected a wigo with a Login to keep refusing anonymous callers", role)
	}

	// Half a credential guards nothing, so it cannot be what closes the door
	half := &HttpConfig{Login: "germain"}
	if role := ResolvedAnonymousRole(half); role != RoleOperator {
		t.Errorf("Got %q, expected a login with no password to change nothing", role)
	}
}

// A typo in the setting meant to lock a wigo down must not be what opens it.
func TestAnUnknownAnonymousRoleFallsBackToReadOnly(t *testing.T) {
	config := &HttpConfig{Login: "germain", Password: "s3cret", AnonymousRole: "read-only"}

	if role := ResolvedAnonymousRole(config); role != RoleReadOnly {
		t.Errorf("Got %q, expected read only rather than operator", role)
	}
}

func TestAnonymousRoleIsHonouredWhenSet(t *testing.T) {
	for _, role := range []string{RoleNone, RoleReadOnly, RoleOperator} {
		config := &HttpConfig{AnonymousRole: role}
		if got := ResolvedAnonymousRole(config); got != role {
			t.Errorf("Got %q, expected %q", got, role)
		}
	}
}
