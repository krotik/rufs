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
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"devt.de/krotik/common/datautil"
	"devt.de/krotik/common/errorutil"
	"devt.de/krotik/common/fileutil"
	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/config"
)

/*
DefaultServerConfigFile is the default config file when running in server mode
*/
const DefaultServerConfigFile = "rufs.server.json"

/*
serverCli handles the server command line.
*/
func serverCli() error {
	var err error

	serverConfigFile := flag.String("config", DefaultServerConfigFile, "Server configuration file")

	secretFile, certDir := commonCliOptions()

	showHelp := flag.Bool("help", false, "Show this help message")

	flag.Usage = func() {
		fmt.Println()
		fmt.Println(fmt.Sprintf("Usage of %s server [options]", os.Args[0]))
		fmt.Println()
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("The server will automatically create a default config file and")
		fmt.Println("default directories if nothing is specified.")
		fmt.Println()
	}

	flag.CommandLine.Parse(os.Args[2:])

	if *showHelp {
		flag.Usage()
		return nil
	}

	// Load secret and ssl certificate

	secret, cert, err := loadSecretAndCert(*secretFile, *certDir)

	if err == nil {

		// Load configuration

		var cfg map[string]interface{}

		defaultConfig := datautil.MergeMaps(config.DefaultBranchExportConfig)
		delete(defaultConfig, config.BranchSecret)

		// Set environment specific values for default config

		if ip, lerr := externalIP(); lerr == nil {
			defaultConfig[config.BranchName] = ip
			defaultConfig[config.RPCHost] = ip
		}

		cfg, err = fileutil.LoadConfig(*serverConfigFile, defaultConfig)
		errorutil.AssertOk(err)

		cfg[config.BranchSecret] = secret

		fmt.Println(fmt.Sprintf("Using config: %s", *serverConfigFile))

		// Ensure the local shared folder actually exists

		if ok, _ := fileutil.PathExists(cfg[config.LocalFolder].(string)); !ok {
			os.MkdirAll(cfg[config.LocalFolder].(string), 0777)
		}

		absLocalFolder, _ := filepath.Abs(cfg[config.LocalFolder].(string))
		fmt.Println(fmt.Sprintf("Exporting folder: %s", absLocalFolder))

		// We got everything together let's start

		var branch *rufs.Branch

		if branch, err = rufs.NewBranch(cfg, cert); err == nil {

			// Attach SIGINT handler - on unix and windows this is send
			// when the user presses ^C (Control-C).

			sigchan := make(chan os.Signal)
			signal.Notify(sigchan, syscall.SIGINT)

			// Create a wait group to wait for the os signal

			wg := sync.WaitGroup{}

			// Kick off a polling thread which waits for the signal

			go func() {
				for true {
					signal := <-sigchan

					if signal == syscall.SIGINT {

						// Shutdown the branch

						branch.Shutdown()
						break
					}
				}

				// Done waiting main thread can exit

				wg.Done()
			}()

			// Suspend main thread until branch is shutdown

			wg.Add(1)
			wg.Wait()
		}
	}

	return err
}
