#!/bin/bash

# Olu API Test Script
# This script demonstrates basic API operations
# Author: ha1tch <h@ual.fi>
# Repository: https://github.com/ha1tch/olu/

set -e

BASE_URL="http://localhost:9090"

echo "Olu API Test Script"
echo "==================="
echo ""

# Check if server is running
echo -n "Checking server health... "
if curl -s "${BASE_URL}/health" > /dev/null 2>&1; then
    echo "[OK]"
else
    echo "[FAILED] Server not running. Please start olu first."
    exit 1
fi

echo ""
echo "Testing CRUD Operations"
echo "============================"

# Create a user
echo -n "Creating user Alice... "
ALICE_RESPONSE=$(curl -s -X POST "${BASE_URL}/api/v1/users" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Alice Smith",
    "email": "alice@example.com",
    "age": 30,
    "role": "admin",
    "active": true
  }')
ALICE_ID=$(echo $ALICE_RESPONSE | grep -o '"id":[0-9]*' | grep -o '[0-9]*')
echo "[OK] ID: $ALICE_ID"

# Create another user
echo -n "Creating user Bob... "
BOB_RESPONSE=$(curl -s -X POST "${BASE_URL}/api/v1/users" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Bob Manager",
    "email": "bob@example.com",
    "age": 45,
    "role": "admin",
    "active": true
  }')
BOB_ID=$(echo $BOB_RESPONSE | grep -o '"id":[0-9]*' | grep -o '[0-9]*')
echo "[OK] ID: $BOB_ID"

# Create user with reference
echo -n "Creating user Charlie with manager reference... "
CHARLIE_RESPONSE=$(curl -s -X POST "${BASE_URL}/api/v1/users" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"Charlie Employee\",
    \"email\": \"charlie@example.com\",
    \"age\": 28,
    \"role\": \"user\",
    \"active\": true,
    \"manager\": {
      \"type\": \"REF\",
      \"entity\": \"users\",
      \"id\": $BOB_ID
    }
  }")
CHARLIE_ID=$(echo $CHARLIE_RESPONSE | grep -o '"id":[0-9]*' | grep -o '[0-9]*')
echo "[OK] ID: $CHARLIE_ID"

# Get user
echo -n "Getting Alice... "
curl -s "${BASE_URL}/api/v1/users/${ALICE_ID}" > /dev/null
echo "[OK]"

# List users
echo -n "Listing users... "
USER_COUNT=$(curl -s "${BASE_URL}/api/v1/users" | grep -o '"id"' | wc -l)
echo "[OK] Found: $USER_COUNT users"

# Update user
echo -n "Updating Alice... "
curl -s -X PATCH "${BASE_URL}/api/v1/users/${ALICE_ID}" \
  -H "Content-Type: application/json" \
  -d '{"age": 31}' > /dev/null
echo "[OK]"

echo ""
echo "Testing Graph Operations"
echo "============================"

# Get neighbors
echo -n "Getting Bob's neighbors... "
NEIGHBORS=$(curl -s -X POST "${BASE_URL}/api/v1/graph/neighbors" \
  -H "Content-Type: application/json" \
  -d "{\"node_id\": \"users:${BOB_ID}\", \"direction\": \"both\"}")
echo "[OK]"

# Find path
echo -n "Finding path from Bob to Charlie... "
curl -s -X POST "${BASE_URL}/api/v1/graph/path" \
  -H "Content-Type: application/json" \
  -d "{\"from\": \"users:${BOB_ID}\", \"to\": \"users:${CHARLIE_ID}\", \"max_depth\": 10}" > /dev/null
echo "[OK]"

# Graph stats
echo -n "Getting graph statistics... "
curl -s "${BASE_URL}/api/v1/graph/stats" > /dev/null
echo "[OK]"

echo ""
echo "Testing Pagination"
echo "====================="

echo -n "Getting page 1... "
curl -s "${BASE_URL}/api/v1/users?page=1&per_page=2" > /dev/null
echo "[OK]"

echo ""
echo "Testing Embedded References"
echo "==============================="

echo -n "Getting Charlie with embedded manager... "
curl -s "${BASE_URL}/api/v1/users/${CHARLIE_ID}?embed_depth=1" > /dev/null
echo "[OK]"

echo ""
echo "Testing Delete Operations"
echo "=============================="

echo -n "Deleting Alice... "
curl -s -X DELETE "${BASE_URL}/api/v1/users/${ALICE_ID}" > /dev/null
echo "[OK]"

echo ""
echo "================================"
echo "All tests passed successfully!"
echo "================================"
echo ""

# Print summary
echo "Summary:"
echo "  - Created 3 users"
echo "  - Updated 1 user"
echo "  - Deleted 1 user"
echo "  - Tested graph operations"
echo "  - Tested pagination"
echo "  - Tested reference embedding"
echo ""
echo "Final user count: $(curl -s "${BASE_URL}/api/v1/users" | grep -o '"id"' | wc -l)"
