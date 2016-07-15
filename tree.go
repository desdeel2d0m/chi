package chi

// Radix tree implementation below is a based on the original work by
// Armon Dadgar in https://github.com/armon/go-radix/blob/master/radix.go
// (MIT licensed)

import (
	"net/http"
	"sort"
	"strings"
)

// TODO: set the RoutePattern on the RouteContext

type methodTyp int

const (
	mCONNECT methodTyp = 1 << iota
	mDELETE
	mGET
	mHEAD
	mOPTIONS
	mPATCH
	mPOST
	mPUT
	mTRACE

	mALL methodTyp = mCONNECT | mDELETE | mGET | mHEAD | mOPTIONS |
		mPATCH | mPOST | mPUT | mTRACE
)

var methodMap = map[string]methodTyp{
	"CONNECT": mCONNECT,
	"DELETE":  mDELETE,
	"GET":     mGET,
	"HEAD":    mHEAD,
	"OPTIONS": mOPTIONS,
	"PATCH":   mPATCH,
	"POST":    mPOST,
	"PUT":     mPUT,
	"TRACE":   mTRACE,
}

type nodeTyp uint8

const (
	ntStatic   nodeTyp = iota // /home
	ntRegexp                  // /:id([0-9]+) or #id^[0-9]+$
	ntParam                   // /:user
	ntCatchAll                // /api/v1/*
)

// TODO: comment
// TODO: if WalkFn is exported, this needs to be as well, which its better not to.
// I have a few ideas, will massage it later.
type methodHandlers map[methodTyp]http.Handler

// WalkFn is used when walking the tree. Takes a
// key and value, returning if iteration should
// be terminated.

// TODO: .. lets leave it like this for now..
// but we could also just make it
// type WalkFn func(method string, pattern string, handler http.Handler) bool
type WalkFn func(path string, handlers methodHandlers) bool

type node struct {
	typ nodeTyp

	// first byte of the prefix
	label byte

	// prefix is the common prefix we ignore
	prefix string

	// TODO: param name..
	// pname string

	// TODO:
	// subtree *tree

	// HTTP handler on the leaf node
	handlers methodHandlers

	// Child nodes should be stored in-order for iteration,
	// in groups of the node type.
	children [ntCatchAll + 1]nodes
}

func (n *node) Find(ctx *Context, method methodTyp, path string) methodHandlers {
	rn := n.findNode(ctx, method, path)
	if rn == nil {
		return nil
	}
	return rn.handlers
}

func (n *node) Insert(method methodTyp, pattern string, handler http.Handler) {
	var parent *node
	search := pattern

	for {
		// Handle key exhaustion
		if len(search) == 0 {
			// Insert or update the node's leaf handler
			n.setHandler(method, handler)
			return
		}

		// Look for the edge
		parent = n
		n = n.getEdge(search[0])

		// No edge, create one
		if n == nil {
			cn := &node{label: search[0], prefix: search}
			cn.setHandler(method, handler)
			parent.addChild(cn)
			return
		}

		if n.typ > ntStatic {
			// We found a wildcard node, meaning search path starts with
			// a wild prefix. Trim off the wildcard search path and continue.
			p := strings.Index(search, "/")
			if p < 0 {
				p = len(search)
			}
			search = search[p:]
			continue
		}

		// Static nodes fall below here.
		// Determine longest prefix of the search key on match.
		commonPrefix := n.longestPrefix(search, n.prefix)
		if commonPrefix == len(n.prefix) {
			// the common prefix is as long as the current node's prefix we're attempting to insert.
			// keep the search going.
			search = search[commonPrefix:]
			continue
		}

		// Split the node
		child := &node{
			typ:    ntStatic,
			prefix: search[:commonPrefix],
		}
		parent.replaceChild(search[0], child)

		// Restore the existing node
		n.label = n.prefix[commonPrefix]
		n.prefix = n.prefix[commonPrefix:]
		child.addChild(n)

		// If the new key is a subset, add to to this node
		search = search[commonPrefix:]
		if len(search) == 0 {
			child.setHandler(method, handler)
			return
		}

		// Create a new edge for the node
		subchild := &node{
			typ:    ntStatic,
			label:  search[0],
			prefix: search,
		}
		subchild.setHandler(method, handler)
		child.addChild(subchild)
		return
	}
}

func (n *node) isLeaf() bool {
	return n.handlers != nil
}

func (n *node) addChild(child *node) {
	search := child.prefix

	// Find any wildcard segments
	p := strings.IndexAny(search, ":*")

	// Determine new node type
	ntyp := ntStatic
	if p >= 0 {
		switch search[p] {
		case ':':
			ntyp = ntParam
		case '*':
			ntyp = ntCatchAll
		}
	}

	if p == 0 {
		// Path starts with a wildcard

		handlers := child.handlers
		child.typ = ntyp

		if ntyp == ntCatchAll {
			p = -1
		} else {
			p = strings.IndexByte(search, '/')
		}
		if p < 0 {
			p = len(search)
		}
		child.prefix = search[:p]

		if p != len(search) {
			// add edge for the remaining part, split the end.
			child.handlers = nil

			search = search[p:]

			child.addChild(&node{
				typ:      ntStatic,
				label:    search[0], // this will always start with /
				prefix:   search,
				handlers: handlers,
			})
		}

	} else if p > 0 {
		// Path has some wildcard

		// starts with a static segment
		handlers := child.handlers
		child.typ = ntStatic
		child.prefix = search[:p]
		child.handlers = nil

		// add the wild edge node
		search = search[p:]

		child.addChild(&node{
			typ:      ntyp,
			label:    search[0],
			prefix:   search,
			handlers: handlers,
		})

	} else {
		// Path is all static
		child.typ = ntyp

	}

	n.children[child.typ] = append(n.children[child.typ], child)
	n.children[child.typ].Sort()
}

