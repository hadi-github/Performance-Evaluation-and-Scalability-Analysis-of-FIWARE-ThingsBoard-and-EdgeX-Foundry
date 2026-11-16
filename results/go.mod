// Evidence can contain copied Go source snapshots with conflicting package
// clauses. This module boundary keeps `go build ./...` and `go test ./...`
// from treating retained evidence as buildable source.
module iotbench/results

go 1.27
