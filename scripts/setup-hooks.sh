#!/bin/bash
# Setup git hooks for Pinchtab development

YELLOW='\033[1;33m'
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${YELLOW}🦀 Pinchtab Git Hooks Setup${NC}"
echo ""

# Check if we're in the right directory
if [ ! -d ".githooks" ]; then
    echo -e "${RED}✗ Error: .githooks directory not found${NC}"
    echo "  Please run this script from the Pinchtab root directory"
    exit 1
fi

# Check current hooks path
CURRENT_PATH=$(git config core.hooksPath)

if [ "$CURRENT_PATH" = ".githooks" ]; then
    echo -e "${GREEN}✓ Git hooks already configured correctly${NC}"
    echo "  Pre-commit checks will run automatically"
else
    echo "Setting up git hooks..."
    git config core.hooksPath .githooks
    
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Git hooks configured successfully${NC}"
        echo ""
        echo "Pre-commit hook will now check:"
        echo "  • gofmt formatting"
        echo "  • go vet"
        echo "  • go test"
        echo ""
        echo -e "${GREEN}Ready to commit with confidence! 🚀${NC}"
    else
        echo -e "${RED}✗ Failed to configure git hooks${NC}"
        exit 1
    fi
fi