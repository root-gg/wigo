package wigo

import (
	"compress/gzip"
	"context"
	"crypto/subtle"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// The http layer, on the standard library alone.
//
// This used to be martini, unmaintained since 2014 and disowned by its own
// author. Replacing it needed no router library at all : net/http has matched
// method and path parameters since Go 1.22, which is the only thing martini
// was really providing here. Two dependencies went away and none arrived.
//
// The handlers keep returning a status and a body rather than writing to the
// response themselves. That shape is worth keeping : it makes a handler a
// function of its request, testable without a server, and it is why every
// refusal in this codebase can be a sentence rather than a bare status code.

// Handler is what every route is. Returning the status and the body rather than
// writing them keeps a handler a plain function, and keeps the one place that
// decides on content types in one place.
type Handler func(w http.ResponseWriter, r *http.Request) (int, string)

func (handler Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	status, body := handler(w, r)

	// The api speaks json, and says so unless a handler has already decided
	// otherwise -- /metrics being the one that does.
	if w.Header().Get("Content-Type") == "" && strings.HasPrefix(r.URL.Path, "/api") {
		w.Header().Set("Content-Type", "application/json")
	}

	w.WriteHeader(status)
	if _, err := io.WriteString(w, body); err != nil {
		log.Printf("Http server : fail to write the response for %s : %s", r.URL.Path, err)
	}
}

// Middleware wraps a handler, in the order they are applied.
type Middleware func(http.Handler) http.Handler

// Chain applies middlewares so the first one listed is the outermost, which is
// the order they are read in.
func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	return handler
}

// Recovering keeps one panicking handler from taking the process with it.
//
// A monitoring tool going down because one probe result had an unexpected shape
// is the worst possible failure : nobody is watching, and nobody is told.
func Recovering() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if problem := recover(); problem != nil {
					log.Printf("Http server : panic serving %s : %v", r.URL.Path, problem)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// Logging writes one line per request, with what was answered.
func Logging() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(recorder, r)

			log.Printf("Http server : %s %s %d in %s", r.Method, r.URL.Path, recorder.status,
				time.Since(started).Round(time.Millisecond))
		})
	}
}

// statusRecorder remembers what was answered, which the log line needs and the
// ResponseWriter does not expose.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

// Caller is who made a request, once they have been recognised.
type Caller struct {
	// What they may do. Empty when nothing guards this wigo at all.
	Role string

	// How to name them in a log or a disable record.
	Name string
}

// May reports whether the caller holds at least that role.
func (caller Caller) May(role string) bool {
	if role == RoleReadOnly {
		return true
	}

	return caller.Role == RoleOperator
}

type callerKey struct{}

// CallerOf returns who made this request.
//
// An unguarded wigo has no caller, and everything is allowed : that is the
// existing behaviour of an install with no Login set, and turning it into a
// refusal on upgrade would break every one of them.
func CallerOf(r *http.Request) Caller {
	if r == nil {
		return Caller{Role: RoleOperator, Name: "unauthenticated"}
	}

	if caller, ok := r.Context().Value(callerKey{}).(Caller); ok {
		return caller
	}

	return Caller{Role: RoleOperator, Name: "unauthenticated"}
}

