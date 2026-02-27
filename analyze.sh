#!/bin/bash
# Wrapper script for backward compatibility
# Forwards all arguments to scripts/analyze.sh
exec "$(dirname "$0")/scripts/analyze.sh" "$@"
