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

import (
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {

	err := CheckBranchExportConfig(map[string]interface{}{})

	if err == nil || strings.HasPrefix(err.Error(), "Unexpected result: Missing") {
		t.Error("Unexpected result:", err)
		return
	}

	if err = CheckBranchExportConfig(DefaultBranchExportConfig); err != nil {
		t.Error(err)
		return
	}

	err = CheckTreeConfig(map[string]interface{}{})

	if err == nil || strings.HasPrefix(err.Error(), "Unexpected result: Missing") {
		t.Error("Unexpected result:", err)
		return
	}

	if err = CheckTreeConfig(DefaultTreeConfig); err != nil {
		t.Error(err)
		return
	}
}
