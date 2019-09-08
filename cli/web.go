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
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"devt.de/krotik/common/cryptutil"
	"devt.de/krotik/common/errorutil"
	"devt.de/krotik/common/fileutil"
	"devt.de/krotik/common/httputil"
	"devt.de/krotik/rufs"
	"devt.de/krotik/rufs/api"
	"devt.de/krotik/rufs/api/v1"
)

/*
webdir is the web directory which will contain all html files
*/
const webdir = "web"

/*
setupWebExport exports Rufs through a web interface
*/
func setupWebExport(webExport *string, tree *rufs.Tree, certDir *string) error {
	var ok bool
	var err error

	// Register REST endpoints for version 1

	api.RegisterRestEndpoints(v1.V1EndpointMap)
	api.RegisterRestEndpoints(api.GeneralEndpointMap)

	// Set the default tree

	api.AddTree("default", tree)

	// Ensure web folder

	if ok, err = fileutil.PathExists(webdir); err == nil && !ok {
		fmt.Println("Creating web folder")

		err = extractWebFiles(webdir)
	}

	// Start the web server

	if ok, _ = fileutil.PathExists(webdir); err == nil && ok {

		fs := http.FileServer(http.Dir(webdir))

		api.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fs.ServeHTTP(w, r)
		})

		// Start HTTPS server and enable REST API

		hs := &httputil.HTTPServer{}

		var wg sync.WaitGroup
		wg.Add(1)

		weblocString := *webExport
		if strings.HasPrefix(weblocString, ":") {
			weblocString = fmt.Sprintf("<any interface>%s", weblocString)
		}

		fmt.Println(fmt.Sprintf("Starting server on: https://%s", weblocString))

		go hs.RunHTTPSServer(*certDir, "cert.pem", "key.pem",
			*webExport, &wg)

		// Wait until the server has started

		wg.Wait()

		if err = hs.LastError; err == nil {

			// Read server certificate and write a fingerprint file

			fpfile := filepath.Join(webdir, "fingerprint.json")

			fmt.Println("Writing fingerprint file: ", fpfile)

			certs, _ := cryptutil.ReadX509CertsFromFile(filepath.Join(*certDir, "cert.pem"))

			if len(certs) > 0 {
				buf := bytes.Buffer{}

				buf.WriteString("{\n")
				buf.WriteString(fmt.Sprintf(`  "md5"    : "%s",`, cryptutil.Md5CertFingerprint(certs[0])))
				buf.WriteString("\n")
				buf.WriteString(fmt.Sprintf(`  "sha1"   : "%s",`, cryptutil.Sha1CertFingerprint(certs[0])))
				buf.WriteString("\n")
				buf.WriteString(fmt.Sprintf(`  "sha256" : "%s"`, cryptutil.Sha256CertFingerprint(certs[0])))
				buf.WriteString("\n")
				buf.WriteString("}\n")

				ioutil.WriteFile(fpfile, buf.Bytes(), 0644)
			}

			// Add to the wait group so we can wait for the shutdown

			wg.Add(1)

			fmt.Println("Waiting for shutdown")
			wg.Wait()
		}
	}

	return err
}

/*
extractWebFiles extracts the web files from the executable.
*/
func extractWebFiles(webfolder string) error {
	end := "####"
	marker := fmt.Sprintf("%v%v%v", end, "WEBZIP", end)

	exename, err := filepath.Abs(os.Args[0])
	errorutil.AssertOk(err)

	if ok, _ := fileutil.PathExists(exename); !ok {

		// Try an optional .exe suffix which might work on Windows

		exename += ".exe"
	}

	stat, err := os.Stat(exename)
	if err != nil {
		return err
	}

	// Open the executable

	f, err := os.Open(exename)
	if err != nil {
		return err
	}
	defer f.Close()

	found := false
	buf := make([]byte, 4096)
	buf2 := make([]byte, len(marker)+10)

	var pos int64

	// Look for the marker which marks the beginning of the attached zip file

	for i, err := f.Read(buf); err == nil; i, err = f.Read(buf) {

		// Check if the marker could be in the read string

		if strings.Contains(string(buf), "#") {

			// Marker was found - read a bit more to ensure we got the full marker

			if i2, err := f.Read(buf2); err == nil || err == io.EOF {
				candidateString := string(append(buf, buf2...))

				// Now determine the position if the zip file

				if markerIndex := strings.Index(candidateString, marker); markerIndex >= 0 {
					start := int64(markerIndex + len(marker))
					for unicode.IsSpace(rune(candidateString[start])) || unicode.IsControl(rune(candidateString[start])) {
						start++ // Skip final control characters \n or \r\n
					}
					pos += start
					found = true
					break
				}

				pos += int64(i2)
			}
		}

		pos += int64(i)
	}

	if err == nil {
		if found {

			// Extract the zip

			if _, err = f.Seek(pos, 0); err == nil {
				zipLen := stat.Size() - pos

				if err = os.MkdirAll(webfolder, 0755); err == nil {
					err = fileutil.UnzipReader(io.NewSectionReader(f, pos, zipLen), zipLen, webfolder, false)
				}
			}

		} else {

			err = fmt.Errorf("Could not find web content marker in executable - invalid executable")
		}
	}

	return err
}
