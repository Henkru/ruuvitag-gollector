#!/usr/bin/env bash

TAGS="influxdb postgresql gcp aws"

docker run \
  -it \
  --rm \
  --network ruuvitag \
  -v "$(pwd):/go/src/app" \
  ruuvitag-gollector:latest \
  test -tags "$TAGS" ./...
