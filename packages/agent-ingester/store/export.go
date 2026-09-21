package store

import (
	internalstore "github.com/alexgorbatchev/agent-status/packages/agent-ingester/internal/store"
)

type Store = internalstore.Store

var Open = internalstore.Open
var Migrate = internalstore.Migrate
