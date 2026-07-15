* `docker compose up -d`
* `docker compose down -v`

-- elastic
* `curl http://localhost:9200`

-- clickhouse
* `docker exec -it clickhouse_full clickhouse-client`

-- postgres
* `docker exec -it postgres psql -U postgres`


`go run .\cmd\perfomance\elastic_full\main.go`
