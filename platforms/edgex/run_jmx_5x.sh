#!/bin/bash

JMX_FILE="/home/frog/Desktop/research-platforms/platforms/edgex/edgex-plan.jmx"
RESULTS_DIR="/home/frog/Desktop/research-platforms/platforms/edgex"

for i in {1..5}
do
    echo "Run $i: Starting JMeter test..."
    jmeter -n -t "$JMX_FILE" -l "$RESULTS_DIR/results_run_${i}.jtl"
    if [ $i -lt 5 ]; then
        echo "Waiting 1 minute before next run..."
        sleep 60
    fi
done

echo "All 5 runs completed."
