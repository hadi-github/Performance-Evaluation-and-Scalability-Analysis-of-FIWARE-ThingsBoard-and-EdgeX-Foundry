
# Workloads

This branch contains JMeter test plans

## Commands and Scripts
- To run a JMeter test:
  ```bash
  jmeter -n -t testplan.jmx -l results.jtl
  ```

some plans have user defined variables to handle the amount of sending data but some requires change those values from its source code, if you cant find user defined variables in gui mode, try to search the source of the jmx file and find the variables that are defined to work on
the influxdb listener should be configured with your own influxdb setup and accessToken
