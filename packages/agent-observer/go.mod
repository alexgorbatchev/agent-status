module github.com/alexgorbatchev/agent-status/packages/agent-observer

go 1.26.2

require (
	github.com/alexgorbatchev/agent-status/packages/agent-common v0.0.0-00010101000000-000000000000
	github.com/alexgorbatchev/agent-status/packages/agent-parser v0.0.0-00010101000000-000000000000
	github.com/alexgorbatchev/cobra-help-tree v1.1.0
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/coder/websocket v1.8.15
	github.com/fsnotify/fsnotify v1.10.1
	github.com/ncruces/go-sqlite3 v0.35.5
	github.com/shirou/gopsutil/v4 v4.26.8
	github.com/spf13/cobra v1.10.2
)

require (
	github.com/ebitengine/purego v0.10.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/ncruces/go-sqlite3-wasm/v6 v6.2.35304 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/term v0.45.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)

replace (
	github.com/alexgorbatchev/agent-status/packages/agent-common => ../agent-common
	github.com/alexgorbatchev/agent-status/packages/agent-parser => ../agent-parser
)
