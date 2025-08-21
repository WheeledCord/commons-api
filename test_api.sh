#!/bin/bash

echo "Testing Chat App API..."

# Start server in background
./chatapp &
SERVER_PID=$!
sleep 2

# Test registration
echo "Testing user registration..."
REGISTER_RESPONSE=$(curl -s -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"testpass"}')

echo "Register response: $REGISTER_RESPONSE"

# Extract token
TOKEN=$(echo $REGISTER_RESPONSE | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
echo "Token: $TOKEN"

# Test hall creation
echo "Testing hall creation..."
HALL_RESPONSE=$(curl -s -X POST http://localhost:8080/api/halls/create \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"Test Hall"}')

echo "Hall response: $HALL_RESPONSE"

# Test getting halls
echo "Testing get halls..."
HALLS_RESPONSE=$(curl -s -X GET http://localhost:8080/api/halls \
  -H "Authorization: Bearer $TOKEN")

echo "Halls response: $HALLS_RESPONSE"

# Clean up
kill $SERVER_PID 2>/dev/null
rm -f chat.db

echo "API test completed!"
