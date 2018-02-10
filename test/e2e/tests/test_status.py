#!/usr/bin/env python3

# Tests for `wfcli status`
# Note that this is a small experiment to see if python is more appropriate for the tests
import sys, os
print(sys.version_info)

sys.path.append(os.path.dirname(__file__)) # Needed to import testutils when invoked from another wd.
import testutils

# Check for correctly deployed cluster
testutils.run(["wfcli", "status"], check=True)

# Check if hitting a non-existing cluster results in an error
out = testutils.run(["wfcli", "--url", "http://127.0.0.1:1337", "status"])
if out.returncode == 0:
    raise "%s should have failed!" % " ".join(out.args)

print("bla: %s" % out.args)