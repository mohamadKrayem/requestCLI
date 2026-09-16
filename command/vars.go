package command

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mohamadkrayem/requestCLI/collection"
	"github.com/mohamadkrayem/requestCLI/httpfile"
)

// Vars reports every variable a .http file references, with the value it
// resolves to and where that value came from.
//
// Modelled on `git config --list --show-origin`. It exists because the scope
// model is explicit: with eight scopes and two namespaces, "why is this
// hitting the wrong host" is otherwise a twenty-minute hunt through files, and
// here it is one command. Nothing else in this space reports origins.
func Vars(path string, opts *Options) error {
	file, err := httpfile.ParseFile(path)
	if err != nil {
		return err
	}

	built, err := buildScopes(file, path, opts)
	if err != nil {
		return err
	}

	rows := collectVars(file, built, path, opts)
	if len(rows) == 0 {
		fmt.Printf("%s references no variables\n", path)
		return nil
	}

	printVars(rows)
	return nil
}

// varRow is one reported variable.
type varRow struct {
	name   string
	value  string
	origin string
	// unresolved marks a variable nothing defines, which is the case worth
	// finding: it is why the request is about to fail.
	unresolved bool
}

// collectVars gathers every referenced variable across the file's requests,
// and across the collection defaults those requests inherit.
//
// The defaults matter as much as the file: an inherited
// `token = "{{secret:api_token}}"` is what authenticates every request here,
// and a report that omitted it would be exactly the report someone reaches for
// when auth is failing.
func collectVars(file *httpfile.File, built scopes, path string, opts *Options) []varRow {
	var (
		rows []varRow
		seen = map[string]bool{}
	)

	for _, name := range defaultsNames(built.defaults) {
		if seen[name] {
			continue
		}
		seen[name] = true
		rows = append(rows, resolveRow(built.resolver, name, opts))
	}

	for _, request := range file.Requests {
		resolver := built.resolver
		if len(request.Vars) > 0 {
			resolver = withRequestVars(resolver, request, path)
		}

		for _, name := range httpfile.Names(request) {
			if seen[name] {
				continue
			}
			seen[name] = true
			rows = append(rows, resolveRow(resolver, name, opts))
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })
	return rows
}

// defaultsNames lists the placeholders a collection's inherited headers and
// auth reference.
func defaultsNames(defaults collection.Defaults) []string {
	var (
		names []string
		seen  = map[string]bool{}
	)

	// Scanned through a synthetic request so the same placeholder grammar
	// applies here as everywhere else.
	fields := make([]httpfile.Header, 0, len(defaults.Headers)+3)
	for name, value := range defaults.Headers {
		fields = append(fields, httpfile.Header{Name: name, Value: value})
	}
	fields = append(fields,
		httpfile.Header{Value: defaults.Auth.Token},
		httpfile.Header{Value: defaults.Auth.Username},
		httpfile.Header{Value: defaults.Auth.Password},
	)

	for _, name := range httpfile.Names(httpfile.Request{Headers: fields}) {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// resolveRow resolves one name for reporting.
func resolveRow(resolver httpfile.Resolver, name string, opts *Options) varRow {
	// A generated built-in has no source to report, and printing the value
	// would be misleading: it differs on every request.
	if strings.HasPrefix(name, "$") {
		return varRow{name: name, value: "(generated per request)", origin: "built-in"}
	}

	resolution, ok := resolver.Lookup(name)
	if !ok {
		return varRow{name: name, value: "(undefined)", origin: "-", unresolved: true}
	}

	value := resolution.Value
	if resolution.Secret && !opts.ShowSecrets {
		value = "****"
	}

	origin := resolution.Origin
	if resolution.System {
		// Flagged inline, because this is the scope that works here and fails
		// for a teammate.
		origin += " (machine-local)"
	}
	return varRow{name: name, value: value, origin: origin}
}

// printVars renders the table, aligned on the widest name and value.
func printVars(rows []varRow) {
	nameWidth, valueWidth := 0, 0
	for _, row := range rows {
		nameWidth = max(nameWidth, len(row.name))
		valueWidth = max(valueWidth, len(row.value))
	}

	unresolved := 0
	for _, row := range rows {
		fmt.Printf("%-*s = %-*s  (%s)\n", nameWidth, row.name, valueWidth, row.value, row.origin)
		if row.unresolved {
			unresolved++
		}
	}

	// On stderr, so the table itself stays parseable.
	if unresolved > 0 {
		fmt.Fprintf(os.Stderr, "\n%d %s undefined; these requests will fail\n",
			unresolved, plural(unresolved, "variable is", "variables are"))
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