// Authenticating recognises the caller, by token or by the shared credential,
// and decides what somebody who presents neither is allowed to do.
//
// A token is looked at first, because it is the one that carries a role. The
// shared credential is kept, and kept as an operator : an upgrade must not lock
// an administrator out of their own install. It is what mints the first token,
// and what you remove once you have.
//
// anonymousRole is what a request with no credentials at all gets. RoleNone
// means it gets nothing and is challenged, which is what a wigo with a Login
// set has always done. A read-only anonymous role is the interesting one : the
// dashboard is open to whoever can reach it, and acting on it still needs the
// credential or a token.
func Authenticating(login string, password string, anonymousRole string) Middleware {
	expectedLogin := []byte(login)
	expectedPassword := []byte(password)

	// Nothing to compare against, so nothing a presented password could ever
	// unlock. Without this an open wigo would refuse a client that sends
	// credentials it does not need.
	credentialConfigured := login != "" && password != ""

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			if secret := presentedToken(r); secret != "" {
				if token, ok := authenticateToken(secret); ok {
					next.ServeHTTP(w, withCaller(r, Caller{
						Role: token.Role,
						Name: fmt.Sprintf("token %q", token.Name),
					}))
					return
				}

				// A token that was presented and refused is not an invitation
				// to fall back on the shared credential, nor on being anonymous
				http.Error(w, "Not Authorized", http.StatusUnauthorized)
				return
			}

			givenLogin, givenPassword, given := r.BasicAuth()

			if credentialConfigured && given {
				// Both compared every time, so the answer takes the same time
				// whether the login was wrong, the password was, or both.
				loginOk := subtle.ConstantTimeCompare([]byte(givenLogin), expectedLogin) == 1
				passwordOk := subtle.ConstantTimeCompare([]byte(givenPassword), expectedPassword) == 1

				if loginOk && passwordOk {
					next.ServeHTTP(w, withCaller(r, Caller{Role: RoleOperator, Name: givenLogin}))
					return
				}

				// Somebody tried to say who they were and got it wrong. Quietly
				// serving them as anonymous would hide the typo behind a
				// dashboard that half works.
				challenge(w)
				return
			}

			if anonymousRole == RoleNone || anonymousRole == "" {
				challenge(w)
				return
			}

			next.ServeHTTP(w, withCaller(r, Caller{Role: anonymousRole, Name: "anonymous"}))
		})
	}
}

func challenge(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="Authorization Required"`)
	http.Error(w, "Not Authorized", http.StatusUnauthorized)
}

func withCaller(r *http.Request, caller Caller) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), callerKey{}, caller))
}

// presentedToken reads the token out of a request, if there is one.
func presentedToken(r *http.Request) string {
	if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	}

	// A browser cannot set a header on a plain navigation, and a token in a
	// query string ends up in access logs, so this is the header only.
	return strings.TrimSpace(r.Header.Get("X-Wigo-Token"))
}

// SecurityHeaders sets the few headers that are worth setting unconditionally.
//
// What this replaces did nothing at all : every branch of it was gated on an
// option, and it was called with none of them set. These three cost nothing,
// need no configuration and are right for an interface that is not meant to be
// framed, sniffed or leaked in a referer.
func SecurityHeaders() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "same-origin")

			next.ServeHTTP(w, r)
		})
	}
}

// Gzip compresses what the client says it can read.
var gzipWriters = sync.Pool{
	New: func() interface{} { return gzip.NewWriter(io.Discard) },
}

func Gzip() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				next.ServeHTTP(w, r)
				return
			}

			// A stream is never compressed : buffering it is exactly what a
			// stream is not. EventSource says what it wants, so nothing here
			// needs to know which path serves one.
			if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
				next.ServeHTTP(w, r)
				return
			}

			writer := gzipWriters.Get().(*gzip.Writer)
			writer.Reset(w)
			defer func() {
				writer.Close()
				gzipWriters.Put(writer)
			}()

			w.Header().Set("Content-Encoding", "gzip")

			// The length of the uncompressed body would be wrong, and a wrong
			// Content-Length is worse than none.
			w.Header().Del("Content-Length")
			w.Header().Add("Vary", "Accept-Encoding")

			next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, writer: writer}, r)
		})
	}
}

type gzipResponseWriter struct {
	http.ResponseWriter
	writer *gzip.Writer
}

func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	return w.writer.Write(data)
}

// ResolvedAnonymousRole is what a request with no credentials gets on this
// host.
//
// Empty means nothing was configured, and the answer is then whatever this
// install already did : a wigo with no Login has always served everybody as an
// operator, and a wigo with one has always refused everybody without it.
// Neither may change because somebody upgraded.
//
// An unknown value resolves to read only rather than to operator. A typo in the
// setting meant to lock a wigo down must not be what opens it, and the mistake
// shows up immediately as buttons that refuse rather than silently as a
// dashboard anybody can act on.
func ResolvedAnonymousRole(config *HttpConfig) string {
	switch config.AnonymousRole {
	case "":
		if config.Login != "" && config.Password != "" {
			return RoleNone
		}
		return RoleOperator

	case RoleNone, RoleReadOnly, RoleOperator:
		return config.AnonymousRole

	default:
		log.Printf("Http server : unknown AnonymousRole %q, expected %q, %q or %q. "+
			"Treating anonymous callers as read only.",
			config.AnonymousRole, RoleNone, RoleReadOnly, RoleOperator)
		return RoleReadOnly
	}
}
