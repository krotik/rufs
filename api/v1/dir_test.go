package v1

import (
	"fmt"
	"testing"

	"devt.de/krotik/common/errorutil"
	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/api"
	"devt.de/krotik/rufs/config"
)

func TestDirQuery(t *testing.T) {
	adminQueryURL := "http://localhost" + TESTPORT + EndpointAdmin

	queryURL := "http://localhost" + TESTPORT + EndpointDir

	// Setup a tree

	defer func() {

		// Make sure all trees are removed

		api.ResetTrees()
	}()

	tree, err := rufs.NewTree(api.TreeConfigTemplate, api.TreeCertTemplate)
	errorutil.AssertOk(err)

	api.AddTree("Hans1", tree)

	fooRPC := fmt.Sprintf("%v:%v", branchConfigs["footest"][config.RPCHost], branchConfigs["footest"][config.RPCPort])
	fooFP := footest.SSLFingerprint()

	err = tree.AddBranch("footest", fooRPC, fooFP)
	errorutil.AssertOk(err)

	err = tree.AddMapping("/", "footest", false)
	errorutil.AssertOk(err)

	st, _, res := sendTestRequest(adminQueryURL, "GET", nil)
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

	// Get a directory listing

	st, _, res = sendTestRequest(queryURL+"Hans1", "GET", nil)
	if st != "200 OK" || res != `
{
  "/": [
    {
      "isdir": true,
      "name": "sub1",
      "size": 4096
    },
    {
      "isdir": false,
      "name": "test1",
      "size": 10
    },
    {
      "isdir": false,
      "name": "test2",
      "size": 10
    }
  ]
}`[1:] {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1/sub1", "GET", nil)
	if st != "200 OK" || res != `
{
  "/sub1": [
    {
      "isdir": false,
      "name": "test3",
      "size": 17
    }
  ]
}`[1:] {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Test recursive with checksums

	st, _, res = sendTestRequest(queryURL+"Hans1?recursive=TRUE&checksums=1", "GET", nil)
	if st != "200 OK" || res != `
{
  "/": [
    {
      "checksum": "",
      "isdir": true,
      "name": "sub1",
      "size": 4096
    },
    {
      "checksum": "73b8af47",
      "isdir": false,
      "name": "test1",
      "size": 10
    },
    {
      "checksum": "b0c1fadd",
      "isdir": false,
      "name": "test2",
      "size": 10
    }
  ],
  "/sub1": [
    {
      "checksum": "f89782b1",
      "isdir": false,
      "name": "test3",
      "size": 17
    }
  ]
}`[1:] {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Test error cases

	st, _, res = sendTestRequest(queryURL, "GET", nil)
	if st != "400 Bad Request" || res != "Need at least a tree name" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"dave", "GET", nil)
	if st != "400 Bad Request" || res != "Unknown tree: dave" {
		t.Error("Unexpected response:", st, res)
		return
	}
}
