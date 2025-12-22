#!/bin/bash
# Usage: source ./load_secrets.sh

echo "Loading secrets from .spice_secrets..."
if [ -f .spice_secrets ]; then
    export $(grep -v '^#' .spice_secrets | xargs)
    echo "Secrets loaded."
else
    echo "Warning: .spice_secrets not found in current directory."
fi
