# Confluence Publishing Scripts

Scripts to publish forecast documentation to Confluence.

## Setup

### 1. Get Confluence API Token

1. Go to https://id.atlassian.com/manage-profile/security/api-tokens
2. Click "Create API token"
3. Give it a name like "Forecast Docs"
4. Copy the token

### 2. Set Environment Variables

Add to your `~/.zshrc` or `~/.bashrc`:

```bash
export CONFLUENCE_URL="https://yourcompany.atlassian.net"
export CONFLUENCE_USER="your.email@company.com"
export CONFLUENCE_TOKEN="your-api-token-here"
```

Reload your shell:
```bash
source ~/.zshrc  # or source ~/.bashrc
```

### 3. Run Setup Script

```bash
cd /Users/andrewcostello/go/forecast
./scripts/setup-confluence.sh
```

This will verify your configuration is correct.

## Publishing Documentation

### Publish to a New Space

```bash
./scripts/publish-docs.sh FORECAST
```

This will:
1. Create a main "Forecast Documentation" page
2. Create child pages for each doc:
   - Methodology
   - Developer Guide
   - AI Agent Guide
   - AI System Prompt
   - Quick Start

### Publish to Existing Space

If you want to publish under an existing page:

```bash
./scripts/publish-docs.sh YOUR-SPACE 123456
```

Where `123456` is the parent page ID.

### Update Documentation

Just run the script again - it will update existing pages:

```bash
./scripts/publish-docs.sh FORECAST
```

## Finding Your Space Key

1. Go to your Confluence instance
2. Navigate to the space you want to use
3. The space key is in the URL: `https://yourcompany.atlassian.net/wiki/spaces/SPACEKEY/`

## Finding a Page ID

1. Go to the page you want to use as parent
2. Click "..." → "Page Information"
3. The page ID is in the URL: `https://yourcompany.atlassian.net/wiki/pages/viewinfo.action?pageId=123456`

## Troubleshooting

### "❌ CONFLUENCE_URL not set"

Environment variables not loaded. Run:
```bash
source ~/.zshrc  # or source ~/.bashrc
```

### "❌ Failed to create page"

Common issues:
1. **Space doesn't exist**: Create the space in Confluence first
2. **No permissions**: Ensure you have write access to the space
3. **Invalid parent page ID**: Check the parent page ID is correct

### Pages Look Strange

The script uses Confluence's markdown macro. If your instance doesn't have it:
1. Install the Markdown macro from Atlassian Marketplace
2. Or edit the `publish-docs.sh` script to use HTML conversion instead

## Manual Publishing

If the script doesn't work, you can manually copy-paste the docs:

1. Go to your Confluence space
2. Create a new page
3. Click "Insert" → "Markup" → "Markdown"
4. Paste the contents of any `.md` file
5. Repeat for each document

## Script Details

### setup-confluence.sh
- Checks environment variables
- Installs confluence CLI (optional)
- Validates configuration

### publish-docs.sh
- Uses Confluence REST API
- Supports creating and updating pages
- Preserves markdown formatting
- Creates parent-child page hierarchy

## Advanced Usage

### Publish to Multiple Spaces

```bash
# Publish to Engineering space
./scripts/publish-docs.sh ENG

# Publish to Product space
./scripts/publish-docs.sh PRODUCT
```

### Automate Updates

Add to a cron job or CI/CD pipeline:

```bash
# Update docs daily at 2am
0 2 * * * cd /path/to/forecast && ./scripts/publish-docs.sh FORECAST
```

## Support

For issues with:
- **Scripts**: Check this README and troubleshooting section
- **Confluence API**: See https://developer.atlassian.com/cloud/confluence/rest/
- **Permissions**: Contact your Confluence admin
