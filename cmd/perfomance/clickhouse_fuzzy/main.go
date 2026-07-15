package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	host = "127.0.0.1"
	port = 8124
)

func main() {
	fmt.Println("Connected to ClickHouse (Fuzzy Search)")
	queries := loadQueries("data/queries_fuzzy.csv")
	var (
		total time.Duration
		max   time.Duration
	)
	var min = time.Hour

	for i, q := range queries {
		start := time.Now()

		escaped := escape(q)
		sql := fmt.Sprintf(`
			SELECT id
			FROM people
			WHERE ngramDistanceCaseInsensitiveUTF8(full_name, '%s') <= 0.45
			ORDER BY ngramDistanceCaseInsensitiveUTF8(full_name, '%s')
			LIMIT 1
		`, escaped, escaped)

		resp, err := http.Post(
			fmt.Sprintf("http://%s:%d/", host, port),
			"text/plain",
			strings.NewReader(sql),
		)

		if err != nil {
			log.Fatal(err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			log.Fatal(err)
		}

		if len(strings.TrimSpace(string(body))) == 0 {
			log.Fatalf("Query not found: %s", q)
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
	fmt.Printf("Minimum : %.5f\n", float64(min.Nanoseconds())/1000)
	fmt.Printf("Maximum : %v\n", max)
	fmt.Printf("Total : %v\n", total)
	fmt.Printf("QPS : %v\n", qps)
}

func escape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
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
