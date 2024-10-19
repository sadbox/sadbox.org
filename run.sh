#!/bin/bash -eux
sudo docker build -t sadbox/sadbox-web .
sudo docker run --rm --mount type=bind,source="$(pwd)",target=/db -p 9000:9000 sadbox/sadbox-web
