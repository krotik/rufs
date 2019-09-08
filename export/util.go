/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

package export

import "log"

// Logging
// =======

/*
Logger is a function which processes log messages
*/
type Logger func(v ...interface{})

/*
LogError is called if an error message is logged
*/
var LogError = Logger(log.Print)

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
