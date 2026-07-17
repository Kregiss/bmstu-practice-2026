package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	host     = "localhost"
	port     = 5633
	user     = "postgres"
	password = "postgres"
	dbname   = "people"
)

func main() {
	ctx := context.Background()

	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		user,
		password,
		host,
		port,
		dbname,
	)

	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		log.Fatal(err)
	}

	config.MinConns = 2
	config.MaxConns = 10

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected to PostgreSQL (Fuzzy Search)")

	queries := loadQueries("data/queries_fuzzy.csv")

	var (
		total time.Duration
		min   = time.Hour
		max   time.Duration
	)

	query := `
		SELECT id
		FROM people
		ORDER BY full_name <-> $1
		LIMIT 1;
	`

	for i, q := range queries {
		start := time.Now()

		rows, err := pool.Query(ctx, query, q)
		if err != nil {
			log.Fatal(err)
		}

		found := false

		for rows.Next() {
			var id int64

			if err := rows.Scan(&id); err != nil {
				rows.Close()
				log.Fatal(err)
			}

			found = true
		}

		if err := rows.Err(); err != nil {
			rows.Close()
			log.Fatal(err)
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
	}

	avg := total / time.Duration(len(queries))
	qps := float64(len(queries)) / total.Seconds()

	fmt.Println()
	fmt.Println("-------------- RESULT --------------")
	fmt.Printf("Queries : %d\n", len(queries))
	fmt.Printf("Average : %v\n", avg)
	fmt.Printf("Minimum : %v\n", min)
	fmt.Printf("Maximum : %v\n", max)
	fmt.Printf("Total   : %v\n", total)
	fmt.Printf("QPS     : %.2f\n", qps)
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
		queries = append(queries, row[0])
	}

	return queries
}