package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	host = "127.0.0.1"
	port = 9201
)

type MultiSearchResponse struct {
	Responses []struct {
		Hits struct {
			Hits []struct {
				ID string `json:"_id"`
			} `json:"hits"`
		} `json:"hits"`
		Error  json.RawMessage `json:"error,omitempty"`
		Status int             `json:"status"`
	} `json:"responses"`
}

const batchSize = 100

func main() {
	fmt.Println("Connected to Elasticsearch (Fuzzy Search)")
	queries := loadQueries("data/queries_fuzzy.csv")
	var (
		total time.Duration
		min   time.Duration
		max   time.Duration
	)
	min = time.Hour
	url := fmt.Sprintf("http://%s:%d/people/_msearch?filter_path=responses.hits.hits._id,responses.error,responses.status", host, port)
	client := &http.Client{Timeout: 30 * time.Second}

	for batchStart := 0; batchStart < len(queries); batchStart += batchSize {
		batchEnd := batchStart + batchSize
		if batchEnd > len(queries) {
			batchEnd = len(queries)
		}

		var body bytes.Buffer
		for _, q := range queries[batchStart:batchEnd] {
			body.WriteString("{}\n")
			query := map[string]interface{}{
				"size":             1,
				"_source":          false,
				"track_total_hits": false,
				"query": map[string]interface{}{
					"match": map[string]interface{}{
						"full_name": map[string]interface{}{
							"query":          q,
							"operator":       "and",
							"fuzziness":      "AUTO",
							"prefix_length":  1,
							"max_expansions": 20,
						},
					},
				},
			}

			payload, err := json.Marshal(query)
			if err != nil {
				log.Fatal(err)
			}
			body.Write(payload)
			body.WriteByte('\n')
		}

		req, err := http.NewRequest("POST", url, &body)
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/x-ndjson")

		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			responseBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			log.Fatalf("msearch failed: %s: %s", resp.Status, string(responseBody))
		}

		var result MultiSearchResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if err != nil {
			log.Fatal(err)
		}
		if len(result.Responses) != batchEnd-batchStart {
			log.Fatalf("expected %d responses, got %d", batchEnd-batchStart, len(result.Responses))
		}
		for j, item := range result.Responses {
			if item.Status >= 400 || len(item.Error) > 0 {
				log.Fatalf("search failed for query %q: status=%d error=%s", queries[batchStart+j], item.Status, string(item.Error))
			}
			if len(item.Hits.Hits) == 0 {
				log.Fatalf("Nothing found for query: %s", queries[batchStart+j])
			}
		}

		elapsed := time.Since(start)
		total += elapsed
		perQuery := elapsed / time.Duration(batchEnd-batchStart)
		if perQuery < min {
			min = perQuery
		}
		if perQuery > max {
			max = perQuery
		}

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

	var queries []string
	for _, row := range rows {
		queries = append(queries, row[0])
	}
	return queries
}
