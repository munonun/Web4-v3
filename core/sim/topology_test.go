package sim

import (
	"reflect"
	"testing"
)

func TestFullMeshTopology(t *testing.T) {
	topology := FullMeshTopology([]string{"C", "A", "B"})

	assertNeighborhood(t, topology.Neighborhood("A", false), []string{"B", "C"})
	assertNeighborhood(t, topology.Neighborhood("A", true), []string{"A", "B", "C"})
}

func TestChainTopology(t *testing.T) {
	topology := ChainTopology([]string{"A", "B", "C"})

	assertNeighborhood(t, topology.Neighborhood("A", true), []string{"A", "B"})
	assertNeighborhood(t, topology.Neighborhood("B", true), []string{"A", "B", "C"})
	assertNeighborhood(t, topology.Neighborhood("C", true), []string{"B", "C"})
}

func TestClusteredTopology(t *testing.T) {
	topology := ClusteredTopology([][]string{{"A", "B"}, {"C", "D"}})

	assertNeighborhood(t, topology.Neighborhood("A", true), []string{"A", "B"})
	assertNeighborhood(t, topology.Neighborhood("C", true), []string{"C", "D"})
}

func TestNeighborhoodsAreDeterministicSorted(t *testing.T) {
	topology := NewTopology(map[string][]string{"A": {"D", "B", "C", "B"}})

	assertNeighborhood(t, topology.Neighborhood("A", true), []string{"A", "B", "C", "D"})
}

func TestTopologyConstructorCopiesInputs(t *testing.T) {
	neighbors := map[string][]string{"A": {"B"}}
	topology := NewTopology(neighbors)
	neighbors["A"][0] = "C"
	topology.Neighbors["A"][0] = "D"

	copyTopology := NewTopology(neighbors)
	assertNeighborhood(t, copyTopology.Neighborhood("A", false), []string{"C"})
}

func TestUnknownNodeNeighborhood(t *testing.T) {
	topology := NewTopology(nil)

	assertNeighborhood(t, topology.Neighborhood("Z", true), []string{"Z"})
	assertNeighborhood(t, topology.Neighborhood("Z", false), nil)
}

func assertNeighborhood(t *testing.T, got []string, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("neighborhood = %#v, want %#v", got, want)
	}
}
