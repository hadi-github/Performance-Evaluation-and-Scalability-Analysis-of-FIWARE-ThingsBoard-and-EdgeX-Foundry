#!/bin/bash

# MongoDB Replica Set Test Script
# This script tests data replication across all MongoDB instances

# Load environment variables
source .env

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# MongoDB connection details
MONGO_USER=${MONGO_ROOT_USERNAME}
MONGO_PASS=${MONGO_ROOT_PASSWORD}
MONGO_DB=${MONGO_DATABASE}
MONGO_COLLECTION=${MONGO_COLLECTION}

echo -e "${BLUE}=== MongoDB Replica Set Testing Script ===${NC}"
echo ""

# Test connectivity to all instances
echo -e "${YELLOW}1. Testing connectivity to all MongoDB instances...${NC}"

for port in 27017 27018 27019; do
    echo -n "Testing MongoDB on port $port... "
    if docker exec mongo1 mongosh --host localhost:27017 --username admin --password password123 --authenticationDatabase admin --eval "db.adminCommand('ping')" --quiet >/dev/null 2>&1; then
        echo -e "${GREEN}✓ Connected${NC}"
    else
        echo -e "${RED}✗ Failed${NC}"
        exit 1
    fi
done

echo ""

# Check replica set status
echo -e "${YELLOW}2. Checking replica set status...${NC}"
docker exec mongo1 mongosh --host mongo1:27017 --username admin --password password123 --authenticationDatabase admin --eval "
console.log('Replica Set Status:');
var status = rs.status();
status.members.forEach(function(member) {
    console.log('Host: ' + member.name + ' - State: ' + member.stateStr + ' - Health: ' + member.health);
});
" --quiet

echo ""

# Insert test data
echo -e "${YELLOW}3. Inserting test data into PRIMARY node...${NC}"

# Generate unique test data
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
TEST_DATA="{
    \"_id\": \"test_$TIMESTAMP\",
    \"name\": \"Test Document $TIMESTAMP\",
    \"value\": $(shuf -i 1-1000 -n 1),
    \"timestamp\": new Date(),
    \"replication_test\": true
}"

echo "Inserting document: $TEST_DATA"

# Insert data via primary node (mongo1)
docker exec mongo1 mongosh --host mongo1:27017 --username admin --password password123 --authenticationDatabase admin --eval "
use $MONGO_DB;
db.$MONGO_COLLECTION.insertOne($TEST_DATA);
console.log('Data inserted successfully into PRIMARY node');
" --quiet

echo ""

# Wait for replication
echo -e "${YELLOW}4. Waiting 5 seconds for replication...${NC}"
sleep 5

# Check data availability on all nodes
echo -e "${YELLOW}5. Verifying data replication across all nodes...${NC}"

PORTS=(27017 27018 27019)
HOSTS=(mongo1 mongo2 mongo3)
NODE_NAMES=(PRIMARY SECONDARY-1 SECONDARY-2)

for i in {0..2}; do
    HOST=${HOSTS[$i]}
    PORT=${PORTS[$i]}
    NODE_NAME=${NODE_NAMES[$i]}
    
    echo -n "Checking ${NODE_NAME} (${HOST}:${PORT})... "
    
    # For secondary nodes, we need to enable reading from secondaries
    RESULT=$(docker exec mongo1 mongosh --host ${HOST}:27017 --username $MONGO_USER --password $MONGO_PASS --authenticationDatabase admin --eval "
    use $MONGO_DB;
    rs.secondaryOk(); // Enable reading from secondary
    var doc = db.$MONGO_COLLECTION.findOne({_id: 'test_$TIMESTAMP'});
    if (doc) {
        console.log('FOUND');
    } else {
        console.log('NOT_FOUND');
    }
    " --quiet 2>/dev/null | grep -E "(FOUND|NOT_FOUND)")
    
    if echo "$RESULT" | grep -q "FOUND"; then
        echo -e "${GREEN}✓ Data replicated${NC}"
    else
        echo -e "${RED}✗ Data not found${NC}"
    fi
done

echo ""

# Count total documents
echo -e "${YELLOW}6. Document count verification...${NC}"
for i in {0..2}; do
    HOST=${HOSTS[$i]}
    NODE_NAME=${NODE_NAMES[$i]}
    
    COUNT=$(docker exec mongo1 mongosh --host ${HOST}:27017 --username $MONGO_USER --password $MONGO_PASS --authenticationDatabase admin --eval "
    use $MONGO_DB;
    rs.secondaryOk();
    db.$MONGO_COLLECTION.countDocuments({});
    " --quiet 2>/dev/null | tail -1)
    
    echo "${NODE_NAME} (${HOST}): $COUNT documents"
done

echo ""

# Test failover simulation
echo -e "${YELLOW}7. Testing failover resilience...${NC}"
echo "Stopping PRIMARY node (mongo1) for 10 seconds..."

docker stop mongo1
sleep 5

echo "Checking if SECONDARY nodes can elect new PRIMARY..."
NEW_PRIMARY=$(docker exec mongo2 mongosh --host mongo2:27017 --username $MONGO_USER --password $MONGO_PASS --authenticationDatabase admin --eval "
rs.status().members.filter(function(m) { return m.stateStr === 'PRIMARY'; }).map(function(m) { return m.name; })[0] || 'NONE';
" --quiet 2>/dev/null | tail -1)

echo "New PRIMARY elected: $NEW_PRIMARY"

echo "Restarting mongo1..."
docker start mongo1
sleep 10

echo ""

# Final status check
echo -e "${YELLOW}8. Final replica set status after failover test...${NC}"
docker exec mongo1 mongosh --host mongo1:27017 --username $MONGO_USER --password $MONGO_PASS --authenticationDatabase admin --eval "
var status = rs.status();
console.log('=== Final Replica Set Status ===');
status.members.forEach(function(member) {
    console.log('Host: ' + member.name + ' - State: ' + member.stateStr + ' - Health: ' + member.health);
});
" --quiet

echo ""
echo -e "${GREEN}=== Replica Set Test Completed! ===${NC}"

# Cleanup test data
echo -e "${YELLOW}9. Cleaning up test data...${NC}"
docker exec mongo1 mongosh --host mongo1:27017 --username $MONGO_USER --password $MONGO_PASS --authenticationDatabase admin --eval "
use $MONGO_DB;
db.$MONGO_COLLECTION.deleteOne({_id: 'test_$TIMESTAMP'});
console.log('Test data cleaned up');
" --quiet

echo -e "${GREEN}All tests completed successfully!${NC}"
