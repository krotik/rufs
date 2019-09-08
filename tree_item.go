/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

package rufs

import (
	"bytes"
	"fmt"
	"path"
	"sort"

	"devt.de/krotik/common/errorutil"
	"devt.de/krotik/common/stringutil"
)

/*
treeItem models an item in the tree. This is an internal data structure
which is not exposed.
*/
type treeItem struct {
	children            map[string]*treeItem // Mapping from path component to branch
	remoteBranches      []string             // List of remote branches which are present on this level
	remoteBranchWriting []bool               // Flag if the remote branch should receive write requests
}

/*
findPathBranches finds all relevant branches for a single path. The iterator
function receives 4 parameters: The tree item, the total path within the tree,
the subpath within the branch and a list of all branches for the tree path.
Calling code should always give a treePath of "/".
*/
func (t *treeItem) findPathBranches(treePath string, branchPath []string,
	recursive bool, visit func(*treeItem, string, []string, []string, []bool)) {

	visit(t, treePath, branchPath, t.remoteBranches, t.remoteBranchWriting)

	if len(branchPath) > 0 {

		if c, ok := t.children[branchPath[0]]; ok {

			// Check if a subpath matches

			c.findPathBranches(path.Join(treePath, branchPath[0]),
				branchPath[1:], recursive, visit)
		}

	} else if recursive {
		var childNames []string

		for n := range t.children {
			childNames = append(childNames, n)
		}

		sort.Strings(childNames)

		for _, n := range childNames {
			t.children[n].findPathBranches(path.Join(treePath, n),
				branchPath, recursive, visit)
		}
	}
}

/*
addMapping adds a new mapping.
*/
func (t *treeItem) addMapping(mappingPath []string, branchName string, writable bool) {

	// Add mapping to a child

	if len(mappingPath) > 0 {

		childName := mappingPath[0]
		rest := mappingPath[1:]

		errorutil.AssertTrue(childName != "",
			"Adding a mapping with an empty path is not supported")

		// Ensure child exists

		child, ok := t.children[childName]
		if !ok {
			child = &treeItem{make(map[string]*treeItem), []string{}, []bool{}}
			t.children[childName] = child
		}

		// Add rest of the mapping to the child

		child.addMapping(rest, branchName, writable)

		return
	}

	// Add branch name to this branch - keep the order in which the branches were added

	t.remoteBranches = append(t.remoteBranches, branchName)
	t.remoteBranchWriting = append(t.remoteBranchWriting, writable)
}

/*
String returns a string representation of this item and its children.
*/
func (t *treeItem) String(indent int, buf *bytes.Buffer) {

	for i, b := range t.remoteBranches {
		buf.WriteString(b)

		if t.remoteBranchWriting[i] {
			buf.WriteString("(w)")
		} else {
			buf.WriteString("(r)")
		}

		if i < len(t.remoteBranches)-1 {
			buf.WriteString(", ")
		}
	}
	buf.WriteString("\n")

	names := make([]string, 0, len(t.children))
	for n := range t.children {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, n := range names {
		i := t.children[n]
		buf.WriteString(stringutil.GenerateRollingString(" ", indent*2))
		buf.WriteString(fmt.Sprintf("%v/: ", n))
		i.String(indent+1, buf)
	}
}
