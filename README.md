# bmstu-practice-2026
1. go.mod & go.sum
2. создать и запустить 6 docker compose
3. для Elasticsearch запустить:
* ```go run .\cmd\elastic_loader\main.go -port 9200```
* ```go run .\cmd\elastic_loader\main.go -port 9201```
4. запускать по очереди файлы из .\cmd\perfomance\...
