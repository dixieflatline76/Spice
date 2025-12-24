#!/bin/bash
# Usage: source ./load_secrets.sh

# Check if the script is being sourced or executed
(return 0 2>/dev/null) && sourced=1 || sourced=0
if [ $sourced -eq 0 ]; then
  echo "Error: This script must be sourced to work correctly."
  echo "Usage: source ./load_secrets.sh"
  exit 1
fi

echo "Loading secrets from .spice_secrets..."
if [ -f .spice_secrets ]; then
    while IFS='=' read -r key value; do
      # Skip comments and empty lines
      if [[ $key =~ ^#.* ]] || [[ -z $key ]]; then
        continue
      fi
      
      # Remove surrounding quotes from value if present
      value="${value%\"}"
      value="${value#\"}"
      
      # Remove invisible trailing carriage return (DOS line endings)
      value="${value//$'\r'/}"
      
      export "$key=$value"
      
      # Mask value for display
      masked_value="${value:0:4}****${value: -4}"
      echo "Exported $key=$masked_value"
    done < .spice_secrets
    echo "Secrets loaded."
else
    echo "Warning: .spice_secrets not found in current directory."
fi
