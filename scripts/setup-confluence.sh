#!/bin/bash
# Setup script for Confluence CLI integration

set -e

echo "Setting up Confluence CLI for forecast documentation..."
echo ""

# Check if confluence-cli is installed
if ! command -v confluence &> /dev/null; then
    echo "Installing confluence CLI from pchuri/confluence-cli..."
    go install github.com/pchuri/confluence-cli@latest
    echo "✓ confluence CLI installed"
else
    echo "✓ confluence CLI already installed"
fi

# Check for environment variables
echo ""
echo "Checking Confluence configuration..."

if [ -z "$CONFLUENCE_URL" ]; then
    echo "❌ CONFLUENCE_URL not set"
    echo "Please add to your ~/.zshrc or ~/.bashrc:"
    echo "  export CONFLUENCE_URL='https://yourcompany.atlassian.net'"
    EXIT_CODE=1
else
    echo "✓ CONFLUENCE_URL: $CONFLUENCE_URL"
fi

if [ -z "$CONFLUENCE_USER" ]; then
    echo "❌ CONFLUENCE_USER not set"
    echo "Please add to your ~/.zshrc or ~/.bashrc:"
    echo "  export CONFLUENCE_USER='your.email@company.com'"
    EXIT_CODE=1
else
    echo "✓ CONFLUENCE_USER: $CONFLUENCE_USER"
fi

if [ -z "$CONFLUENCE_TOKEN" ]; then
    echo "❌ CONFLUENCE_TOKEN not set"
    echo "Please add to your ~/.zshrc or ~/.bashrc:"
    echo "  export CONFLUENCE_TOKEN='your-api-token'"
    echo ""
    echo "Create API token at: https://id.atlassian.com/manage-profile/security/api-tokens"
    EXIT_CODE=1
else
    echo "✓ CONFLUENCE_TOKEN is set"
fi

if [ -n "$EXIT_CODE" ]; then
    echo ""
    echo "Setup incomplete. Please set the required environment variables and run again."
    exit 1
fi

echo ""
echo "✓ All configuration is set!"
echo ""
echo "Next steps:"
echo "1. Create/choose a Confluence space for documentation"
echo "2. Run ./scripts/publish-docs.sh <SPACE-KEY>"
echo ""
echo "Example: ./scripts/publish-docs.sh FORECAST"
