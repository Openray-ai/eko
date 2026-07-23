package router

import (
	"net/http"
	"sort"
	"strings"

	"eko/internal/proxy/common"
)

type RoutingConfig struct {
	DefaultProvider common.ProviderName
	Models          map[string]common.ProviderName
	Prefixes        map[string]common.ProviderName
}

type ModelRouter struct {
	defaultProvider common.ProviderName
	models          map[string]common.ProviderName
	prefixes        map[string]common.ProviderName
}

type ResolvedRoute struct {
	Provider common.ProviderName
	Model    string
	Route    string
}

func NewModelRouter(cfg RoutingConfig) *ModelRouter {
	models := map[string]common.ProviderName{}
	for model, provider := range cfg.Models {
		models[model] = provider
	}
	prefixes := map[string]common.ProviderName{}
	for prefix, provider := range cfg.Prefixes {
		prefixes[prefix] = provider
	}
	return &ModelRouter{
		defaultProvider: cfg.DefaultProvider,
		models:          models,
		prefixes:        prefixes,
	}
}

func (r *ModelRouter) Resolve(route, model string) (ResolvedRoute, error) {
	if strings.TrimSpace(model) == "" {
		return ResolvedRoute{}, &common.ProxyError{Status: http.StatusBadRequest, Type: "invalid_request_error", Message: "model is required", Route: route}
	}
	provider := r.defaultProvider
	if exact, ok := r.models[model]; ok {
		provider = exact
	} else {
		prefixes := make([]string, 0, len(r.prefixes))
		for prefix := range r.prefixes {
			prefixes = append(prefixes, prefix)
		}
		sort.Slice(prefixes, func(i, j int) bool {
			return len(prefixes[i]) > len(prefixes[j])
		})
		for _, prefix := range prefixes {
			if strings.HasPrefix(model, prefix) {
				provider = r.prefixes[prefix]
				break
			}
		}
	}
	return ResolvedRoute{Provider: provider, Model: model, Route: route}, nil
}
