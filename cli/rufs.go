/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

/*
Rufs main entry point for the standalone server.
*/
package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"time"

	"devt.de/krotik/common/cryptutil"
	"devt.de/krotik/common/fileutil"
	"devt.de/krotik/rufs/config"
)

/*
DefaultSecretFile is the default secret file which is used in server and client mode
*/
const DefaultSecretFile = "rufs.secret"

/*
DefaultSSLDir is the default directory containing the ssl key.pem and cert.pem files
*/
const DefaultSSLDir = "ssl"

/*
Main entry point for Rufs.
*/
func main() {
	var err error

	fmt.Println(fmt.Sprintf("Rufs %v", config.ProductVersion))

	flag.Usage = func() {

		// Print usage for tool selection

		fmt.Println()
		fmt.Println(fmt.Sprintf("Usage of %s [tool]", os.Args[0]))
		fmt.Println()
		fmt.Println("The tools are:")
		fmt.Println()
		fmt.Println("    server    Run as a server")
		fmt.Println("    client    Run as a client")
		fmt.Println()
		fmt.Println(fmt.Sprintf("Use %s [tool] --help for more information about a tool.", os.Args[0]))
		fmt.Println()
	}

	flag.Parse()

	if len(flag.Args()) == 0 {
		flag.Usage()
		return
	}

	if flag.Args()[0] == "server" {

		err = serverCli()

	} else if flag.Args()[0] == "client" {

		err = clientCli()

	} else {
		err = fmt.Errorf("Invalid tool")
	}

	if err != nil {
		fmt.Println(fmt.Sprintf("Error: %v", err))
	}
}

// Common code
// ===========

/*
commonCliOptions returns common command line options which are relevant
for both server and client.
*/
func commonCliOptions() (*string, *string) {
	secretFile := flag.String("secret", DefaultSecretFile, "Secret file containing the secret token")
	certDir := flag.String("ssl-dir", DefaultSSLDir, "Directory containing the ssl key.pem and cert.pem files")

	return secretFile, certDir
}

/*
loadSecretAndCert loads the secret string and the SSL key and certificate.
*/
func loadSecretAndCert(secretFile, certDir string) ([]byte, *tls.Certificate, error) {
	var ok bool
	var err error

	// Load secret

	if ok, _ = fileutil.PathExists(secretFile); !ok {
		uuid := cryptutil.GenerateUUID()
		err = ioutil.WriteFile(secretFile, uuid[:], 0600)
	}

	if err == nil {
		var secret []byte

		if secret, err = ioutil.ReadFile(secretFile); err == nil {

			fmt.Println(fmt.Sprintf("Using secret from: %s", secretFile))

			// Load ssl key and certificate

			if ok, _ = fileutil.PathExists(certDir); !ok {

				if err = os.MkdirAll(certDir, 0700); err == nil {

					err = cryptutil.GenCert(certDir, "cert.pem", "key.pem", "localhost",
						"", 365*24*time.Hour, false, 4096, "")
				}
			}

			if err == nil {
				var cert tls.Certificate

				cert, err = tls.LoadX509KeyPair(filepath.Join(certDir, "cert.pem"),
					filepath.Join(certDir, "key.pem"))

				if err == nil {
					fmt.Println(fmt.Sprintf("Using ssl key.pem and cert.pem from: %s", certDir))

					return secret, &cert, nil
				}
			}
		}
	}

	return nil, nil, err
}

/*
externalIP returns the first found external IP
*/
func externalIP() (string, error) {
	var ipstr string

	ifaces, err := net.Interfaces()

	if err == nil {

	Loop:
		for _, iface := range ifaces {
			var addrs []net.Addr

			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {

				// Ignore interfaces which are down or loopback devices

				continue
			}

			if addrs, err = iface.Addrs(); err == nil {

				// Go through all found addresses

				for _, addr := range addrs {
					var ip net.IP

					switch v := addr.(type) {
					case *net.IPNet:
						ip = v.IP
					case *net.IPAddr:
						ip = v.IP
					default:
						continue
					}

					if !ip.IsLoopback() {
						if ip = ip.To4(); ip != nil {
							ipstr = ip.String()
							break Loop
						}
					}
				}
			}
		}
	}

	if ipstr == "" {
		err = errors.New("No external interface found")
	}

	return ipstr, err
}
