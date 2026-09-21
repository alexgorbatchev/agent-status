package store

import (
	internalstore "github.com/alexgorbatchev/agent-status/packages/agent-server/internal/store"
)

type Store = internalstore.Store
type AgentFilter = internalstore.AgentFilter

var New = internalstore.New
var OpenReadOnly = internalstore.OpenReadOnly
var ErrNotFound = internalstore.ErrNotFound
