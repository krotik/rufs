/*
 * Rufs - Remote Union File System
 *
 * Copyright 2017 Matthias Ladkau. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the MIT
 * License, If a copy of the MIT License was not distributed with this
 * file, You can obtain one at https://opensource.org/licenses/MIT.
 */

package rufs

import (
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestFileInfo(t *testing.T) {

	oldUnitTestModes := unitTestModes
	unitTestModes = false
	defer func() {
		unitTestModes = oldUnitTestModes
	}()

	fi := &FileInfo{"test", 500, os.FileMode(0764), time.Time{}, "123", false, ""}

	if fi.String() != "test 123 [500] -rwxrw-r-- (0001-01-01 00:00:00 +0000 UTC) - <nil>" {
		t.Error("Unexpected result:", fi)
		return
	}

	fi = &FileInfo{"test", 500, os.FileMode(0764), time.Time{}, "", false, ""}

	if fi.String() != "test [500] -rwxrw-r-- (0001-01-01 00:00:00 +0000 UTC) - <nil>" {
		t.Error("Unexpected result:", fi)
		return
	}

	ioutil.WriteFile("foo.txt", []byte("bar"), 0660)
	defer os.Remove("foo.txt")

	fi = &FileInfo{"foo.txt", 500, os.ModeSymlink, time.Time{}, "", false, ""}
	fi = WrapFileInfo("./", fi).(*FileInfo)

	if fi.String() != "foo.txt [3] -rw-rw---- (0001-01-01 00:00:00 +0000 UTC) - <nil>" &&
		fi.String() != "foo.txt [3] -rw-r----- (0001-01-01 00:00:00 +0000 UTC) - <nil>" &&
		fi.String() != "foo.txt [3] -rw-rw-rw- (0001-01-01 00:00:00 +0000 UTC) - <nil>" {
		t.Error("Unexpected result:", fi)
		return
	}
}
