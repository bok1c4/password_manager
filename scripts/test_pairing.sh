#!/bin/bash

set -e

echo "=== Password Manager Pairing Test (Direct Connect) ==="

# Test directories
DEVICE_A_BASE="/tmp/pwman-device-a"
DEVICE_B_BASE="/tmp/pwman-device-b"

# Cleanup function
cleanup() {
    echo "Cleaning up..."
    pkill -f "pwman-server" 2>/dev/null || true
    rm -rf "$DEVICE_A_BASE" "$DEVICE_B_BASE" 2>/dev/null || true
}

trap cleanup EXIT

# Clean up any existing processes
cleanup 2>/dev/null || true

echo "Step 1: Setting up Device A (Generator)..."
export PWMAN_BASE_PATH="$DEVICE_A_BASE"
export PWMAN_PORT="18475"

# Start server A in background
cd /home/user/Projects/fun-projects/password_manager
./pwman-server > /tmp/server-a.log 2>&1 &
SERVER_A_PID=$!
echo "Started Server A (PID: $SERVER_A_PID)"

sleep 2

# Check if server is running
curl -s http://localhost:18475/api/ping || { echo "Server A not responding"; cat /tmp/server-a.log; exit 1; }

# Initialize vault on Device A
echo "Step 2: Initializing vault on Device A..."
curl -s -X POST http://localhost:18475/api/init \
    -H "Content-Type: application/json" \
    -d '{"vault": "work", "name": "Device-A", "password": "test123456"}' | jq .

sleep 1

# Unlock vault on Device A
echo "Step 3: Unlocking vault on Device A..."
curl -s -X POST http://localhost:18475/api/unlock \
    -H "Content-Type: application/json" \
    -d '{"password": "test123456"}' | jq .

sleep 1

# Start P2P on Device A and get address
echo "Step 4: Starting P2P on Device A..."
curl -s -X POST http://localhost:18475/api/p2p/start | jq .

sleep 2

# Get P2P status and address
echo "Step 5: Getting P2P address from Device A..."
P2P_STATUS=$(curl -s http://localhost:18475/api/p2p/status)
echo "$P2P_STATUS" | jq .
P2P_ADDR=$(echo "$P2P_STATUS" | jq -r '.data.addresses[0]' 2>/dev/null || echo "")
P2P_PEER_ID=$(echo "$P2P_STATUS" | jq -r '.data.peer_id' 2>/dev/null || echo "")
echo "Device A P2P address: $P2P_ADDR"
echo "Device A P2P peer ID: $P2P_PEER_ID"

# Construct full multiaddr with peer ID
if [ -n "$P2P_PEER_ID" ]; then
    P2P_ADDR="${P2P_ADDR}/p2p/${P2P_PEER_ID}"
    echo "Full multiaddr: $P2P_ADDR"
fi

if [ -z "$P2P_ADDR" ] || [ "$P2P_ADDR" = "null" ]; then
    echo "Failed to get P2P address"
    cat /tmp/server-a.log
    exit 1
fi

# Add some test passwords
echo "Step 6: Adding test passwords..."
curl -s -X POST http://localhost:18475/api/entries/add \
    -H "Content-Type: application/json" \
    -d '{"site": "github.com", "username": "user1", "password": "password123"}' | jq .

curl -s -X POST http://localhost:18475/api/entries/add \
    -H "Content-Type: application/json" \
    -d '{"site": "gmail.com", "username": "user2", "password": "password456"}' | jq .

echo "Listing entries on Device A:"
curl -s http://localhost:18475/api/entries | jq .

# Generate pairing code on Device A
echo "Step 7: Generating pairing code on Device A..."
PAIRING_RESPONSE=$(curl -s -X POST http://localhost:18475/api/pairing/generate)
echo "$PAIRING_RESPONSE" | jq .
PAIRING_CODE=$(echo "$PAIRING_RESPONSE" | jq -r '.data.code')
echo "Pairing code: $PAIRING_CODE"

# Now set up Device B (Joiner)
echo "Step 8: Setting up Device B (Joiner)..."
export PWMAN_BASE_PATH="$DEVICE_B_BASE"
export PWMAN_PORT="18476"

# Start server B in background
cd /home/user/Projects/fun-projects/password_manager
./pwman-server > /tmp/server-b.log 2>&1 &
SERVER_B_PID=$!
echo "Started Server B (PID: $SERVER_B_PID)"

sleep 2

# Check if server B is running
curl -s http://localhost:18476/api/ping || { echo "Server B not responding"; cat /tmp/server-b.log; exit 1; }

# Initialize vault on Device B (needed for keypair before pairing)
echo "Step 8b: Initializing vault on Device B..."
curl -s -X POST http://localhost:18476/api/init \
    -H "Content-Type: application/json" \
    -d '{"vault": "work", "name": "Device-B", "password": "test123456"}' | jq .

sleep 1

# Unlock vault on Device B
echo "Step 8c: Unlocking vault on Device B..."
curl -s -X POST http://localhost:18476/api/unlock \
    -H "Content-Type: application/json" \
    -d '{"password": "test123456"}' | jq .

sleep 1

# Start P2P on Device B
echo "Step 9: Starting P2P on Device B..."
curl -s -X POST http://localhost:18476/api/p2p/start | jq .

sleep 2

# Connect Device B to Device A directly
echo "Step 10: Connecting Device B to Device A..."
CONNECT_RESPONSE=$(curl -s -X POST http://localhost:18476/api/p2p/connect \
    -H "Content-Type: application/json" \
    -d "{\"address\": \"$P2P_ADDR\"}")
echo "$CONNECT_RESPONSE" | jq .

sleep 2

# Now join from Device B (using the same local network discovery flow)
echo "Step 11: Joining vault from Device B..."
JOIN_RESPONSE=$(curl -s -X POST http://localhost:18476/api/pairing/join \
    -H "Content-Type: application/json" \
    -d "{\"code\": \"$PAIRING_CODE\", \"device_name\": \"Device-B\", \"password\": \"test123456\"}")
echo "$JOIN_RESPONSE" | jq .

# Check if join was successful
SUCCESS=$(echo "$JOIN_RESPONSE" | jq -r '.success')
if [ "$SUCCESS" = "true" ]; then
    echo "Pairing successful!"
    
    # Wait a moment for sync to complete
    sleep 3
    
    # Check entries on Device B
    echo "Step 12: Checking entries on Device B:"
    curl -s http://localhost:18476/api/entries | jq .
    
    # Try to unlock and get password
    echo "Step 13: Unlocking vault on Device B..."
    curl -s -X POST http://localhost:18476/api/unlock \
        -H "Content-Type: application/json" \
        -d '{"password": "test123456"}' | jq .
    
    sleep 1
    
    # Get a password
    echo "Step 14: Getting password from Device B:"
    curl -s "http://localhost:18476/api/entries/get_password?site=github.com" | jq .
    
    echo "=== TEST PASSED ==="
else
    echo "Pairing failed!"
    echo "$JOIN_RESPONSE" | jq .
    echo "Server A log:"
    cat /tmp/server-a.log
    echo "Server B log:"
    cat /tmp/server-b.log
    echo "=== TEST FAILED ==="
    exit 1
fi

echo "Test complete!"
