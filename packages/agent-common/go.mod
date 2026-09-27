module github.com/alexgorbatchev/agent-status/packages/agent-common

go 1.26.2

require (
	github.com/alexgorbatchev/agent-status/packages/agent-parser v0.0.0-00010101000000-000000000000
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
)

require (
	github.com/ncruces/go-sqlite3 v0.35.5 // indirect
	github.com/ncruces/go-sqlite3-wasm/v6 v6.2.35304 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace github.com/alexgorbatchev/agent-status/packages/agent-parser => ../agent-parser
