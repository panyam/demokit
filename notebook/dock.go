package notebook

// Docked cells are positioned slots that reuse the Cell interface
// but live OUTSIDE the cursor-navigable main list. They're how
// apps add vim-style command bars, status rows, breadcrumbs, and
// per-cell annotations without forking the cell type or
// duplicating the input-handling story.
//
// v1 ships two families of Position:
//
//   - Edges (Top, Bottom) — viewport-pinned chrome. Always visible,
//     screen-relative; render outside the body window.
//   - Cell-anchored (After(id), Before(id)) — annotations that
//     belong to a specific main cell; participate in the body row
//     flow and scroll with their anchor. Auto-unregister when
//     Remove(id) fires on the anchor.
//
// One Cell per Position: a second SetDockedCell at the same
// Position replaces the first. Apps that want richer chrome (e.g.
// multiple status segments) compose them into a single Cell that
// renders multiple lines.
//
// Architectural commitment: docked cells live in a separate
// registry, NOT in the main cells[] slice. This preserves the
// invariant that cells[N] is always the N-th cursor-navigable
// cell. Iteration over the main list stays linear and dock-free.

// Position identifies a docked-cell slot. Construct one via the
// package-level Top / Bottom values or the After / Before
// functions; Position values are comparable and used as map keys
// inside the notebook's dock registry.
type Position interface {
	// positionKey returns the registry key for this Position.
	// Unexported so external packages can't fabricate Positions —
	// they must go through the package-provided constructors.
	positionKey() positionKey
}

// positionKey is the comparable value the dock registry maps from.
// edge=0 means cell-anchored (cellID + before/after); edge!=0
// means an edge position (cellID is ignored).
type positionKey struct {
	edge   edge
	rel    anchorRel
	cellID CellID
}

type edge int

const (
	edgeNone edge = iota
	edgeTop
	edgeBottom
)

type anchorRel int

const (
	relNone anchorRel = iota
	relAfter
	relBefore
)

// edgePosition implements Position for viewport-pinned chrome.
type edgePosition struct{ e edge }

func (p edgePosition) positionKey() positionKey { return positionKey{edge: p.e} }

// cellAnchor implements Position for cell-anchored docks. Two
// anchors on the same cell with different rel produce different
// keys; the same (rel, cellID) pair always produces equal keys.
type cellAnchor struct {
	rel    anchorRel
	cellID CellID
}

func (p cellAnchor) positionKey() positionKey {
	return positionKey{rel: p.rel, cellID: p.cellID}
}

// Top is the viewport-pinned slot above the body. Apps use it for
// breadcrumbs, tabs, or persistent banners. No default occupant —
// the slot is empty until SetDockedCell(Top, ...) is called.
var Top Position = edgePosition{edgeTop}

// Bottom is the viewport-pinned slot below the body. The notebook
// installs a built-in StatusCell here at construction so the
// existing "MODE  cell N/M" line keeps working without app code.
// Apps replace it via SetDockedCell(Bottom, ...) and can restore
// the default with SetDockedCell(Bottom, NewStatusCell()).
var Bottom Position = edgePosition{edgeBottom}

// After returns the Position immediately following the cell with
// the given ID in the body flow. Auto-unregisters when the anchor
// cell is removed via nb.Remove(id).
func After(id CellID) Position { return cellAnchor{rel: relAfter, cellID: id} }

// Before returns the Position immediately preceding the cell with
// the given ID in the body flow. Auto-unregisters when the anchor
// cell is removed via nb.Remove(id).
func Before(id CellID) Position { return cellAnchor{rel: relBefore, cellID: id} }

// isEdge reports whether p is one of the viewport-pinned slots.
func isEdge(p Position) bool {
	k := p.positionKey()
	return k.edge != edgeNone
}

// isCellAnchored reports whether p tracks a specific cell ID.
func isCellAnchored(p Position) bool {
	k := p.positionKey()
	return k.rel != relNone
}
