package git

import (
	internalgit "github.com/alexgorbatchev/agent-status/packages/agent-observer/internal/git"
)

type Resolver = internalgit.Resolver
type CacheEntry = internalgit.CacheEntry

const DefaultCacheTTL = internalgit.DefaultCacheTTL

var NewResolver = internalgit.NewResolver
