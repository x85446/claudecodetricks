#!/bin/sh
# Forward conductor operations to the installed binary without owning policy.
set -eu
exec iterate-run conductor "$@"
