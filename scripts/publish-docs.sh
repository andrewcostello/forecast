#!/bin/bash
# Publish forecast documentation to Confluence

set -e

# Check arguments
if [ -z "$1" ]; then
    echo "Usage: ./scripts/publish-docs.sh <SPACE-KEY> [PARENT-PAGE-ID]"
    echo ""
    echo "Example: ./scripts/publish-docs.sh FORECAST"
    echo "Example: ./scripts/publish-docs.sh DEV 123456"
    exit 1
fi

SPACE_KEY="$1"
PARENT_PAGE_ID="$2"

# Check environment variables
if [ -z "$CONFLUENCE_URL" ] || [ -z "$CONFLUENCE_USER" ] || [ -z "$CONFLUENCE_TOKEN" ]; then
    echo "❌ Confluence environment variables not set"
    echo "Run ./scripts/setup-confluence.sh first"
    exit 1
fi

# Remove /wiki suffix if present
CONFLUENCE_BASE="${CONFLUENCE_URL%/wiki}"

echo "Publishing forecast documentation to Confluence..."
echo "Space: $SPACE_KEY"
echo "URL: $CONFLUENCE_URL"
echo ""

# Function to convert markdown to Confluence storage format (basic)
convert_md_to_confluence() {
    local file="$1"

    # Read file and do basic conversions
    # This is a simple converter - Confluence has its own storage format
    cat "$file" | \
        sed 's/^# \(.*\)/<h1>\1<\/h1>/' | \
        sed 's/^## \(.*\)/<h2>\1<\/h2>/' | \
        sed 's/^### \(.*\)/<h3>\1<\/h3>/' | \
        sed 's/^#### \(.*\)/<h4>\1<\/h4>/' | \
        sed 's/^\*\*\(.*\)\*\*/<strong>\1<\/strong>/g' | \
        sed 's/^- \(.*\)/<li>\1<\/li>/' | \
        sed 's/```\(.*\)/<ac:structured-macro ac:name="code"><ac:parameter ac:name="language">\1<\/ac:parameter><ac:plain-text-body><!\[CDATA\[/g' | \
        sed 's/```/\]\]><\/ac:plain-text-body><\/ac:structured-macro>/g'
}

# Function to create or update a page
publish_page() {
    local title="$1"
    local file="$2"
    local parent_id="$3"

    echo "Publishing: $title"

    # Read and convert markdown content
    CONTENT=$(cat "$file")

    # Create JSON payload
    if [ -n "$parent_id" ]; then
        PARENT_JSON="\"ancestors\": [{\"id\": \"$parent_id\"}],"
    else
        PARENT_JSON=""
    fi

    # Check if page exists
    PAGE_RESPONSE=$(curl -s -u "$CONFLUENCE_USER:$CONFLUENCE_TOKEN" \
        "$CONFLUENCE_BASE/wiki/rest/api/content?title=$title&spaceKey=$SPACE_KEY&expand=version")

    PAGE_ID=$(echo "$PAGE_RESPONSE" | grep -o '"id":"[0-9]*"' | head -1 | cut -d'"' -f4)

    if [ -n "$PAGE_ID" ]; then
        # Update existing page
        VERSION=$(echo "$PAGE_RESPONSE" | grep -o '"number":[0-9]*' | head -1 | cut -d':' -f2)
        NEW_VERSION=$((VERSION + 1))

        echo "  Updating existing page (ID: $PAGE_ID, version $VERSION -> $NEW_VERSION)"

        curl -s -u "$CONFLUENCE_USER:$CONFLUENCE_TOKEN" \
            -X PUT \
            -H "Content-Type: application/json" \
            -d "{
                \"id\": \"$PAGE_ID\",
                \"type\": \"page\",
                \"title\": \"$title\",
                \"space\": {\"key\": \"$SPACE_KEY\"},
                \"body\": {
                    \"storage\": {
                        \"value\": \"<ac:structured-macro ac:name='markdown'><ac:plain-text-body><![CDATA[$CONTENT]]></ac:plain-text-body></ac:structured-macro>\",
                        \"representation\": \"storage\"
                    }
                },
                \"version\": {\"number\": $NEW_VERSION}
            }" \
            "$CONFLUENCE_BASE/wiki/rest/api/content/$PAGE_ID" > /dev/null

        echo "  ✓ Updated: $CONFLUENCE_URL/wiki/spaces/$SPACE_KEY/pages/$PAGE_ID"
    else
        # Create new page
        echo "  Creating new page"

        RESPONSE=$(curl -s -u "$CONFLUENCE_USER:$CONFLUENCE_TOKEN" \
            -X POST \
            -H "Content-Type: application/json" \
            -d "{
                \"type\": \"page\",
                \"title\": \"$title\",
                \"space\": {\"key\": \"$SPACE_KEY\"},
                $PARENT_JSON
                \"body\": {
                    \"storage\": {
                        \"value\": \"<ac:structured-macro ac:name='markdown'><ac:plain-text-body><![CDATA[$CONTENT]]></ac:plain-text-body></ac:structured-macro>\",
                        \"representation\": \"storage\"
                    }
                }
            }" \
            "$CONFLUENCE_BASE/wiki/rest/api/content")

        NEW_PAGE_ID=$(echo "$RESPONSE" | grep -o '"id":"[0-9]*"' | head -1 | cut -d'"' -f4)

        if [ -n "$NEW_PAGE_ID" ]; then
            echo "  ✓ Created: $CONFLUENCE_URL/wiki/spaces/$SPACE_KEY/pages/$NEW_PAGE_ID"
            echo "$NEW_PAGE_ID"
        else
            echo "  ❌ Failed to create page"
            echo "$RESPONSE"
            return 1
        fi
    fi
}

# Change to docs directory
cd "$(dirname "$0")/../docs"

# Publish main overview page first
echo "Publishing main overview page..."
OVERVIEW_ID=$(publish_page "Forecast Documentation" "README.md" "$PARENT_PAGE_ID")

# If parent page wasn't specified, use the overview as parent for other pages
if [ -z "$PARENT_PAGE_ID" ]; then
    PARENT_PAGE_ID="$OVERVIEW_ID"
fi

echo ""
echo "Publishing documentation pages..."

# Publish all other docs as child pages
publish_page "Methodology: Probabilistic Forecasting" "METHODOLOGY.md" "$PARENT_PAGE_ID"
publish_page "Developer Guide" "DEVELOPER_GUIDE.md" "$PARENT_PAGE_ID"
publish_page "AI Agent Guide" "AI_AGENT_GUIDE.md" "$PARENT_PAGE_ID"
publish_page "AI System Prompt" "AI_SYSTEM_PROMPT.md" "$PARENT_PAGE_ID"
publish_page "Quick Start Guide" "QUICK_START.md" "$PARENT_PAGE_ID"

echo ""
echo "✓ Documentation published successfully!"
echo ""
echo "View at: $CONFLUENCE_URL/wiki/spaces/$SPACE_KEY"
