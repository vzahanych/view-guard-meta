module github.com/vzahanych/view-guard-meta/edge/orchestrator

go 1.25.0

require (
	github.com/bluenviron/gortsplib/v4 v4.8.0
	github.com/google/uuid v1.6.0
	github.com/mattn/go-sqlite3 v1.14.22
	github.com/pion/rtp v1.8.3
	go.uber.org/zap v1.27.0
	google.golang.org/grpc v1.64.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/bluenviron/mediacommon v1.9.2 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.14 // indirect
	github.com/pion/sdp/v3 v3.0.6 // indirect
	github.com/vzahanych/view-guard-meta/proto v0.0.0-20251123071821-e3a4380befa5 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.22.0 // indirect
	golang.org/x/sys v0.18.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240318140521-94a12d6c2237 // indirect
	google.golang.org/protobuf v1.34.1 // indirect
)

replace github.com/vzahanych/view-guard-meta/proto => ../../proto
