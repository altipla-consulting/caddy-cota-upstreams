package caddy_docker_upstreams

import (
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)

const (
	LabelHosts = "cota.hosts"
)

var producers = map[string]func(string) (caddyhttp.RequestMatcher, error){
	LabelHosts: func(value string) (caddyhttp.RequestMatcher, error) {
		return caddyhttp.MatchHost(strings.Split(value, ",")), nil
	},
}

func buildMatchers(ctx caddy.Context, labels map[string]string) caddyhttp.MatcherSet {
	var matchers caddyhttp.MatcherSet

	for key, producer := range producers {
		value, ok := labels[key]
		if !ok {
			continue
		}

		matcher, err := producer(value)
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
