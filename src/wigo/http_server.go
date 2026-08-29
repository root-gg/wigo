package wigo

import (
	"compress/gzip"
	"crypto/subtle"
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

// BasicAuth guards everything behind the one credential wigo has.
//
// The comparison is constant time. It is the same shared login for the whole
// api and the whole interface, which is exactly why it must not also leak
// through how long it takes to reject : until real credentials land, this is
// the only thing standing in front of an api that can now disable probes.
func BasicAuth(login string, password string) Middleware {
	expectedLogin := []byte(login)
	expectedPassword := []byte(password)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			givenLogin, givenPassword, ok := r.BasicAuth()

			// Both compared every time, so the answer takes the same time
			// whether the login was wrong, the password was, or both.
			loginOk := subtle.ConstantTimeCompare([]byte(givenLogin), expectedLogin) == 1
			passwordOk := subtle.ConstantTimeCompare([]byte(givenPassword), expectedPassword) == 1

			if !ok || !loginOk || !passwordOk {
				w.Header().Set("WWW-Authenticate", `Basic realm="Authorization Required"`)
				http.Error(w, "Not Authorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
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
