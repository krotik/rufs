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
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"

	"devt.de/krotik/common/datautil"
	"devt.de/krotik/common/errorutil"
	"devt.de/krotik/common/fileutil"
	"devt.de/krotik/common/termutil"
	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/config"
	"devt.de/krotik/rufs/term"
)

/*
DefaultMappingFile is the default mapping file for client trees
*/
const DefaultMappingFile = "rufs.mapping.json"

/*
clientCli handles the client command line.
*/
func clientCli() error {
	var tree *rufs.Tree
	var err error
	var fuseMount, dokanMount, webExport *string

	if runtime.GOOS == "linux" {
		fuseMount = flag.String("fuse-mount", "", "Mount tree as FUSE filesystem at specified path (read-only)")
	}
	if runtime.GOOS == "windows" {
		dokanMount = flag.String("dokan-mount", "", "Mount tree as DOKAN filesystem at specified path (read-only)")
	}

	webExport = flag.String("web", "", "Export the tree through a https interface on the specified host:port")

	secretFile, certDir := commonCliOptions()

	showHelp := flag.Bool("help", false, "Show this help message")

	flag.Usage = func() {
		fmt.Println()
		fmt.Println(fmt.Sprintf("Usage of %s client [mapping file]", os.Args[0]))
		fmt.Println()
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("The mapping file assignes remote branches to the local tree.")
		fmt.Println(fmt.Sprintf("The client tries to load %v if no mapping file is defined.",
			DefaultMappingFile))
		fmt.Println("It starts empty if no mapping file exists. The mapping file")
		fmt.Println("should have the following json format:")
		fmt.Println()
		fmt.Println("{")
		fmt.Println(`  "branches" : [`)
		fmt.Println(`    {`)
		fmt.Println(`      "branch"      : <branch name>,`)
		fmt.Println(`      "rpc"         : <rpc interface>,`)
		fmt.Println(`      "fingerprint" : <fingerprint>`)
		fmt.Println(`    },`)
		fmt.Println("    ...")
		fmt.Println("  ],")
		fmt.Println(`  "tree" : [`)
		fmt.Println(`    {`)
		fmt.Println(`      "path"      : <path>,`)
		fmt.Println(`      "branch"    : <branch name>,`)
		fmt.Println(`      "writeable" : <writable flag>`)
		fmt.Println(`    },`)
		fmt.Println("    ...")
		fmt.Println("  ]")
		fmt.Println("}")
		fmt.Println()
	}

	flag.CommandLine.Parse(os.Args[2:])

	if *showHelp {
		flag.Usage()
		return nil
	}

	// Load secret and ssl certificate

	secret, cert, err := loadSecretAndCert(*secretFile, *certDir)
	errorutil.AssertOk(err)

	// Create config

	cfg := datautil.MergeMaps(config.DefaultTreeConfig)
	delete(cfg, config.TreeSecret)

	cfg[config.TreeSecret] = secret

	// Check for a mapping file

	mappingFile := DefaultMappingFile

	if len(flag.Args()) > 0 {
		mappingFile = flag.Arg(0)
	}

	// Create the tree object

	if tree, err = rufs.NewTree(cfg, cert); err == nil {

		// Load mapping file

		if ok, _ := fileutil.PathExists(mappingFile); ok {
			var conf []byte

			fmt.Println(fmt.Sprintf("Using mapping file: %s", mappingFile))

			if conf, err = ioutil.ReadFile(mappingFile); err == nil {
				tree.SetMapping(string(conf))
			}

		} else if webExport != nil && *webExport != "" {

			err = fmt.Errorf("Need a mapping file when using web export")

		} else if fuseMount != nil && *fuseMount != "" {

			err = fmt.Errorf("Need a mapping file when using FUSE mount")

		} else if dokanMount != nil && *dokanMount != "" {

			err = fmt.Errorf("Need a mapping file when using DOKAN mount")
		}

		if err == nil {

			// Check if we want a file system or a terminal

			if webExport != nil && *webExport != "" {

				err = setupWebExport(webExport, tree, certDir)

			} else if fuseMount != nil && *fuseMount != "" {

				err = setupFuseMount(fuseMount, tree)

			} else if dokanMount != nil && *dokanMount != "" {

				err = setupDokanMount(dokanMount, tree)

			} else {

				// Create the terminal

				tt := term.NewTreeTerm(tree, os.Stdout)

				// Add special store config command only available in the command line version

				tt.AddCmd("storeconfig",
					"storeconfig [local file]", "Store the current tree mapping in a local file",
					func(tt *term.TreeTerm, arg ...string) (string, error) {
						mf := DefaultMappingFile
						if len(arg) > 0 {
							mf = arg[0]
						}
						return "", ioutil.WriteFile(mf, []byte(tree.Config()), 0600)
					})

				// Run the terminal

				clt, err := termutil.NewConsoleLineTerminal(os.Stdout)

				if err == nil {
					isExitLine := func(s string) bool {
						return s == "exit" || s == "q" || s == "quit" || s == "bye" || s == "\x04"
					}

					// Add history functionality

					clt, err = termutil.AddHistoryMixin(clt, ".rufs_client_history",
						func(s string) bool {
							return isExitLine(s)
						})

					if err == nil {
						dictChooser := func(lineWords []string,
							dictCache map[string]termutil.Dict) (termutil.Dict, error) {

							// Simple dict chooser 1st level are available commands
							// 2nd level are the contents of the current directory

							if len(lineWords) <= 1 {
								return dictCache["cmds"], nil
							}

							var suggestions []string

							if _, fis, err := tree.Dir(tt.CurrentDir(), "", false,
								false); err == nil {

								for _, f := range fis[0] {
									suggestions = append(suggestions, f.Name())
								}
							}

							return termutil.NewWordListDict(suggestions), nil
						}

						dict := termutil.NewMultiWordDict(dictChooser, map[string]termutil.Dict{
							"cmds": termutil.NewWordListDict(tt.Cmds()),
						})

						// Add auto complete

						clt, err = termutil.AddAutoCompleteMixin(clt, dict)

						if err == nil {
							if err = clt.StartTerm(); err == nil {
								var line string

								defer clt.StopTerm()

								fmt.Println("Type 'q' or 'quit' to exit the shell and '?' to get help")

								line, err = clt.NextLine()
								for err == nil && !isExitLine(line) {

									// Process the entered line

									res, terr := tt.Run(line)

									if res != "" {
										clt.WriteString(fmt.Sprintln(res))
									}
									if terr != nil {
										clt.WriteString(fmt.Sprintln(terr.Error()))
									}

									line, err = clt.NextLine()
								}
							}
						}
					}
				}
			}
		}
	}

	return err
}
