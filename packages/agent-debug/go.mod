module github.com/alexgorbatchev/agent-status/packages/agent-debug

go 1.26.2

replace (
	github.com/alexgorbatchev/agent-status/packages/agent-common => ../agent-common
	github.com/alexgorbatchev/agent-status/packages/agent-ingester => ../agent-ingester
	github.com/alexgorbatchev/agent-status/packages/agent-observer => ../agent-observer
	github.com/alexgorbatchev/agent-status/packages/agent-server => ../agent-server
)

require (
	github.com/alexgorbatchev/agent-parser v1.0.0
	github.com/alexgorbatchev/agent-status/packages/agent-common v0.0.0-00010101000000-000000000000
	github.com/alexgorbatchev/agent-status/packages/agent-ingester v0.0.0-00010101000000-000000000000
	github.com/alexgorbatchev/agent-status/packages/agent-observer v0.0.0-00010101000000-000000000000
	github.com/alexgorbatchev/agent-status/packages/agent-server v0.0.0-00010101000000-000000000000
	github.com/alexgorbatchev/agent-watcher v1.0.0
	github.com/alexgorbatchev/cobra-help-tree v1.1.0
	github.com/mattn/go-isatty v0.0.20
	github.com/mattn/go-runewidth v0.0.30
	github.com/ncruces/go-sqlite3 v0.35.5
	github.com/olekukonko/tablewriter v1.1.5
	github.com/shirou/gopsutil/v4 v4.26.8
	github.com/spf13/cobra v1.10.2
	golang.org/x/term v0.46.0
)

require (
	cloud.google.com/go v0.116.0 // indirect
	cloud.google.com/go/auth v0.9.3 // indirect
	cloud.google.com/go/compute/metadata v0.5.0 // indirect
	github.com/anthropics/anthropic-sdk-go v1.26.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/clipperhouse/displaywidth v0.10.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.6.0 // indirect
	github.com/ebitengine/purego v0.10.2 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/s2a-go v0.1.8 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.4 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mozilla-ai/any-llm-go v0.9.0 // indirect
	github.com/ncruces/go-sqlite3-wasm/v6 v6.2.35304 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/olekukonko/cat v0.0.0-20250911104152-50322a0618f6 // indirect
	github.com/olekukonko/errors v1.2.0 // indirect
	github.com/olekukonko/ll v0.1.6 // indirect
	github.com/openai/openai-go v1.12.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opencensus.io v0.24.0 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	google.golang.org/genai v1.49.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240903143218-8af14fe29dc1 // indirect
	google.golang.org/grpc v1.66.2 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)
