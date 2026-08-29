package main

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/root-gg/wigo/src/wigo"
)

// A specification nobody compares to the code is a specification that lies
// within a month, and quietly, which is the only way one hurts.
//
// So it is compared. Both ways : an undocumented route leaves a caller
// guessing, and a documented route that does not exist sends them somewhere
// that answers 404, which is worse because they trusted it.
func TestTheDocumentAndTheRoutesAgree(t *testing.T) {
	registerRoutes(http.NewServeMux())

	documented := make(map[string]bool)
	for _, route := range wigo.DocumentedRoutes(wigo.OpenApiDocument()) {
		documented[key(route)] = true
	}

	registered := make(map[string]bool)
	for _, route := range registeredRoutes {
		registered[key(route)] = true
	}

	if len(registeredRoutes) == 0 {
		t.Fatalf("No routes were registered, so this test checks nothing")
	}

	var undocumented, imaginary []string

	for _, route := range registeredRoutes {
		if !documented[key(route)] {
			undocumented = append(undocumented, key(route))
		}
	}
	for wanted := range documented {
		if !registered[wanted] {
			imaginary = append(imaginary, wanted)
		}
	}

	sort.Strings(undocumented)
	sort.Strings(imaginary)

	if len(undocumented) > 0 {
		t.Errorf("Answered but not in openapi.yaml :\n\t%s", strings.Join(undocumented, "\n\t"))
	}
	if len(imaginary) > 0 {
		t.Errorf("In openapi.yaml but answered by nothing :\n\t%s", strings.Join(imaginary, "\n\t"))
	}
}

// The reader is deliberately small rather than a yaml dependency, so it is
// worth checking it reads what it is pointed at.
func TestTheDocumentIsReadAtAll(t *testing.T) {
	routes := wigo.DocumentedRoutes(wigo.OpenApiDocument())

	if len(routes) < 40 {
		t.Fatalf("Got %d routes out of the document, expected the whole api", len(routes))
	}

	found := false
	for _, route := range routes {
		if route.Method == "post" && route.Pattern == "/api/tokens" {
			found = true
		}
		if !strings.HasPrefix(route.Pattern, "/") {
			t.Errorf("Read %q as a path", route.Pattern)
		}
	}

	if !found {
		t.Errorf("A path with two verbs under it lost one of them")
	}
}

func key(route wigo.Route) string {
	return route.Method + " " + route.Pattern
}

// A reference to a schema nobody wrote is a lie the route comparison cannot
// see : the paths all line up, and a generator reading it produces nothing.
func TestEveryReferencePointsAtSomething(t *testing.T) {
	references, defined := wigo.DocumentedReferences(wigo.OpenApiDocument())

	if len(references) == 0 {
		t.Fatalf("No references were read, so this test checks nothing")
	}

	var broken []string
	for _, reference := range references {
		// A reference into a component rather than at one : keep the first
		// three segments, which is what has to exist.
		parts := strings.Split(strings.TrimPrefix(reference, "#/"), "/")
		if len(parts) < 3 {
			broken = append(broken, reference)
			continue
		}

		if !defined["#/"+strings.Join(parts[:3], "/")] {
			broken = append(broken, reference)
		}
	}

	sort.Strings(broken)
	broken = unique(broken)

	if len(broken) > 0 {
		t.Errorf("Referenced but never defined :\n\t%s", strings.Join(broken, "\n\t"))
	}
}

func unique(values []string) []string {
	seen := make(map[string]bool)
	kept := make([]string, 0, len(values))

	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		kept = append(kept, value)
	}

	return kept
}
