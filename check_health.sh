#!/bin/bash

# EdgeX Foundry Health Check Script
# Checks /api/v3/ping endpoint for available services

echo "EdgeX Foundry Service Health Check"
echo "=================================="

# List of services with their ports
declare -A services=(
    ["core-metadata"]="59881"
    ["core-data"]="59880"
    ["core-command"]="59882"
    ["device-virtual"]="59900"
    ["device-rest"]="59986"
    ["app-rules-engine"]="59701"
    ["support-notifications"]="59860"
    ["support-scheduler"]="59861"
)

# Check each service
for service in "${!services[@]}"; do
    port=${services[$service]}
    url="http://localhost:$port/api/v3/ping"

    echo -n "Checking $service ($port): "

    # Use curl with timeout
    response=$(curl -s --max-time 5 "$url" 2>/dev/null)
    status=$?

    if [ $status -eq 0 ] && echo "$response" | grep -q "apiVersion"; then
        echo "✓ HEALTHY"
    else
        echo "✗ UNHEALTHY or UNAVAILABLE"
    fi
done

echo ""
echo "Health check complete."
