package node

import "web4-v3/core/model"

func (n *Node) Balance(unit model.UnitID) model.Amount {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.balanceLocked(unit)
}

func (n *Node) AddInventory(unit model.UnitID, amount model.Amount) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.init()
	n.Inventory.Add(n.ID, unit, amount)
}

func (n *Node) SubInventory(unit model.UnitID, amount model.Amount) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.init()
	return n.Inventory.Sub(n.ID, unit, amount)
}

func (n *Node) balanceLocked(unit model.UnitID) model.Amount {
	return n.Inventory.Get(n.ID, unit)
}
