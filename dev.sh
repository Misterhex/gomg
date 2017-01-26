export POSTGRES_USER=xxx
export POSTGRES_PASSSWORD=xxx
export POSTGRES_DB=xxx
export POSTGRES_PORT_5432_TCP_ADDR=xxx
export POSTGRES_PORT_5432_TCP_PORT=xxx
export IMAGE_SERVER=https://xxx

go run job.go replaceSpecial.go	--runMode=top30
