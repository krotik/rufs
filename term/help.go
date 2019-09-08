/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

package term

import (
	"bytes"
	"fmt"
	"sort"
	"unicode/utf8"

	"devt.de/krotik/common/stringutil"
)

/*
cmdHelp executes the help command.
*/
func cmdHelp(tt *TreeTerm, arg ...string) (string, error) {
	var res bytes.Buffer

	if len(arg) == 0 {
		var maxlen = 0

		cmds := make([]string, 0, len(helpMap))

		res.WriteString("Available commands:\n")
		res.WriteString("----\n")

		for c := range helpMap {

			if cc := utf8.RuneCountInString(c); cc > maxlen {
				maxlen = cc
			}

			cmds = append(cmds, c)
		}

		sort.Strings(cmds)

		for _, c := range cmds {
			cc := utf8.RuneCountInString(c)
			spacer := stringutil.GenerateRollingString(" ", maxlen-cc)

			res.WriteString(fmt.Sprintf("%v%v : %v\n", c, spacer, helpMap[c]))

		}
	}

	return res.String(), nil
}
