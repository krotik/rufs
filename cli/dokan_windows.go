/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

package main

import (
	"fmt"

	"devt.de/krotik/rufs"
)

/*
setupDokanMount mounts Rufs as a DOKAN filesystem.
*/
func setupDokanMount(dokanMount *string, tree *rufs.Tree) error {
	var err error

	// Create a FUSE mount

	fmt.Println(fmt.Sprintf("Mounting: %s", *dokanMount))

	// TODO ...

	return err
}
