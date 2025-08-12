package grid

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"

	"github.com/pkg/errors"
	"github.com/scottyeager/tfgrid-sdk-go/grid-client/deployer"
	"github.com/threefoldtech/quantum-storage/quantumd/internal/config"
	"github.com/threefoldtech/tfgrid-sdk-go/grid-proxy/pkg/types"
)

// newNodePool creates a helper for managing available nodes for deployment.
func NewNodePool(cfg *config.Config, gridClient *deployer.TFPluginClient, existing map[uint32]string) *NodePool {
	return &NodePool{
		cfg:        cfg,
		gridClient: gridClient,
		Used:       existing,
		mu:         sync.Mutex{},
	}
}

type NodePool struct {
	cfg        *config.Config
	gridClient *deployer.TFPluginClient
	Used       map[uint32]string // nodeID -> type
	mu         sync.Mutex
}

func (p *NodePool) Get(count int) ([]uint32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.cfg.Farms) == 0 {
		return nil, errors.New("no farms configured for automatic node selection")
	}

	// Collect all nodes that have been used in any capacity
	excludedNodes := make(map[uint32]bool)
	for nodeID := range p.Used {
		excludedNodes[nodeID] = true
	}
	for _, nodeID := range p.cfg.ExcludeNodes {
		excludedNodes[nodeID] = true
	}

	// Fetch available nodes from farms
	hru := uint64(p.cfg.DataSizeGb) * 1024 * 1024 * 1024 // Use data size for filtering
	availableNodes, err := p.getNodesFromFarms(p.cfg.Farms, hru, 0)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get nodes from farms")
	}

	// Filter out excluded nodes
	candidates := []uint32{}
	for _, nodeID := range availableNodes {
		if !excludedNodes[nodeID] {
			candidates = append(candidates, nodeID)
		}
	}

	if len(candidates) < count {
		return nil, fmt.Errorf("not enough available nodes in farms, needed %d, found %d", count, len(candidates))
	}

	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	return candidates[:count], nil
}

func (p *NodePool) MarkUsed(nodeID uint32, nodeType string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Used[nodeID] = nodeType
}

func (p *NodePool) getNodesFromFarms(farmIDs []uint64, hru, sru uint64) ([]uint32, error) {
	rentedFalse := false
	filter := types.NodeFilter{
		FarmIDs: farmIDs,
		FreeSRU: &sru,
		FreeHRU: &hru,
		Rented:  &rentedFalse,
		Status:  []string{"up"},
	}

	nodes, _, err := p.gridClient.GridProxyClient.Nodes(context.Background(), filter, types.Limit{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to query nodes from grid proxy")
	}

	var nodeIDs []uint32
	for _, node := range nodes {
		nodeIDs = append(nodeIDs, uint32(node.NodeID))
	}

	return nodeIDs, nil
}
