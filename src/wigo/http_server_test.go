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
	}), Authenticating("germain", "s3cret"))

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
