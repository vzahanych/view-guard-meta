package types

import (
	httpsclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/http-impl/https-client-service/types"
	httpsservertypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/http-impl/https-server-service/types"
	wgclienttypes "github.com/vzahanych/view-guard-meta/edge/orchestrator/internal/vm-gateway/wg-client-service/types"
)

// VMGatewayConfig contains VM gateway configuration
type VMGatewayConfig struct {
	Provider          string                             `yaml:"provider"`
	WireGuard         wgclienttypes.WGClientConfig       `yaml:"wireguard"`
	HTTPServerConfig  httpsservertypes.HTTPServerConfig  `yaml:"https_server_config"`
	HTTPSClientConfig httpsclienttypes.HTTPSClientConfig `yaml:"https_client_config"`
	EdgeID            string                             `yaml:"edge_id"`
}
