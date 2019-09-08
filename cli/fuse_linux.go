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
	"os"
	"os/signal"
	"syscall"

	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/export"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/hanwen/go-fuse/v2/fuse/nodefs"
	"github.com/hanwen/go-fuse/v2/fuse/pathfs"
)

/*
setupFuseMount mounts Rufs as a FUSE filesystem.
*/
func setupFuseMount(fuseMount *string, tree *rufs.Tree) error {
	var err error
	var server *fuse.Server

	// Create a FUSE mount

	fmt.Println(fmt.Sprintf("Mounting: %s", *fuseMount))

	// Set up FUSE server

	nfs := pathfs.NewPathNodeFs(&export.RufsFuse{
		FileSystem: pathfs.NewDefaultFileSystem(),
		Tree:       tree,
	}, nil)

	if server, _, err = nodefs.MountRoot(*fuseMount, nfs.Root(), nil); err != nil {
		return err
	}

	// Add an unmount handler

	// Attach SIGINT handler - on unix and windows this is send
	// when the user presses ^C (Control-C).

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, syscall.SIGINT)

	go func() {
		for true {
			signal := <-sigchan

			if signal == syscall.SIGINT {
				fmt.Println(fmt.Sprintf("Unmounting: %s", *fuseMount))
				err := server.Unmount()
				if err != nil {
					fmt.Println(err)
				}
				break
			}
		}
	}()

	// Run FUSE server

	server.Serve()

	return err
}
