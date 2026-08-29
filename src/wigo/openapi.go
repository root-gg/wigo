package wigo

import (
	_ "embed"
	"net/http"
	"strings"
)

// The api, written down.
//
// Served as well as kept in the tree so whatever reads it -- the browser
// extension, a generator, somebody with curl -- can ask this wigo what it
// answers rather than guess from a version number.
//
// It sits next to the code it describes on purpose. A document a directory
// away from what it documents is a document that gets updated a week later, or
// never ; and openapi_test.go compares it to the routes actually registered,
// so "never" fails the build instead of misleading a reader.

//go:embed openapi.yaml
var openApiDocument string

// Route is one thing the http server answers.
type Route struct {
	// Lower case, to match the openapi document rather than net/http.
	Method  string
	Pattern string
}

// OpenApiDocument is the specification as written.
func OpenApiDocument() string {
	return openApiDocument
}

// HttpOpenApiHandler serves it.
func HttpOpenApiHandler(w http.ResponseWriter, r *http.Request) (int, string) {
	w.Header().Set("Content-Type", "application/yaml")

	return 200, openApiDocument
}

// DocumentedRoutes reads the paths and methods out of the document.
//
// Deliberately a small reader over the few lines that matter rather than a
// yaml dependency : what is needed here is the list of paths and the verbs
// under each, and pulling in a parser to learn that would be a dependency
// bigger than the thing it checks.
func DocumentedRoutes(document string) []Route {
	routes := make([]Route, 0)

	inPaths := false
	path := ""

	for _, line := range strings.Split(document, "\n") {
		if strings.HasPrefix(line, "paths:") {
			inPaths = true
			continue
		}
		if !inPaths {
			continue
		}

		// Back to column zero : the paths section is over.
		if len(line) > 0 && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "#") {
			break
		}

		trimmed := strings.TrimRight(line, " ")
		if trimmed == "" || strings.HasPrefix(strings.TrimSpace(trimmed), "#") {
			continue
		}

		indent := len(trimmed) - len(strings.TrimLeft(trimmed, " "))
		content := strings.TrimSpace(trimmed)

		// Two spaces : a path. Four : a verb under it.
		if indent == 2 && strings.HasSuffix(content, ":") {
			path = strings.TrimSuffix(content, ":")
			path = strings.Trim(path, `"`)
			continue
		}

		if indent == 4 && strings.HasSuffix(content, ":") && path != "" {
			method := strings.TrimSuffix(content, ":")
			if method == "get" || method == "post" || method == "put" || method == "delete" {
				routes = append(routes, Route{Method: method, Pattern: path})
			}
		}
	}

	return routes
}

// DocumentedReferences reads every $ref in the document, and every component it
// could point at.
//
// Shallow on purpose : it resolves "#/components/<section>/<Name>", which is
// the shape every reference here has and the one that breaks silently. A yaml
// parser would resolve more and would be a dependency larger than the document
// it checks.
func DocumentedReferences(document string) (references []string, defined map[string]bool) {
	references = make([]string, 0)
	defined = make(map[string]bool)

	inComponents := false
	section := ""

	for _, line := range strings.Split(document, "\n") {
		if strings.HasPrefix(line, "components:") {
			inComponents = true
		} else if len(line) > 0 && !strings.HasPrefix(line, " ") {
			inComponents = false
		}

		if index := strings.Index(line, "$ref:"); index >= 0 {
			if reference := readReference(line[index+len("$ref:"):]); reference != "" {
				references = append(references, reference)
			}
		}

		if !inComponents {
			continue
		}

		trimmed := strings.TrimRight(line, " ")
		content := strings.TrimSpace(trimmed)
		if content == "" || !strings.HasSuffix(content, ":") {
			continue
		}

		indent := len(trimmed) - len(strings.TrimLeft(trimmed, " "))
		name := strings.TrimSuffix(content, ":")

		if indent == 2 {
			section = name
		} else if indent == 4 && section != "" {
			defined["#/components/"+section+"/"+name] = true
		}
	}

	return references, defined
}

// readReference pulls the value out of what follows a $ref key.
//
// It has to stop at the first delimiter rather than trim the ends : the inline
// form, `parameters: [{ $ref: "#/..." }]`, leaves a quote, a brace and a
// bracket behind, and trimming them off the end silently produced a reference
// that pointed at nothing and was reported as such.
func readReference(rest string) string {
	value := strings.TrimSpace(rest)
	value = strings.TrimLeft(value, `"'`)

	if end := strings.IndexAny(value, `"' }],`); end >= 0 {
		value = value[:end]
	}

	return value
}
