package v1

import (
	"archive/zip"
	"bytes"
	"fmt"
	"testing"

	"devt.de/krotik/common/errorutil"
	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/api"
	"devt.de/krotik/rufs/config"
)

func TestZipDownload(t *testing.T) {
	queryURL := "http://localhost" + TESTPORT + EndpointDir
	zipURL := "http://localhost" + TESTPORT + EndpointZip

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

	st, _, res := sendTestRequest(queryURL+"Hans1?recursive=1", "GET", nil)
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
  ],
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

	st, _, res = sendTestRequest(zipURL+"Hans1", "FORMPOST",
		[]byte(`files=["/test1", "/test2", "/sub1/test3"]`))
	if st != "200 OK" {
		t.Error("Unexpected response:", st, res)
		return
	}

	r, err := zip.NewReader(bytes.NewReader([]byte(res)), int64(len(res)))
	if err != nil {
		t.Error(err)
		return
	}

	if res := len(r.File); res != 3 {
		t.Error("Unexpected result:", res)
		return
	}

	if r.File[0].Name != "/test1" {
		t.Error("Unexpected result:", r.File[0].Name)
		return
	}

	if r.File[1].Name != "/test2" {
		t.Error("Unexpected result:", r.File[1].Name)
		return
	}

	if r.File[2].Name != "/sub1/test3" {
		t.Error("Unexpected result:", r.File[2].Name)
		return
	}

	if r.File[2].UncompressedSize != 17 {
		t.Error("Unexpected result:", r.File[2].UncompressedSize)
		return
	}

	// Test error cases

	st, _, res = sendTestRequest(zipURL, "POST", nil)
	if st != "400 Bad Request" || res != "Need a tree name" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(zipURL+"dave", "POST", nil)
	if st != "400 Bad Request" || res != "Unknown tree: dave" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(zipURL+"Hans1", "POST", nil)
	if st != "400 Bad Request" || res != "Could not decode request body: Field 'files' should be a list of files as JSON encoded string" {
		t.Error("Unexpected response:", st, res)
		return
	}
}
