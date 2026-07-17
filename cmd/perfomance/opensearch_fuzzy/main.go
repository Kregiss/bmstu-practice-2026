package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

const (
	host = "127.0.0.1"
	port = 9203
	indexName = "people"
	batchSize = 100
)

func main() {
	client, err := opensearchapi.NewClient(
		opensearchapi.Config{
			Client: opensearch.Config{
				Addresses: []string{
					fmt.Sprintf(
						"http://%s:%d",
						host,
						port,
					),
				},
			},
		},
	)

	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Connected to OpenSearch (Fuzzy Search)")
	queries := loadQueries("data/queries_fuzzy.csv")
	ctx := context.Background()
	var (
		total time.Duration
		min   time.Duration
		max   time.Duration
	)
	min = time.Hour

	for batchStart := 0; batchStart < len(queries); batchStart += batchSize {
		batchEnd := batchStart + batchSize
		if batchEnd > len(queries) {
			batchEnd = len(queries)
		}
		var body bytes.Buffer

		for _, q := range queries[batchStart:batchEnd] {
			// msearch header
			body.WriteString(
				fmt.Sprintf(
					`{"index":"%s"}`,
					indexName,
				),
			)
			body.WriteByte('\n')

			query := map[string]interface{}{
				"size": 1,
				"_source": false,
				"track_total_hits": false,
				"query": map[string]interface{}{
					"match": map[string]interface{}{
						"full_name": map[string]interface{}{
							"query": q,
							"operator": "and",
							"fuzziness": 1,
							"prefix_length": 0,
							"max_expansions": 5,
						},
					},
				},
			}

			payload, err := json.Marshal(query)
			if err != nil {log.Fatal(err)}
			body.Write(payload)
			body.WriteByte('\n')
		}

		start := time.Now()
		resp, err := client.MSearch(
			ctx,
			opensearchapi.MSearchReq{
				Body: bytes.NewReader(
					body.Bytes(),
				),
			},
		)
		if err != nil {log.Fatal(err)}

		if len(resp.Responses) != batchEnd-batchStart {
			log.Fatalf("expected %d responses, got %d", batchEnd-batchStart, len(resp.Responses))
		}
		for j, item := range resp.Responses {
			if item.Error != nil {
				log.Fatalf("search failed for query %q: %+v", queries[batchStart+j], item.Error)
			}
			if len(item.Hits.Hits) == 0 {
				log.Fatalf("Nothing found for query: %s", queries[batchStart+j])
			}
		}

		elapsed := time.Since(start)
		total += elapsed
		perQuery :=	elapsed / time.Duration(batchEnd - batchStart)
		if perQuery < min { min = perQuery }
		if perQuery > max { max = perQuery }
		fmt.Printf("%d/%d completed\n", batchEnd, len(queries))
	}

	qps := float64(len(queries)) / total.Seconds()
	avg := total / time.Duration(len(queries))
	fmt.Println()
	fmt.Println("-------------- RESULT --------------")
	fmt.Printf("Queries : %d\n", len(queries))
	fmt.Printf("Average : %v\n", avg)
	fmt.Printf("Minimum : %v\n", min)
	fmt.Printf("Maximum : %v\n", max)
	fmt.Printf("Total : %v\n", total)
	fmt.Printf("QPS : %v\n", qps)
}

func loadQueries(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		log.Fatal(err)
	}

	queries := make([]string, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {continue}
		queries = append(queries, row[0])
	}
	return queries
}