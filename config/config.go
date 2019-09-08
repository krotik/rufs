/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

package config

import "fmt"

/*
ProductVersion is the current version of Rufs
*/
const ProductVersion = "1.1.0"

/*
Defaut configuration keys
*/
const (

	// Branch configuration (export)

	BranchName     = "BranchName"
	BranchSecret   = "BranchSecret"
	EnableReadOnly = "EnableReadOnly"
	RPCHost        = "RPCHost"
	RPCPort        = "RPCPort"
	LocalFolder    = "LocalFolder"

	// Tree configuration

	TreeSecret = "TreeSecret"
)

/*
DefaultBranchExportConfig is the default configuration for an exported branch
*/
var DefaultBranchExportConfig = map[string]interface{}{
	BranchName:     "",      // Auto name (based on available network interface)
	BranchSecret:   "",      // Secret needs to be provided by the client
	EnableReadOnly: false,   // FS access is readonly for clients
	RPCHost:        "",      // Auto (first available external interface)
	RPCPort:        "9020",  // Communication port for this branch
	LocalFolder:    "share", // Local folder which is being made available
}

/*
DefaultTreeConfig is the default configuration for a tree which imports branches
*/
var DefaultTreeConfig = map[string]interface{}{
	TreeSecret: "", // Secret needs to be provided by the client
}

// Helper functions
// ================

/*
CheckBranchExportConfig checks a given branch export config.
*/
func CheckBranchExportConfig(config map[string]interface{}) error {
	for k := range DefaultBranchExportConfig {
		if _, ok := config[k]; !ok {
			return fmt.Errorf("Missing %v key in branch export config", k)
		}
	}
	return nil
}

/*
CheckTreeConfig checks a given tree config.
*/
func CheckTreeConfig(config map[string]interface{}) error {
	for k := range DefaultTreeConfig {
		if _, ok := config[k]; !ok {
			return fmt.Errorf("Missing %v key in tree config", k)
		}
	}
	return nil
}
