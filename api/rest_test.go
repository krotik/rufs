/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

package api

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"devt.de/krotik/common/httputil"
	"devt.de/krotik/rufs"
)

const testPort = ":9040"

var testQueryURL = "http://localhost" + testPort

var lastRes []string

type testEndpoint struct {
	*DefaultEndpointHandler
}

/*
handleSearchQuery handles a search query REST call.
*/
func (te *testEndpoint) HandleGET(w http.ResponseWriter, r *http.Request, resources []string) {
	lastRes = resources
	te.DefaultEndpointHandler.HandleGET(w, r, resources)
}

func (te *testEndpoint) SwaggerDefs(s map[string]interface{}) {
}

var testEndpointMap = map[string]RestEndpointInst{
	"/": func() RestEndpointHandler {
		return &testEndpoint{}
	},
}

func TestMain(m *testing.M) {
	flag.Parse()

	hs, wg := startServer()
	if hs == nil {
		return
	}
	defer stopServer(hs, wg)

	RegisterRestEndpoints(testEndpointMap)
	RegisterRestEndpoints(GeneralEndpointMap)

	// Run the tests

	res := m.Run()

	// Teardown

	stopServer(hs, wg)

	os.Exit(res)
}

func TestTreeManagement(t *testing.T) {

	if err := AddTree("1", &rufs.Tree{}); err != nil {
		t.Error(err)
		return
	}

	if err := AddTree("1", &rufs.Tree{}); err == nil || err.Error() != "Tree 1 already exists" {
		t.Error(err)
		return
	}

	if res, _ := Trees(); fmt.Sprint(res) != "map[1:/: ]" {
		t.Error("Unexpected result: ", res)
		return
	}

	if res, _, _ := GetTree("1"); res == nil {
		t.Error("Unexpected result: ", res)
		return
	}

	if err := RemoveTree("1"); err != nil {
		t.Error(err)
		return
	}

	if err := RemoveTree("1"); err == nil || err.Error() != "Tree 1 does not exist" {
		t.Error(err)
		return
	}

	if err := AddTree("1", &rufs.Tree{}); err != nil {
		t.Error(err)
		return
	}

	ResetTrees()

	if res, _ := Trees(); fmt.Sprint(res) != "map[]" {
		t.Error("Unexpected result: ", res)
		return
	}
}

func TestEndpointHandling(t *testing.T) {

	lastRes = nil

	if _, _, res, _ := sendTestRequest(testQueryURL, "GET", nil); res != "Method Not Allowed" {
		t.Error("Unexpected response:", res)
		return
	}

	if lastRes != nil {
		t.Error("Unexpected lastRes:", lastRes)
	}

	lastRes = nil

	if _, _, res, _ := sendTestRequest(testQueryURL+"/foo/bar", "GET", nil); res != "Method Not Allowed" {
		t.Error("Unexpected response:", res)
		return
	}

	if fmt.Sprint(lastRes) != "[foo bar]" {
		t.Error("Unexpected lastRes:", lastRes)
	}

	lastRes = nil

	if _, _, res, _ := sendTestRequest(testQueryURL+"/foo/bar/", "GET", nil); res != "Method Not Allowed" {
		t.Error("Unexpected response:", res)
		return
	}

	if fmt.Sprint(lastRes) != "[foo bar]" {
		t.Error("Unexpected lastRes:", lastRes)
	}

	if _, _, res, _ := sendTestRequest(testQueryURL, "POST", nil); res != "Method Not Allowed" {
		t.Error("Unexpected response:", res)
		return
	}

	if _, _, res, _ := sendTestRequest(testQueryURL, "PUT", nil); res != "Method Not Allowed" {
		t.Error("Unexpected response:", res)
		return
	}

	if _, _, res, _ := sendTestRequest(testQueryURL, "DELETE", nil); res != "Method Not Allowed" {
		t.Error("Unexpected response:", res)
		return
	}

	if _, _, res, _ := sendTestRequest(testQueryURL, "UPDATE", nil); res != "Method Not Allowed" {
		t.Error("Unexpected response:", res)
		return
	}

	// Test swagger endpoint

	if _, _, res, _ := sendTestRequest(testQueryURL+"/fs/swagger.json", "GET", nil); res != `
{
  "basePath": "/fs",
  "definitions": {
    "Error": {
      "description": "A human readable error mesage.",
      "type": "string"
    }
  },
  "host": "localhost:9040",
  "info": {
    "description": "Query and control the Remote Union File System.",
    "title": "RUFS API",
    "version": "1.0.0"
  },
  "paths": {
    "/about": {
      "get": {
        "description": "Returns available API versions, product name and product version.",
        "produces": [
          "application/json"
        ],
        "responses": {
          "200": {
            "description": "About info object",
            "schema": {
              "properties": {
                "api_versions": {
                  "description": "List of available API versions.",
                  "items": {
                    "description": "Available API version.",
                    "type": "string"
                  },
                  "type": "array"
                },
                "product": {
                  "description": "Product name of the REST API provider.",
                  "type": "string"
                },
                "version": {
                  "description": "Version of the REST API provider.",
                  "type": "string"
                }
              },
              "type": "object"
            }
          },
          "default": {
            "description": "Error response",
            "schema": {
              "$ref": "#/definitions/Error"
            }
          }
        },
        "summary": "Return information about the REST API provider."
      }
    }
  },
  "produces": [
    "application/json"
  ],
  "schemes": [
    "https"
  ],
  "swagger": "2.0"
}`[1:] {
		t.Error("Unexpected response:", res)
		return
	}
}

/*
Send a request to a HTTP test server
*/
func sendTestRequest(url string, method string, content []byte) (string, http.Header, string, *http.Response) {
	var req *http.Request
	var err error

	if content != nil {
		req, err = http.NewRequest(method, url, bytes.NewBuffer(content))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		panic(err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	bodyStr := strings.Trim(string(body), " \n")

	// Try json decoding first

	out := bytes.Buffer{}
	err = json.Indent(&out, []byte(bodyStr), "", "  ")
	if err == nil {
		return resp.Status, resp.Header, out.String(), resp
	}

	// Just return the body

	return resp.Status, resp.Header, bodyStr, resp
}

/*
Start a HTTP test server.
*/
func startServer() (*httputil.HTTPServer, *sync.WaitGroup) {
	hs := &httputil.HTTPServer{}

	var wg sync.WaitGroup
	wg.Add(1)

	go hs.RunHTTPServer(testPort, &wg)

	wg.Wait()

	// Server is started

	if hs.LastError != nil {
		panic(hs.LastError)
	}

	return hs, &wg
}

/*
Stop a started HTTP test server.
*/
func stopServer(hs *httputil.HTTPServer, wg *sync.WaitGroup) {

	if hs.Running == true {

		wg.Add(1)

		// Server is shut down

		hs.Shutdown()

		wg.Wait()

	} else {

		panic("Server was not running as expected")
	}
}
