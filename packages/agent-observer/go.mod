module github.com/alexgorbatchev/agent-status/packages/agent-observer

go 1.26.2

require (
	github.com/alexgorbatchev/agent-parser v1.1.0
	github.com/alexgorbatchev/agent-status/packages/agent-common v0.0.0-00010101000000-000000000000
	github.com/alexgorbatchev/agent-watcher v1.1.0
	github.com/alexgorbatchev/cobra-help-tree/v4 v4.0.0
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/coder/websocket v1.8.15
	github.com/ncruces/go-sqlite3 v0.35.5
	github.com/spf13/cobra v1.10.2
)

require (
	github.com/clipperhouse/uax29/v2 v2.2.0 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/ebitengine/purego v0.10.2 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.20.3 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-runewidth v0.0.30 // indirect
	github.com/ncruces/go-sqlite3-wasm/v6 v6.2.35304 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/shirou/gopsutil/v4 v4.26.8 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/term v0.46.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	modernc.org/libc v1.79.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.61.0 // indirect
)

replace github.com/alexgorbatchev/agent-status/packages/agent-common => ../agent-common
