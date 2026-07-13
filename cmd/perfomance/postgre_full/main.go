package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

const (
    host     = "127.0.0.1"
    port     = 5632
    user     = "postgres"
    password = "postgres"
    dbname   = "people"
)

func main() {

	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host,
		port,
		user,
		password,
		dbname,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected to PostgreSQL")

	queries := loadQueries("data/queries.csv")

	var (
		total time.Duration
		min   time.Duration
		max   time.Duration
	)

	min = time.Hour

	for i, q := range queries {

		start := time.Now()

		rows, err := db.Query(`
			SELECT id
			FROM people
			WHERE to_tsvector('russian', full_name)
			      @@ plainto_tsquery('russian', $1)
			LIMIT 1
		`, q)

		if err != nil {
			log.Fatal(err)
		}

		found := false

		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				log.Fatal(err)
			}
			found = true
		}

		rows.Close()

		if !found {
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
		if i==1000 {break}
	}

	qps := float64(len(queries)) / total.Seconds()
	avg := total / time.Duration(len(queries))

	fmt.Println()
	fmt.Println("========== RESULT: ==========")
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