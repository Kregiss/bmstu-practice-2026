package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	host = "127.0.0.1"
	port = 9201
)

type SearchResponse struct {
	Hits struct {
		Hits []struct {
			ID string `json:"_id"`
		} `json:"hits"`
	} `json:"hits"`
}

func main() {
	fmt.Println("Connected to Elasticsearch (Fuzzy Search)")
	queries := loadQueries("data/queries_fuzzy.csv")
	var (
		total time.Duration
		min   time.Duration
		max   time.Duration
	)
	min = time.Hour
	url := fmt.Sprintf("http://%s:%d/people/_search", host, port)
	client := &http.Client{}

	for i, q := range queries {
		query := map[string]interface{}{
			"size": 1,
			"query": map[string]interface{}{
				"match": map[string]interface{}{
					"full_name": map[string]interface{}{
						"query": q,
						"fuzziness": "AUTO",
					},
				},
			},
		}

		body, err := json.Marshal(query)
		if err != nil {
			log.Fatal(err)
		}

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
		if err != nil {
			log.Fatal(err)
		}

		req.Header.Set("Content-Type", "application/json")
		start := time.Now()

		resp, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}

		var result SearchResponse

		err = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if err != nil {
			log.Fatal(err)
		}
		if len(result.Hits.Hits) == 0 {
			log.Fatalf("Nothing found for query: %s", q)
		}

		elapsed := time.Since(start)
		total += elapsed

		if elapsed < min {
			min = elapsed
		}
		if elapsed > max {
			max = elapsed
		}
		if (i+1)%100 == 0 {
			fmt.Printf("%d/%d completed\n", i+1, len(queries))
		}
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