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

type NodePool struct {
	cfg        *config.Config
	gridClient *deployer.TFPluginClient
	metaNodes  map[uint32]struct{}
	dataNodes  map[uint32]struct{}
	mu         sync.Mutex
}

// newNodePool creates a helper for managing available nodes for deployment.
func NewNodePool(cfg *config.Config, gridClient *deployer.TFPluginClient, metaNodes []uint32, dataNodes []uint32) *NodePool {
	metaMap := make(map[uint32]struct{})
	for _, node := range metaNodes {
		metaMap[node] = struct{}{}
	}
	dataMap := make(map[uint32]struct{})
	for _, node := range dataNodes {
		dataMap[node] = struct{}{}
	}
	return &NodePool{
		cfg:        cfg,
		gridClient: gridClient,
		metaNodes:  metaMap,
		dataNodes:  dataMap,
		mu:         sync.Mutex{},
	}
}

func (p *NodePool) Get(count int) ([]uint32, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.cfg.Farms) == 0 {
		return nil, errors.New("no farms configured for automatic node selection")
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
		if _, ok := p.metaNodes[nodeID]; ok {
			continue
		}
		if _, ok := p.dataNodes[nodeID]; ok {
			continue
		}
		candidates = append(candidates, nodeID)
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
	if nodeType == "meta" {
		p.metaNodes[nodeID] = struct{}{}
	} else if nodeType == "data" {
		p.dataNodes[nodeID] = struct{}{}
	}
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

// IsMetaNode returns true if the given node ID is marked as a meta node
func (p *NodePool) IsMetaNode(nodeID uint32) bool {
	_, ok := p.metaNodes[nodeID]
	return ok
}

// IsDataNode returns true if the given node ID is marked as a data node
func (p *NodePool) IsDataNode(nodeID uint32) bool {
	_, ok := p.dataNodes[nodeID]
	return ok
}

// IsUsed returns true if the given node ID is marked as either a meta or data node
func (p *NodePool) IsUsed(nodeID uint32) bool {
	return p.IsMetaNode(nodeID) || p.IsDataNode(nodeID)
}
