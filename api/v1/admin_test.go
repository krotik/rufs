/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

package v1

import (
	"fmt"
	"testing"

	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/api"
	"devt.de/krotik/rufs/config"
)

func TestAdminQuery(t *testing.T) {
	queryURL := "http://localhost" + TESTPORT + EndpointAdmin

	defer func() {

		// Make sure all trees are removed

		api.ResetTrees()
	}()

	// In the beginning there should be no trees

	st, _, res := sendTestRequest(queryURL, "GET", nil)
	if st != "200 OK" || res != "{}" {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Create a new tree

	st, _, res = sendTestRequest(queryURL, "POST", []byte("\"Hans1\""))
	if st != "200 OK" || res != "" {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Check a new tree was created

	st, _, res = sendTestRequest(queryURL+"?refresh=Hans1", "GET", nil)
	if st != "200 OK" || res != `
{
  "Hans1": {
    "branches": [],
    "tree": []
  }
}`[1:] {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Add a new branch

	fooRPC := fmt.Sprintf("%v:%v", branchConfigs["footest"][config.RPCHost], branchConfigs["footest"][config.RPCPort])
	fooFP := footest.SSLFingerprint()

	st, _, res = sendTestRequest(queryURL+"Hans1/branch", "POST", []byte(fmt.Sprintf(`
{
	"branch" : "footest",
	"rpc"    : %#v,
	"fingerprint" : %#v
}`, fooRPC, fooFP)))
	if st != "200 OK" || res != "" {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Check the branch was added

	st, _, res = sendTestRequest(queryURL, "GET", nil)
	if st != "200 OK" || res != fmt.Sprintf(`
{
  "Hans1": {
    "branches": [
      {
        "branch": "footest",
        "fingerprint": %#v,
        "rpc": %#v
      }
    ],
    "tree": []
  }
}`[1:], fooFP, fooRPC) {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Add a new mapping

	st, _, res = sendTestRequest(queryURL+"Hans1/mapping", "POST", []byte(`
{
	"dir" : "/",
	"branch" : "footest",
	"writeable" : false
}`))
	if st != "200 OK" || res != "" {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Check the mapping was added

	st, _, res = sendTestRequest(queryURL, "GET", nil)
	if st != "200 OK" || res != fmt.Sprintf(`
{
  "Hans1": {
    "branches": [
      {
        "branch": "footest",
        "fingerprint": %#v,
        "rpc": %#v
      }
    ],
    "tree": [
      {
        "branch": "footest",
        "path": "/",
        "writeable": false
      }
    ]
  }
}`[1:], fooFP, fooRPC) {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Test error cases

	st, _, res = sendTestRequest(queryURL, "POST", []byte(`""`))
	if st != "400 Bad Request" || res != "Body must contain the tree name as a non-empty JSON string" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL, "POST", []byte(`x`))
	if st != "400 Bad Request" || res != "Could not decode request body: invalid character 'x' looking for beginning of value" {
		t.Error("Unexpected response:", st, res)
		return
	}

	origConfig := api.TreeConfigTemplate
	api.TreeConfigTemplate = nil

	st, _, res = sendTestRequest(queryURL, "POST", []byte(`"xx"`))
	if st != "400 Bad Request" || res != "Could not create new tree: Missing TreeSecret key in tree config" {
		api.TreeConfigTemplate = origConfig
		t.Error("Unexpected response:", st, res)
		return
	}

	api.TreeConfigTemplate = origConfig

	// Test error cases

	origTrees := api.Trees

	api.Trees = func() (map[string]*rufs.Tree, error) {
		return nil, fmt.Errorf("Testerror")
	}

	st, _, res = sendTestRequest(queryURL, "GET", nil)
	if st != "400 Bad Request" || res != "Testerror" {
		api.Trees = origTrees
		t.Error("Unexpected response:", st, res)
		return
	}

	api.Trees = origTrees

	st, _, res = sendTestRequest(queryURL, "POST", []byte("\"Hans1\""))
	if st != "400 Bad Request" || res != "Could not add new tree: Tree Hans1 already exists" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans2/mapping", "POST", nil)
	if st != "400 Bad Request" || res != "Unknown tree: Hans2" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1/mapping", "POST", []byte("aaa"))
	if st != "400 Bad Request" || res != "Could not decode request body: invalid character 'a' looking for beginning of value" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans2/", "POST", []byte("aaa"))
	if st != "400 Bad Request" || res != "Need a tree name and a section (either branches or mapping)" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1/branch", "POST", []byte(fmt.Sprintf(`
{
	"branch" : "footest",
	"rpc"    : %#v,
	"fingerprint" : %#v
}`, fooRPC, fooFP)))
	if st != "400 Bad Request" || res != "Could not add branch: Peer already registered: footest" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1/branch", "POST", []byte(fmt.Sprintf(`
{
	"branch" : "footest",
	"rpc2"    : %#v,
	"fingerprint" : %#v
}`, fooRPC, fooFP)))
	if st != "400 Bad Request" || res != "Value for rpc is missing in posted data" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1/mapping", "POST", []byte(`
{
	"dir" : "/",
	"branch" : "footest2",
	"writeable" : false
}`))
	if st != "400 Bad Request" || res != "Could not add branch: Unknown target node" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1/mapping", "POST", []byte(`
{
	"dir" : "/",
	"branch" : "footest2",
	"writeable" : "test"
}`))
	if st != "400 Bad Request" || res != "Writeable value must be a boolean: strconv.ParseBool: parsing \"test\": invalid syntax" {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Delete twice

	if trees, err := api.Trees(); len(trees) != 1 || err != nil {
		t.Error("Unexpected result:", trees, err)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1", "DELETE", nil)
	if st != "200 OK" || res != "" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL, "DELETE", nil)
	if st != "400 Bad Request" || res != "Need a tree name" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1/meyer", "DELETE", nil)
	if st != "400 Bad Request" || res != "Invalid resource specification: meyer" {
		t.Error("Unexpected response:", st, res)
		return
	}

	if trees, err := api.Trees(); len(trees) != 0 || err != nil {
		t.Error("Unexpected result:", trees, err)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1", "DELETE", nil)
	if st != "400 Bad Request" || res != "Could not remove tree: Tree Hans1 does not exist" {
		t.Error("Unexpected response:", st, res)
		return
	}
}
