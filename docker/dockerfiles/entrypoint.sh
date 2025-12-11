#!/bin/sh
set -e

# Minimal entrypoint: drop to appuser and run the app
exec su-exec appuser "$@"
