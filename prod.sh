#!/bin/bash

mkdir images

sudo sshfs root@xxx.xxx.xxx.xxx/mnt/images ./images -o nonempty

export POSTGRES_USER=xxx
export POSTGRES_PASSWORD=xxx
export POSTGRES_DB=xxx
export POSTGRES_PORT_5432_TCP_ADDR=xxx
export POSTGRES_PORT_5432_TCP_PORT=xxx
export IMAGE_SERVER=http://xxx.xxx.xxx
./createFolder.py

go get .

go build . 
