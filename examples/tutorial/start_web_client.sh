#!/bin/sh
cd "$(dirname "$0")"

cd run
../../../rufs client -web localhost:9090
