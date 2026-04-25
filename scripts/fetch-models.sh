#!/usr/bin/env bash

set -e

echo "Waiting for Ollama..."
until ollama list > /dev/null 2>&1; do sleep 1; done

ollama pull llama3.1:8b

echo "Models ready."
