module github.com/alexgorbatchev/agent-status/packages/print-session-cli

go 1.26.2

replace (
	github.com/alexgorbatchev/agent-status/packages/agent-common => ../agent-common
	github.com/alexgorbatchev/agent-status/packages/agent-parser => ../agent-parser
)

require (
	github.com/alexgorbatchev/agent-status/packages/agent-common v0.0.0-00010101000000-000000000000
	github.com/alexgorbatchev/agent-status/packages/agent-parser v0.0.0-00010101000000-000000000000
	github.com/alexgorbatchev/cobra-help-tree v1.1.0
	github.com/spf13/cobra v1.10.2
	golang.org/x/term v0.45.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/ncruces/go-sqlite3 v0.35.5 // indirect
	github.com/ncruces/go-sqlite3-wasm/v6 v6.2.35304 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/sys v0.48.0 // indirect
)
