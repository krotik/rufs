package v1

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"devt.de/krotik/common/datautil"
	"devt.de/krotik/common/errorutil"
	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/api"
	"devt.de/krotik/rufs/config"
)

func TestFileSync(t *testing.T) {

	queryURL := "http://localhost" + TESTPORT + EndpointFile
	dirqueryURL := "http://localhost" + TESTPORT + EndpointDir
	progressqueryURL := "http://localhost" + TESTPORT + EndpointProgress

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

	err = tree.AddMapping("/", "footest", true)
	errorutil.AssertOk(err)

	// Make the target directory

	os.Mkdir("foo/sync1", 0755)
	defer func() {
		errorutil.AssertOk(os.RemoveAll("foo/sync1"))
	}()

	st, _, res := sendTestRequest(dirqueryURL+"Hans1?recursive=TRUE&checksums=1", "GET", nil)

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
      "checksum": "",
      "isdir": true,
      "name": "sync1",
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
  ],
  "/sync1": null
}`[1:] {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Do the sync

	st, _, res = sendTestRequest(queryURL+"Hans1/", "PUT", []byte(`
{
    "action" : "sync",
	"destination" : "/sync1"
}`))
	if st != "200 OK" {
		t.Error("Unexpected response:", st, res)
		return
	}

	var resMap map[string]interface{}
	json.Unmarshal([]byte(res), &resMap)

	pid := fmt.Sprint(resMap["progress_id"])

	for {

		st, _, res = sendTestRequest(progressqueryURL+"Hans1/"+pid, "GET", nil)
		json.Unmarshal([]byte(res), &resMap)

		if st != "200 OK" ||
			len(resMap["errors"].([]interface{})) > 0 ||
			(resMap["progress"] == resMap["total_progress"] && resMap["item"] == resMap["total_items"]) {

			break
		}
	}

	if st != "200 OK" || res != `
{
  "errors": [],
  "item": 5,
  "operation": "Copy file",
  "progress": 17,
  "subject": "/sub1/test3",
  "total_items": 5,
  "total_progress": 17
}`[1:] {
		t.Error("Unexpected response:", st, res)
		return
	}

	time.Sleep(5 * time.Millisecond) // Wait a small time so all operations are guaranteed to finish

	st, _, res = sendTestRequest(dirqueryURL+"Hans1?recursive=TRUE&checksums=1", "GET", nil)

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
      "checksum": "",
      "isdir": true,
      "name": "sync1",
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
  ],
  "/sync1": [
    {
      "checksum": "",
      "isdir": true,
      "name": "sub1",
      "size": 4096
    },
    {
      "checksum": "",
      "isdir": true,
      "name": "sync1",
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
  "/sync1/sub1": [
    {
      "checksum": "f89782b1",
      "isdir": false,
      "name": "test3",
      "size": 17
    }
  ],
  "/sync1/sync1": null
}`[1:] {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Test errors

	st, _, res = sendTestRequest(queryURL+"Hans1/", "PUT", []byte(`
{
    "action" : "sync"
}`))
	if st != "400 Bad Request" || res != "Parameter destination is missing from request body" {
		t.Error("Unexpected response:", st, res)
		return
	}

	tree.Reset(false)
	ProgressMap = datautil.NewMapCache(100, 0)

	err = tree.AddMapping("/", "footest", false)
	errorutil.AssertOk(err)

	st, _, res = sendTestRequest(queryURL+"Hans1/", "PUT", []byte(`
{
    "action" : "sync",
	"destination" : "/sync1"
}`))
	if st != "400 Bad Request" || res != "All applicable branches for the requested path were mounted as not writable" {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Check error in ProgressMap

	for k := range ProgressMap.GetAll() {
		k = strings.Split(k, "#")[1]
		_, _, res = sendTestRequest(progressqueryURL+"Hans1/"+k, "GET", nil)
		resMap = nil
		errorutil.AssertOk(json.Unmarshal([]byte(res), &resMap))

		if res := fmt.Sprint(resMap["errors"]); res != "[All applicable branches for the requested path were mounted as not writable]" {
			t.Error("Unexpected result:", res)
		}

		break
	}
}

func TestFileUpload(t *testing.T) {
	queryURL := "http://localhost" + TESTPORT + EndpointFile
	dirqueryURL := "http://localhost" + TESTPORT + EndpointDir

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

	err = tree.AddMapping("/", "footest", true)
	errorutil.AssertOk(err)

	// Send a file

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("uploadfile", "foo-upload.txt")
	errorutil.AssertOk(err)
	part.Write([]byte("footest"))

	part, err = writer.CreateFormFile("uploadfile", "bar-upload.txt")
	errorutil.AssertOk(err)
	part.Write([]byte("bartest"))

	writer.WriteField("redirect", "/foo/bar")

	writer.Close()

	req, err := http.NewRequest("POST", queryURL+"Hans1/", body)
	if err != nil {
		panic(err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	respBody, _ := ioutil.ReadAll(resp.Body)
	respBodyStr := strings.Trim(string(respBody), " \n")

	if l, _ := resp.Location(); resp.Status != "302 Found" || l.String() != "http://localhost:9040/foo/bar" {
		t.Error("Unexpected response:", resp.Status, l.String(), respBodyStr)
		return
	}

	defer func() {

		// Remove the uploaded files after we are done

		os.RemoveAll("foo/foo-upload.txt")
		os.RemoveAll("foo/bar-upload.txt")
	}()

	// Check that the file has been written

	st, _, res := sendTestRequest(dirqueryURL+"Hans1?recursive=TRUE&checksums=1", "GET", nil)
	if st != "200 OK" || res != `
{
  "/": [
    {
      "checksum": "bab796f6",
      "isdir": false,
      "name": "bar-upload.txt",
      "size": 7
    },
    {
      "checksum": "75f4b6fe",
      "isdir": false,
      "name": "foo-upload.txt",
      "size": 7
    },
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

	// Check the files have been written correctly

	txt, err := ioutil.ReadFile("foo/foo-upload.txt")
	if err != nil || string(txt) != "footest" {
		t.Error("Unexpected result:", string(txt), err)
		return
	}

	txt, err = ioutil.ReadFile("foo/bar-upload.txt")
	if err != nil || string(txt) != "bartest" {
		t.Error("Unexpected result:", string(txt), err)
		return
	}

	// Test error cases

	st, _, res = sendTestRequest(queryURL, "POST", nil)
	if st != "400 Bad Request" || res != "Need a tree name and a file path" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans2", "POST", nil)
	if st != "400 Bad Request" || res != "Unknown tree: Hans2" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1", "POST", nil)
	if st != "400 Bad Request" || res != "Could not read request body: request Content-Type isn't multipart/form-data" {
		t.Error("Unexpected response:", st, res)
		return
	}

	writer = multipart.NewWriter(body)

	part, err = writer.CreateFormFile("uploadfile2", "foo-upload.txt")
	if err != nil {
		panic(err)
	}

	errorutil.AssertOk(err)
	part.Write([]byte("footest"))

	writer.Close()

	req, err = http.NewRequest("POST", queryURL+"Hans1/", body)
	if err != nil {
		panic(err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	client = &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	respBody, _ = ioutil.ReadAll(resp.Body)
	respBodyStr = strings.Trim(string(respBody), " \n")

	if resp.Status != "400 Bad Request" || respBodyStr != "Could not find 'uploadfile' form field" {
		t.Error("Unexpected response:", resp.Status, respBodyStr)
		return
	}

	// Try with a absolute redirect

	writer = multipart.NewWriter(body)

	part, err = writer.CreateFormFile("uploadfile", "foo-upload.txt")
	errorutil.AssertOk(err)
	part.Write([]byte("footest"))

	writer.WriteField("redirect", "http://bla")

	writer.Close()

	req, err = http.NewRequest("POST", queryURL+"Hans1/", body)
	if err != nil {
		panic(err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	client = &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	respBody, _ = ioutil.ReadAll(resp.Body)
	respBodyStr = strings.Trim(string(respBody), " \n")

	if resp.Status != "400 Bad Request" || respBodyStr != "Could not redirect: Redirection URL must not be an absolute URL" {
		t.Error("Unexpected response:", resp.Status, respBodyStr)
		return
	}

	// Remap branch as read-only

	tree.Reset(false)

	err = tree.AddMapping("/", "footest", false)
	errorutil.AssertOk(err)

	writer = multipart.NewWriter(body)

	part, err = writer.CreateFormFile("uploadfile", "foo-upload.txt")
	errorutil.AssertOk(err)
	part.Write([]byte("footest"))

	writer.Close()

	req, err = http.NewRequest("POST", queryURL+"Hans1/", body)
	if err != nil {
		panic(err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	client = &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	respBody, _ = ioutil.ReadAll(resp.Body)
	respBodyStr = strings.Trim(string(respBody), " \n")

	if resp.Status != "400 Bad Request" || respBodyStr != "Could not write file foo-upload.txt: All applicable branches for the requested path were mounted as not writable" {
		t.Error("Unexpected response:", resp.Status, respBodyStr)
		return
	}
}

func TestFileQuery(t *testing.T) {
	queryURL := "http://localhost" + TESTPORT + EndpointFile
	dirqueryURL := "http://localhost" + TESTPORT + EndpointDir
	progressqueryURL := "http://localhost" + TESTPORT + EndpointProgress

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

	barRPC := fmt.Sprintf("%v:%v", branchConfigs["bartest"][config.RPCHost], branchConfigs["bartest"][config.RPCPort])
	barFP := bartest.SSLFingerprint()

	err = tree.AddBranch("bartest", barRPC, barFP)
	errorutil.AssertOk(err)

	err = tree.AddMapping("/tmp", "bartest", true)
	errorutil.AssertOk(err)

	// Create a file for renaming and deletion

	ioutil.WriteFile("bar/newfile1", []byte("test123"), 0660)

	st, _, res := sendTestRequest(dirqueryURL+"Hans1?recursive=TRUE&checksums=1", "GET", nil)
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
    },
    {
      "checksum": "",
      "isdir": true,
      "name": "tmp",
      "size": 0
    }
  ],
  "/sub1": [
    {
      "checksum": "f89782b1",
      "isdir": false,
      "name": "test3",
      "size": 17
    }
  ],
  "/tmp": [
    {
      "checksum": "abcc6601",
      "isdir": false,
      "name": "newfile1",
      "size": 7
    },
    {
      "checksum": "5b62da0f",
      "isdir": false,
      "name": "test1",
      "size": 10
    }
  ]
}`[1:] {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Read a file

	st, _, res = sendTestRequest(queryURL+"Hans1/test2", "GET", nil)
	if st != "200 OK" || res != `
Test2 file`[1:] {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Try to delete a file

	st, _, res = sendTestRequest(queryURL+"Hans1/test2", "DELETE", nil)
	if st != "400 Bad Request" || res != "All applicable branches for the requested path were mounted as not writable" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1/test2", "DELETE", []byte(`["1.txt", "2.txt"]`))
	if st != "400 Bad Request" || res != "All applicable branches for the requested path were mounted as not writable" {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Rename the file

	st, _, res = sendTestRequest(queryURL+"Hans1/tmp/newfile1", "PUT", []byte(`
{
    "action" : "rename",
	"newname" : "newtest1"
}`))
	if st != "200 OK" || res != "{}" {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Create a directory

	st, _, res = sendTestRequest(queryURL+"Hans1/tmp/upload/", "PUT", []byte(`
{
    "action" : "mkdir"
}`))
	if st != "200 OK" || res != "{}" {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Copy a file

	st, _, res = sendTestRequest(queryURL+"Hans1/tmp/newtest1", "PUT", []byte(`
{
    "action" : "copy",
	"destination" : "/tmp/upload/"
}`))
	if st != "200 OK" {
		t.Error("Unexpected response:", st, res)
		return
	}

	var resMap map[string]interface{}
	json.Unmarshal([]byte(res), &resMap)

	pid := fmt.Sprint(resMap["progress_id"])

	for {

		st, _, res = sendTestRequest(progressqueryURL+"Hans1/"+pid, "GET", nil)
		json.Unmarshal([]byte(res), &resMap)

		if st != "200 OK" || resMap["progress"] == resMap["total_progress"] {
			break
		}
	}

	if st != "200 OK" || res != `
{
  "errors": [],
  "item": 1,
  "operation": "Copy",
  "progress": 7,
  "subject": "/newtest1",
  "total_items": 1,
  "total_progress": 7
}`[1:] {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1/tmp/upload/newtest1", "PUT", []byte(`
{
    "action" : "rename",
	"newname" : "bla.txt"
}`))
	if st != "200 OK" {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Check the directory

	st, _, res = sendTestRequest(dirqueryURL+"Hans1/tmp?recursive=1&checksums=1", "GET", nil)
	if st != "200 OK" || res != `
{
  "/tmp": [
    {
      "checksum": "abcc6601",
      "isdir": false,
      "name": "newtest1",
      "size": 7
    },
    {
      "checksum": "5b62da0f",
      "isdir": false,
      "name": "test1",
      "size": 10
    },
    {
      "checksum": "",
      "isdir": true,
      "name": "upload",
      "size": 4096
    }
  ],
  "/tmp/upload": [
    {
      "checksum": "abcc6601",
      "isdir": false,
      "name": "bla.txt",
      "size": 7
    }
  ]
}`[1:] {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Copy multiple files

	st, _, res = sendTestRequest(queryURL+"Hans1", "PUT", []byte(`
{
    "action" : "copy",
	"files" : ["/tmp/newtest1", "/tmp/test1"],
	"destination" : "/tmp/upload"
}`))
	if st != "200 OK" {
		t.Error("Unexpected response:", st, res)
		return
	}

	json.Unmarshal([]byte(res), &resMap)

	pid = fmt.Sprint(resMap["progress_id"])

	for {

		st, _, res = sendTestRequest(progressqueryURL+"Hans1/"+pid, "GET", nil)
		json.Unmarshal([]byte(res), &resMap)

		if st != "200 OK" || (resMap["progress"] == resMap["total_progress"] &&
			resMap["item"] == resMap["total_items"]) {
			break
		}
	}

	if st != "200 OK" || res != `
{
  "errors": [],
  "item": 2,
  "operation": "Copy",
  "progress": 10,
  "subject": "/test1",
  "total_items": 2,
  "total_progress": 10
}`[1:] {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Check the directory

	st, _, res = sendTestRequest(dirqueryURL+"Hans1/tmp?recursive=1&checksums=1", "GET", nil)
	if st != "200 OK" || res != `
{
  "/tmp": [
    {
      "checksum": "abcc6601",
      "isdir": false,
      "name": "newtest1",
      "size": 7
    },
    {
      "checksum": "5b62da0f",
      "isdir": false,
      "name": "test1",
      "size": 10
    },
    {
      "checksum": "",
      "isdir": true,
      "name": "upload",
      "size": 4096
    }
  ],
  "/tmp/upload": [
    {
      "checksum": "abcc6601",
      "isdir": false,
      "name": "bla.txt",
      "size": 7
    },
    {
      "checksum": "abcc6601",
      "isdir": false,
      "name": "newtest1",
      "size": 7
    },
    {
      "checksum": "5b62da0f",
      "isdir": false,
      "name": "test1",
      "size": 10
    }
  ]
}`[1:] {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Rename multiple files

	st, _, res = sendTestRequest(queryURL+"Hans1", "PUT", []byte(`
{
    "action" : "rename",
	"files" : ["/tmp/newtest1", "/tmp/upload", "/tmp/upload1/bla.txt"],
	"newnames" : ["/tmp/newtest2", "/tmp/upload1", "/tmp/upload1/bla1.txt"]
}`))
	if st != "200 OK" {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Check the directory

	st, _, res = sendTestRequest(dirqueryURL+"Hans1/tmp?recursive=1&checksums=1", "GET", nil)
	if st != "200 OK" || res != `
{
  "/tmp": [
    {
      "checksum": "abcc6601",
      "isdir": false,
      "name": "newtest2",
      "size": 7
    },
    {
      "checksum": "5b62da0f",
      "isdir": false,
      "name": "test1",
      "size": 10
    },
    {
      "checksum": "",
      "isdir": true,
      "name": "upload1",
      "size": 4096
    }
  ],
  "/tmp/upload1": [
    {
      "checksum": "abcc6601",
      "isdir": false,
      "name": "bla1.txt",
      "size": 7
    },
    {
      "checksum": "abcc6601",
      "isdir": false,
      "name": "newtest1",
      "size": 7
    },
    {
      "checksum": "5b62da0f",
      "isdir": false,
      "name": "test1",
      "size": 10
    }
  ]
}`[1:] {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Test error conditions

	st, _, res = sendTestRequest(queryURL+"Hans1", "PUT", []byte(`
{
    "action" : "rename",
	"files" : ["/tmp/newtest1", "/tmp/upload", "/tmp/upload1/bla.txt"],
	"newnames" : "/tmp/newtest2"
}`))
	if st != "400 Bad Request" || res != "Parameter newnames must be a list of filenames" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1", "PUT", []byte(`
{
    "action" : "rename",
	"newnames" : ["/tmp/newtest2"]
}`))
	if st != "400 Bad Request" || res != "Parameter files is missing from request body" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1", "PUT", []byte(`
{
    "action" : "rename",
	"files" : "/tmp/newtest1",
	"newnames" : ["/tmp/newtest2"]
}`))
	if st != "400 Bad Request" || res != "Parameter files must be a list of files" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1", "PUT", []byte(`
{
    "action" : "copy",
	"files" : "/tmp/newtest1",
	"destination" : "/tmp/upload"
}`))
	if st != "400 Bad Request" || res != "Parameter files must be a list of files" {
		t.Error("Unexpected response:", st, res)
		return
	}

	ProgressMap = datautil.NewMapCache(100, 0)

	st, _, res = sendTestRequest(queryURL+"Hans1/tmp/newtest3", "PUT", []byte(`
{
    "action" : "copy",
	"destination" : "/tmp/upload/bla.txt"
}`))
	if st != "400 Bad Request" || res != "Cannot stat /tmp/newtest3: RufsError: Remote error (file does not exist)" {
		t.Error("Unexpected response:", st, res)
		return
	}

	// Check error in ProgressMap

	for k := range ProgressMap.GetAll() {
		k = strings.Split(k, "#")[1]
		_, _, res = sendTestRequest(progressqueryURL+"Hans1/"+k, "GET", nil)
		resMap = nil
		errorutil.AssertOk(json.Unmarshal([]byte(res), &resMap))

		if res := fmt.Sprint(resMap["errors"]); res != "[Cannot stat /tmp/newtest3: RufsError: Remote error (file does not exist)]" {
			t.Error("Unexpected result:", res)
		}

		break
	}

	st, _, res = sendTestRequest(queryURL+"Hans1/", "GET", nil)
	if st != "400 Bad Request" || res != "Need a tree name and a file path" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans2/bla", "GET", nil)
	if st != "400 Bad Request" || res != "Unknown tree: Hans2" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1/bla", "GET", nil)
	if st != "400 Bad Request" || res != "Could not read file bla: RufsError: Remote error (stat /bla: no such file or directory)" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(progressqueryURL+"Hans1/", "GET", nil)
	if st != "400 Bad Request" || res != "Need a tree name and a progress ID" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(progressqueryURL+"Hans2/bla", "GET", nil)
	if st != "400 Bad Request" || res != "Unknown tree: Hans2" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(progressqueryURL+"Hans1/bla", "GET", nil)
	if st != "400 Bad Request" || res != "Unknown progress ID: bla" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL, "PUT", []byte(""))
	if st != "400 Bad Request" || res != "Need a tree name and a file path" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans2/bla", "PUT", []byte(""))
	if st != "400 Bad Request" || res != "Unknown tree: Hans2" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1/bla", "PUT", []byte(`
aaa{
    "action" : "copy",
	"destination" : "/tmp/upload/bla.txt"
}`))
	if st != "400 Bad Request" || res != "Could not decode request body: invalid character 'a' looking for beginning of value" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1/bla", "PUT", []byte(`
{
    "action2" : "copy",
	"destination" : "/tmp/upload/bla.txt"
}`))
	if st != "400 Bad Request" || res != "Action command is missing from request body" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1/bla", "PUT", []byte(`
{
    "action" : "copy",
	"destination2" : "/tmp/upload/bla.txt"
}`))
	if st != "400 Bad Request" || res != "Parameter destination is missing from request body" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1/bla", "PUT", []byte(`
{
    "action" : "copy2",
	"destination" : "/tmp/upload/bla.txt"
}`))
	if st != "400 Bad Request" || res != "Unknown action: copy2" {
		t.Error("Unexpected response:", st, res)
		return
	}

	st, _, res = sendTestRequest(queryURL+"Hans1/bla", "PUT", []byte(`
{
    "action" : "rename"
}`))
	if st != "400 Bad Request" || res != "Parameter newname is missing from request body" {
		t.Error("Unexpected response:", st, res)
		return
	}
}
