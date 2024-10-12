#!/bin/bash -eux
sudo docker build -t sadbox/sadbox-web .
sudo docker push sadbox/sadbox-web
