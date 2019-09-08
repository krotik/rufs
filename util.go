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
	"io"

	"devt.de/krotik/rufs/node"
)

/*
IsEOF tests if the given error is an EOF error.
*/
func IsEOF(err error) bool {

	if err == io.EOF {
		return true
	}

	if rerr, ok := err.(*node.Error); ok {
		return rerr.Detail == io.EOF.Error()
	}

	return false
}
