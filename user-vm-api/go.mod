module github.com/vzahanych/view-guard-meta/user-vm-api

go 1.25

require (
	github.com/aws/aws-sdk-go-v2 v1.24.0
	github.com/aws/aws-sdk-go-v2/config v1.26.1
	github.com/aws/aws-sdk-go-v2/credentials v1.16.12
	github.com/aws/aws-sdk-go-v2/service/s3 v1.47.5
	github.com/libp2p/go-libp2p v0.32.0
	github.com/libp2p/go-libp2p-kad-dht v0.24.0
	github.com/libp2p/go-libp2p-pubsub v0.10.0
	github.com/vzahanych/view-guard-meta/proto/go v0.0.0
	golang.zx2c4.com/wireguard/wgctrl v0.0.0-20230429193321-2a6f49fa3db7
	google.golang.org/grpc v1.60.1
	google.golang.org/protobuf v1.32.0
	gopkg.in/yaml.v3 v3.0.1
	go.uber.org/zap v1.26.0
	modernc.org/sqlite v1.28.0
)

replace github.com/vzahanych/view-guard-meta/proto/go => ../../proto/go

