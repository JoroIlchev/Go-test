package auth

import (
	"slices"

	"marquee/internal/store"
)

type Perm int

const (
	PermView Perm = iota
	PermCreate
	PermEdit
	PermPublish
	// PermAdmin covers managing users, editing the tree and approving publications.
	PermAdmin
)

// Staff reports whether u works for the hub operator (super admin or manager).
func Staff(u *store.User) bool { return u.IsSuper || u.IsManager }

// ManagesClient reports whether u has full rights on the whole client: super
// admins on every client, managers on the ones given to them.
func ManagesClient(u *store.User, clientID int64) bool {
	if u.IsSuper {
		return true
	}
	return u.IsManager && (u.AllClients || slices.Contains(u.Clients, clientID))
}

// Can reports whether u holds p on node n. A role on a node applies to the
// node and everything below it; super admins can do everything everywhere,
// managers everything in their clients.
func Can(u *store.User, n *store.Node, p Perm) bool {
	if n.House {
		return u.IsSuper // the operator's own ads
	}
	if ManagesClient(u, n.ClientID) {
		return true
	}
	if !u.ClientID.Valid || u.ClientID.Int64 != n.ClientID {
		return false
	}
	ancestors := map[int64]bool{}
	for _, id := range n.AncestorIDs() {
		ancestors[id] = true
	}
	for _, r := range u.Roles {
		if !ancestors[r.NodeID] {
			continue
		}
		if r.Role == "admin" {
			return true
		}
		switch p {
		case PermView:
			return true
		case PermCreate:
			if r.CanCreate {
				return true
			}
		case PermEdit:
			if r.CanEdit {
				return true
			}
		case PermPublish:
			if r.CanPublish {
				return true
			}
		}
	}
	return false
}

// NeedsApproval reports whether u's publications on n wait for an admin: true
// unless u is an admin on n's path or holds a publish role there without the
// needs-approval flag.
func NeedsApproval(u *store.User, n *store.Node) bool {
	if ManagesClient(u, n.ClientID) {
		return false
	}
	onPath := map[int64]bool{}
	for _, id := range n.AncestorIDs() {
		onPath[id] = true
	}
	for _, r := range u.Roles {
		if !onPath[r.NodeID] {
			continue
		}
		if r.Role == "admin" || (r.CanPublish && !r.NeedsApproval) {
			return false
		}
	}
	return true
}

// CanManageUser reports whether actor may edit target: same client, and every
// node the target has a role on is under one of actor's admin nodes. Nobody
// but a super admin edits their own roles, so no one can escalate themselves.
// Staff (super admins and managers) are edited only on the team page.
func CanManageUser(actor, target *store.User, nodeOf func(int64) *store.Node) bool {
	if actor.IsSuper {
		return !Staff(target) || actor.ID == target.ID
	}
	if Staff(target) || actor.ID == target.ID {
		return false
	}
	if actor.IsManager {
		return target.ClientID.Valid && ManagesClient(actor, target.ClientID.Int64)
	}
	if !actor.ClientID.Valid || !target.ClientID.Valid || actor.ClientID.Int64 != target.ClientID.Int64 {
		return false
	}
	for _, r := range target.Roles {
		n := nodeOf(r.NodeID)
		if n == nil || !Can(actor, n, PermAdmin) {
			return false
		}
	}
	return true
}