func (n *node) replaceChild(label byte, child *node) {
	for i := 0; i < len(n.children[child.typ]); i++ {
		if n.children[child.typ][i].label == label {
			n.children[child.typ][i] = child
			n.children[child.typ][i].label = label
			return
		}
	}

	panic("chi: replacing missing child")
}

func (n *node) getEdge(label byte) *node {
	for _, nds := range n.children {
		num := len(nds)
		for i := 0; i < num; i++ {
			if nds[i].label == label {
				return nds[i]
			}
		}
	}
	return nil
}

func (n *node) findEdge(ntyp nodeTyp, label byte) *node {
	nds := n.children[ntyp]
	num := len(nds)
	idx := 0

	switch ntyp {
	case ntStatic:
		i, j := 0, num-1
		for i <= j {
			idx = i + (j-i)/2
			if label > nds[idx].label {
				i = idx + 1
			} else if label < nds[idx].label {
				j = idx - 1
			} else {
				i = num // breaks cond
			}
		}
		if nds[idx].label != label {
			return nil
		}
		return nds[idx]

	default: // wild nodes
		// TODO: right now we match them all.. but regexp should
		// run through regexp matcher
		return nds[idx]
	}
}

// Recursive edge traversal by checking all nodeTyp groups along the way.
// It's like searching through a three-dimensional radix trie.
func (n *node) findNode(ctx *Context, method methodTyp, path string) *node {
	nn := n
	search := path

	for t, nds := range nn.children {
		ntyp := nodeTyp(t)
		if len(nds) == 0 {
			continue
		}

		// search subset of edges of the index for a matching node
		var label byte
		if search != "" {
			label = search[0]
		}
		xn := nn.findEdge(ntyp, label) // next node

		if xn == nil {
			continue
		}

		// Prepare next search path by trimming prefix from requested path
		xsearch := search
		if xn.typ > ntStatic {
			p := -1
			if xn.typ < ntCatchAll {
				p = strings.IndexByte(xsearch, '/')
			}
			if p < 0 {
				p = len(xsearch)
			}

			if xn.typ == ntCatchAll {
				ctx.Params.Add("*", xsearch)
			} else {
				ctx.Params.Add(xn.prefix[1:], xsearch[:p])
			}

			xsearch = xsearch[p:]
		} else if strings.HasPrefix(xsearch, xn.prefix) {
			xsearch = xsearch[len(xn.prefix):]
		} else {
			continue // no match
		}

		// did we find it yet?
		if len(xsearch) == 0 {
			if xn.isLeaf() {
				return xn
			}
		}

		// recursively find the next node..
		fin := xn.findNode(ctx, method, xsearch)
		if fin != nil {
			// found a node, return it
			return fin
		}

		// Did not found final handler, let's remove the param here if it was set
		// TODO: can we do even better now though...?
		if xn.typ > ntStatic && xn.typ < ntCatchAll {
			ctx.Params.Del(xn.prefix[1:])
		}
	}

	return nil
}

// longestPrefix finds the length of the shared prefix
// of two strings
func (n *node) longestPrefix(k1, k2 string) int {
	max := len(k1)
	if l := len(k2); l < max {
		max = l
	}
	var i int
	for i = 0; i < max; i++ {
		if k1[i] != k2[i] {
			break
		}
	}
	return i
}

func (n *node) setHandler(method methodTyp, handler http.Handler) {
	if n.handlers == nil {
		n.handlers = make(methodHandlers, 0)
	}
	if method == mALL {
		for _, m := range methodMap {
			n.handlers[m] = handler
		}
	} else {
		n.handlers[method] = handler
	}
}

type nodes []*node

// Sort the list of nodes by label
func (ns nodes) Len() int           { return len(ns) }
func (ns nodes) Less(i, j int) bool { return ns[i].label < ns[j].label }
func (ns nodes) Swap(i, j int)      { ns[i], ns[j] = ns[j], ns[i] }
func (ns nodes) Sort()              { sort.Sort(ns) }

// Walk is used to walk the tree
/*func (t *tree) Walk(fn WalkFn) {
	t.recursiveWalk(t.root.prefix, t.root, fn)
}

// recursiveWalk is used to do a pre-order walk of a node
// recursively. Returns true if the walk should be aborted
func (t *tree) recursiveWalk(pattern string, n *node, fn WalkFn) bool {
	pattern += n.prefix

	// Visit the leaf values if any
	if n.handlers != nil && fn(pattern, n.handlers) {
		return true
	}

	// Recurse on the children
	for _, edges := range n.edges {
		for _, e := range edges {
			if t.recursiveWalk(pattern, e.node, fn) {
				return true
			}
		}
	}
	return false
}
*/
