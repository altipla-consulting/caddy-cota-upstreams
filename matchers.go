package caddy_docker_upstreams

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)

const (
	LabelHosts = "cota.hosts"
)

var producers = map[string]func(ctx caddy.Context, value string) (caddyhttp.RequestMatcher, error){
	LabelHosts: func(ctx caddy.Context, value string) (caddyhttp.RequestMatcher, error) {
		ctx.Logger().Debug("build hosts matcher", zap.String("value", value))
		return matchHost(strings.Split(value, ",")), nil
	},
}

func buildMatchers(ctx caddy.Context, labels map[string]string) caddyhttp.MatcherSet {
	var matchers caddyhttp.MatcherSet

	for key, producer := range producers {
		value, ok := labels[key]
		if !ok {
			continue
		}

		matcher, err := producer(ctx, value)
		if err != nil {
			ctx.Logger().Error("unable to load matcher", zap.String("key", key), zap.String("value", value), zap.Error(err))
			continue
		}

		if prov, ok := matcher.(caddy.Provisioner); ok {
			err = prov.Provision(ctx)
			if err != nil {
				ctx.Logger().Error("unable to provision matcher", zap.String("key", key), zap.String("value", value), zap.Error(err))
				continue
			}
		}

		matchers = append(matchers, matcher)
	}

	return matchers
}

type matchHost []string

// Provision sets up and validates m, including making it more efficient for large lists.
func (m matchHost) Provision(_ caddy.Context) error {
	// check for duplicates; they are nonsensical and reduce efficiency
	// (we could just remove them, but the user should know their config is erroneous)
	seen := make(map[string]int)
	for i, h := range m {
		h = strings.ToLower(h)
		if firstI, ok := seen[h]; ok {
			return fmt.Errorf("host at index %d is repeated at index %d: %s", firstI, i, h)
		}
		seen[h] = i
	}

	if m.large() {
		// sort the slice lexicographically, grouping "fuzzy" entries (wildcards and placeholders)
		// at the front of the list; this allows us to use binary search for exact matches, which
		// we have seen from experience is the most common kind of value in large lists; and any
		// other kinds of values (wildcards and placeholders) are grouped in front so the linear
		// search should find a match fairly quickly
		sort.Slice(m, func(i, j int) bool {
			iInexact, jInexact := m.fuzzy(m[i]), m.fuzzy(m[j])
			if iInexact && !jInexact {
				return true
			}
			if !iInexact && jInexact {
				return false
			}
			return m[i] < m[j]
		})
	}

	return nil
}

// Match returns true if r matches m.
func (m matchHost) Match(r *http.Request) bool {
	reqHost := r.Host

	if m.large() {
		// fast path: locate exact match using binary search (about 100-1000x faster for large lists)
		pos := sort.Search(len(m), func(i int) bool {
			if m.fuzzy(m[i]) {
				return false
			}
			return m[i] >= reqHost
		})
		if pos < len(m) && m[pos] == reqHost {
			return true
		}
	}

	repl := r.Context().Value(caddy.ReplacerCtxKey).(*caddy.Replacer)

outer:
	for _, host := range m {
		// fast path: if matcher is large, we already know we don't have an exact
		// match, so we're only looking for fuzzy match now, which should be at the
		// front of the list; if we have reached a value that is not fuzzy, there
		// will be no match and we can short-circuit for efficiency
		if m.large() && !m.fuzzy(host) {
			break
		}

		host = repl.ReplaceAll(host, "")
		if strings.Contains(host, "*") {
			patternParts := strings.Split(host, ".")
			incomingParts := strings.Split(reqHost, ".")
			if len(patternParts) != len(incomingParts) {
				continue
			}
			for i := range patternParts {
				if patternParts[i] == "*" {
					continue
				}
				if !strings.EqualFold(patternParts[i], incomingParts[i]) {
					continue outer
				}
			}
			return true
		} else if strings.EqualFold(reqHost, host) {
			return true
		}
	}

	return false
}

// fuzzy returns true if the given hostname h is not a specific
// hostname, e.g. has placeholders or wildcards.
func (matchHost) fuzzy(h string) bool { return strings.ContainsAny(h, "{*") }

// large returns true if m is considered to be large. Optimizing
// the matcher for smaller lists has diminishing returns.
// See related benchmark function in test file to conduct experiments.
func (m matchHost) large() bool { return len(m) > 100 }
