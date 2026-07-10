# bmstu-practice-2026
1. go.mod & go.sum
2. создать и запустить 6 docker compose. Используемые порты:
   * postgre: 5632 и 5633
   * elasticsearch: 9200 и 9201
   * clickhouse: 9000 и 9001
3. для Elasticsearch запустить:
    * ```go run .\cmd\elastic_loader\main.go -port 9200```
    * ```go run .\cmd\elastic_loader\main.go -port 9201```
4. запускать по очереди файлы из .\cmd\perfomance\...
