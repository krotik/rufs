/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

/*
Package term contains a terminal implementation which can control Rufs trees.
*/
package term

/*
cmdMap contains all available commands
*/
var cmdMap = map[string]func(*TreeTerm, ...string) (string, error){
	"?":        cmdHelp,
	"help":     cmdHelp,
	"cd":       cmdCd,
	"dir":      cmdDir,
	"ll":       cmdDir,
	"checksum": cmdChecksum,
	"tree":     cmdTree,
	"branch":   cmdBranch,
	"mount":    cmdMount,
	"reset":    cmdReset,
	"ping":     cmdPing,
	"cat":      cmdCat,
	"get":      cmdGet,
	"put":      cmdPut,
	"rm":       cmdRm,
	"ren":      cmdRen,
	"mkdir":    cmdMkDir,
	"cp":       cmdCp,
	"sync":     cmdSync,
	"refresh":  cmdRefresh,
}

var helpMap = map[string]string{
	"help [cmd]":             "Show general or command specific help",
	"cd [path]":              "Show or change the current directory",
	"dir [path] [glob]":      "Show a directory listing",
	"checksum [path] [glob]": "Show a directory listing and file checksums",
	"tree [path] [glob]":     "Show the listing of a directory and its subdirectories",
	"branch [branch name] [rpc] [fingerprint]": "List all known branches or add a new branch to the tree",
	"mount [path] [branch name] [ro]":          "List all mount points or add a new mount point to the tree",
	"reset [mounts|brances]":                   "Remove all mounts or all mounts and all branches",
	"ping <branch name> [rpc]":                 "Ping a remote branch",
	"cat <file>":                               "Read and print the contents of a file",
	"get <src file> [dst local file]":          "Retrieve a file and store it locally (in the current directory)",
	"put [src local file] [dst file]":          "Read a local file and store it",
	"rm <file>":                                "Delete a file or directory (* all files; ** all files/recursive)",
	"ren <file> <newfile>":                     "Rename a file or directory",
	"mkdir <dir>":                              "Create a new directory",
	"cp <src file/dir> <dst dir>":              "Copy a file or directory",
	"sync <src dir> <dst dir>":                 "Make sure dst has the same files and directories as src",
	"refresh":                                  "Refreshes all known branches and reconnect if possible",
}
