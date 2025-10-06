# Performance Evaluation and Scalability analysis for iot platforms

this branch is dedicated for the main hub of all documents and an starting point for working with the project

### branch guide
each platform has two method for deployment, the single deployment which will deploy all the needed modules of the platform in a single node
and multiple deployment is deploying modules in scaled format (3 instances)
and will be separated by two node

so each platform has [platform]-single/multiple is pointing to these two different architectures

the grafana_setup branch contains the deployment of the exporting hub, grafana will run with determind configs for having visual view for our metrics

the prometheus-setup branch is the exporters deployment which should be deployed on server among platform
