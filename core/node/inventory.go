package node

import "web4-v3/core/model"

func (n *Node) Balance(unit model.UnitID) model.Amount {
	n.init()
	return n.Inventory.Get(n.ID, unit)
}

func (n *Node) AddInventory(unit model.UnitID, amount model.Amount) {
	n.init()
	n.Inventory.Add(n.ID, unit, amount)
}

func (n *Node) SubInventory(unit model.UnitID, amount model.Amount) error {
	n.init()
	return n.Inventory.Sub(n.ID, unit, amount)
}
