/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

package node

import (
	"errors"
	"fmt"
	"log"
)

// Logging
// =======

/*
Logger is a function which processes log messages
*/
type Logger func(v ...interface{})

/*
LogInfo is called if an info message is logged
*/
var LogInfo = Logger(log.Print)

/*
LogDebug is called if a debug message is logged
(by default disabled)
*/
var LogDebug = Logger(LogNull)

/*
LogNull is a discarding logger to be used for disabling loggers
*/
var LogNull = func(v ...interface{}) {
}

// Errors
// ======

/*
Error is a network related error
*/
type Error struct {
	Type       error  // Error type (to be used for equal checks)
	Detail     string // Details of this error
	IsNotExist bool   // Error is file or directory does not exist
}

/*
Error returns a human-readable string representation of this error.
*/
func (ge *Error) Error() string {
	if ge.Detail != "" {
		return fmt.Sprintf("RufsError: %v (%v)", ge.Type, ge.Detail)
	}

	return fmt.Sprintf("RufsError: %v", ge.Type)
}

/*
Network related error types
*/
var (
	ErrNodeComm        = errors.New("Network error")
	ErrRemoteAction    = errors.New("Remote error")
	ErrUnknownTarget   = errors.New("Unknown target node")
	ErrUntrustedTarget = errors.New("Unexpected SSL certificate from target node")
	ErrInvalidToken    = errors.New("Invalid node token")
)
