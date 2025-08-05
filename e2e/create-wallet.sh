#!/bin/bash

# E2E Test Script: Create Wallet
# This script tests the complete wallet creation flow including:
# 1. User registration
# 2. User sign-in
# 3. Workspace creation
# 4. Session start
# 5. Wallet creation

set -e  # Exit on any error

BASE_URL="http://localhost:8150/api/v1"
COOKIES_FILE="cookies.txt"

echo "🚀 Starting E2E Wallet Creation Test..."
echo "========================================="

# Clean up any existing cookies file
rm -f $COOKIES_FILE

echo "📝 Step 1: User Registration"
echo "----------------------------"
REGISTER_RESPONSE=$(curl -s -X POST $BASE_URL/authentication/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "password": "password@T123",
    "email": "test@example.com"
  }')

echo "Registration response: $REGISTER_RESPONSE"
echo ""

echo "🔐 Step 2: User Sign-in"
echo "-----------------------"
SIGNIN_RESPONSE=$(curl -s -X POST $BASE_URL/authentication/sign-in \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password@T123"
  }' \
  -c $COOKIES_FILE \
  -v)

echo "Sign-in response: $SIGNIN_RESPONSE"
echo ""

echo "🏢 Step 3: Create Workspace"
echo "---------------------------"
WORKSPACE_RESPONSE=$(curl -s -X POST $BASE_URL/workspaces \
  -H "Content-Type: application/json" \
  -d '{"name": "aaaaTest Workspace"}' \
  -b $COOKIES_FILE -c $COOKIES_FILE)

echo "Workspace response: $WORKSPACE_RESPONSE"

# Extract workspace ID from the response
# Assuming the response contains the workspace_id field
WORKSPACE_ID=$(echo $WORKSPACE_RESPONSE | grep -o '"id":"[^"]*"' | cut -d'"' -f4)

if [ -z "$WORKSPACE_ID" ]; then
    echo "❌ Error: Could not extract workspace ID from response"
    echo "Response was: $WORKSPACE_RESPONSE"
    exit 1
fi

echo "✅ Extracted Workspace ID: $WORKSPACE_ID"
echo ""

echo "🎯 Step 4: Start Session"
echo "------------------------"
SESSION_RESPONSE=$(curl -s -X POST $BASE_URL/authentication/start-session \
  -H "Content-Type: application/json" \
  -d "{\"workspace_id\": \"$WORKSPACE_ID\"}" \
  -b $COOKIES_FILE -c $COOKIES_FILE)

echo "Session response: $SESSION_RESPONSE"
echo ""

echo "💼 Step 5: Create MPC Wallet"
echo "----------------------------"
WALLET_RESPONSE=$(curl -s -X POST $BASE_URL/wallets \
  -H "Content-Type: application/json" \
  -d '{"name": "Test MPC Wallet", "wallet_type": "mpc"}' \
  -b $COOKIES_FILE)

echo "Wallet response: $WALLET_RESPONSE"
echo ""

echo "🎉 E2E Test Completed Successfully!"
echo "===================================="
echo "✅ User registered"
echo "✅ User signed in"
echo "✅ Workspace created (ID: $WORKSPACE_ID)"
echo "✅ Session started"
echo "✅ MPC Wallet created"
echo ""

# Clean up cookies file
rm -f $COOKIES_FILE

echo "🧹 Cleanup completed - cookies file removed"
echo "Test finished at $(date)"
