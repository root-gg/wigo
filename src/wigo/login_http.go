package wigo

import (
	"net/http"
	"net/url"
	"strings"
)

// Signing in, when being anonymous is not enough.
//
// A wigo that lets anonymous callers read never sends a 401, so a browser is
// never challenged and there is no way to become the operator from the
// interface -- the credential exists and is unreachable. These two routes are
// the only way to provoke that challenge on purpose.
//
// They are navigated to, not fetched. A browser shows its credential prompt for
// a top level navigation reliably ; for an XHR it depends on the browser and
// the version, which is not something to build a sign-in on.

// HttpLoginHandler challenges the browser, then sends it back where it was.
func HttpLoginHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	if CallerOf(r).May(RoleOperator) {
		w.Header().Set("Location", safeReturnPath(r.URL.Query().Get("next")))
		return 302, ""
	}

	w.Header().Set("WWW-Authenticate", `Basic realm="Authorization Required"`)
	return 401, "Not Authorized"
}

// HttpLogoutHandler refuses, so the browser forgets.
//
// There is no way to tell a browser to drop a basic auth credential. Answering
// 401 makes it ask again, and cancelling that prompt is what clears it. Not
// pretty, and the only thing that works without asking somebody to close every
// window they have open.
func HttpLogoutHandler(w http.ResponseWriter, r *http.Request) (int, string) {
	w.Header().Set("WWW-Authenticate", `Basic realm="Authorization Required"`)
	return 401, "Signed out. Cancel the prompt to stay signed out."
}

// safeReturnPath keeps a redirect on this host.
//
// Sending somebody wherever a query string says is an open redirect, and an
// open redirect on the page somebody just typed a password into is the one
// place it really matters.
func safeReturnPath(next string) string {
	if next == "" {
		return "/"
	}

	// A path, not a url : no scheme, no host, and not "//host" either, which a
	// browser reads as protocol relative and would leave this origin.
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	if parsed, err := url.Parse(next); err != nil || parsed.Scheme != "" || parsed.Host != "" {
		return "/"
	}

	return next
}
