# bmstu-practice-2026
1. go.mod & go.sum
2. создать и запустить 6 docker compose. Используемые порты:
   * postgre: 5632 и 5633
   * elasticsearch: 9200 и 9201
   * clickhouse: 9000 и 9001
3. для Elasticsearch (создаст индекс и запишет документы в него):
    * ```go run .\cmd\elastic_loader\main.go -port 9200```
    * ```go run .\cmd\elastic_loader\main.go -port 9201```
4. запускать по очереди файлы из .\cmd\perfomance\...

## Анализ и исправления производительности поиска

Основная причина, по которой Elasticsearch мог показывать результат хуже ClickHouse, была не в «медленности» Elasticsearch как движка, а в настройках стенда:

* индекс Elasticsearch создавался с анализатором по умолчанию, без явной нормализации русских ФИО и без разных настроек для загрузки и поиска;
* загрузчик не пересоздавал индекс, поэтому повторные загрузки могли оставлять старую схему и мешать воспроизводимости;
* bulk-загрузка не отключала refresh и не проверяла item-level ошибки bulk API;
* полнотекстовый `match` в Elasticsearch использовал стандартный `OR` между токенами, то есть семантически отличался от ClickHouse-запроса с тремя `hasToken(... ) AND ...`;
* Elasticsearch-бенчмарки запрашивали лишние данные по умолчанию (`_source`, подсчёт total hits), хотя для проверки нужен только факт найденного `id`;
* ClickHouse skip indexes добавлялись после вставки данных, но не материализовались для уже существующих строк, поэтому корректнее явно выполнять `MATERIALIZE INDEX`;
* fuzzy-запрос ClickHouse сортировал всю таблицу по расстоянию без предварительного условия, что превращало тест в полный scan и не использовало ngram-индекс как отсекающий фильтр.

Текущая схема сравнения:

* PostgreSQL full-text: GIN по `to_tsvector('russian', full_name)` и запрос через `plainto_tsquery`.
* PostgreSQL fuzzy: GiST trigram index `gist_trgm_ops` и KNN-запрос `ORDER BY full_name <-> $1 LIMIT 1`.
* ClickHouse full-text/token search: `tokenbf_v1` по `full_name`, запрос через три `hasToken` с `AND`.
* ClickHouse fuzzy: `ngrambf_v1` по `full_name`, предварительный фильтр по `ngramDistanceCaseInsensitiveUTF8(...) <= 0.45` и затем сортировка ближайших кандидатов.
* Elasticsearch full-text: custom analyzer `standard + lowercase`, batched `_msearch` по 100 запросов, `match` с `operator: and`, `_source: false`, `track_total_hits: false`.
* Elasticsearch fuzzy: тот же analyzer, batched `_msearch` по 100 запросов, `match` с `operator: and`, `fuzziness: AUTO`, `prefix_length: 1`, `max_expansions: 20`, `_source: false`, `track_total_hits: false`.

После изменения SQL init-файлов нужно пересоздать соответствующие контейнеры/volumes, иначе Docker не применит init-скрипты к уже созданным базам. Elasticsearch-загрузчик по умолчанию удаляет и пересоздаёт индекс; если нужно сохранить индекс, используйте `-recreate=false`.
