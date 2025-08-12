package grid

import (
	"time"

	"github.com/scottyeager/tfgrid-sdk-go/grid-client/deployer"
)

func NewGridClient(network string, mnemonic string, relayURL string, rmbTimeout time.Duration) (deployer.TFPluginClient, error) {
	return deployer.NewTFPluginClient(mnemonic,
		deployer.WithRelayURL(relayURL),
		deployer.WithNetwork(network),
		deployer.WithRMBTimeout(int(rmbTimeout.Seconds())),
	)
}
