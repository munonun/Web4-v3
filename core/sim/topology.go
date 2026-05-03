package sim

import "sort"

type Topology struct {
	Neighbors map[string][]string
}

func NewTopology(neighbors map[string][]string) Topology {
	copied := make(map[string][]string, len(neighbors))
	for nodeID, ids := range neighbors {
		copied[nodeID] = sortedUnique(ids)
	}

	return Topology{Neighbors: copied}
}

func FullMeshTopology(nodeIDs []string) Topology {
	neighbors := make(map[string][]string, len(nodeIDs))
	ids := sortedUnique(nodeIDs)
	for _, nodeID := range ids {
		for _, other := range ids {
			if other != nodeID {
				neighbors[nodeID] = append(neighbors[nodeID], other)
			}
		}
	}

	return NewTopology(neighbors)
}

func ChainTopology(nodeIDs []string) Topology {
	neighbors := make(map[string][]string, len(nodeIDs))
	ids := sortedUnique(nodeIDs)
	for i, nodeID := range ids {
		if i > 0 {
			neighbors[nodeID] = append(neighbors[nodeID], ids[i-1])
		}
		if i < len(ids)-1 {
			neighbors[nodeID] = append(neighbors[nodeID], ids[i+1])
		}
	}

	return NewTopology(neighbors)
}

func ClusteredTopology(clusters [][]string) Topology {
	neighbors := map[string][]string{}
	for _, cluster := range clusters {
		ids := sortedUnique(cluster)
		for _, nodeID := range ids {
			for _, other := range ids {
				if other != nodeID {
					neighbors[nodeID] = append(neighbors[nodeID], other)
				}
			}
		}
	}

	return NewTopology(neighbors)
}

func (t Topology) Neighborhood(nodeID string, includeSelf bool) []string {
	neighbors := append([]string(nil), t.Neighbors[nodeID]...)
	if includeSelf {
		neighbors = append(neighbors, nodeID)
	}

	return sortedUnique(neighbors)
}

func sortedUnique(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}

	copied := append([]string(nil), ids...)
	sort.Strings(copied)
	out := copied[:0]
	for _, id := range copied {
		if len(out) == 0 || out[len(out)-1] != id {
			out = append(out, id)
		}
	}

	return append([]string(nil), out...)
}
