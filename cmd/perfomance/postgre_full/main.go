package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5"
)

const (
	host     = "127.0.0.1"
	port     = 5632
	user     = "postgres"
	password = "postgres"
	dbname   = "people"

	warmupCount  = 100
	runsPerLevel = 3
)

var workerConfigs = []int{
	1,
	4,
	16,
	64,
	128,
}

type BenchmarkResult struct {
	Workers int

	Queries uint64

	Hits   uint64
	Misses uint64
	Errors uint64

	Avg time.Duration

	P50 time.Duration
	P95 time.Duration
	P99 time.Duration

	Max time.Duration

	WallTime time.Duration

	QPS float64
}

const querySQL = `
SELECT id
FROM people
WHERE to_tsvector('russian', full_name)
      @@ plainto_tsquery('russian', $1)
LIMIT 1
`

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

	config.MaxConns = 128
	config.MinConns = 4

	pool, err := pgxpool.NewWithConfig(
		ctx,
		config,
	)

	if err != nil {
		log.Fatal(err)
	}

	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected to PostgreSQL")
	queries := loadQueries("data/queries.csv")
	fmt.Println("warmup...")

	for i := 0; i < warmupCount && i < len(queries); i++ {
		_, _ = execute(ctx,pool,queries[i])
	}

	fmt.Println()
	for _, workers := range workerConfigs {
		fmt.Printf(
			"========== %d workers ==========\n",
			workers,
		)
		var results []BenchmarkResult

		for run := 1; run <= runsPerLevel; run++ {
			result := runBenchmark(
				ctx,
				pool,
				queries,
				workers,
			)

			results = append(
				results,
				result,
			)

			fmt.Printf(
				"run=%d workers=%d qps=%.0f avg=%v p50=%v p95=%v p99=%v errors=%d\n",
				run,
				workers,
				result.QPS,
				result.Avg,
				result.P50,
				result.P95,
				result.P99,
				result.Errors,
			)
		}
		printMedianResult(results)
		fmt.Println()
	}
}

func runBenchmark(
	ctx context.Context,
	pool *pgxpool.Pool,
	queries []string,
	workers int,
) BenchmarkResult {

	queryCh := make(
		chan string,
	)

	var wg sync.WaitGroup

	var (
		hits   atomic.Uint64
		misses atomic.Uint64
		errors atomic.Uint64
	)

	var (
		mu        sync.Mutex
		latencies []time.Duration
	)

	startWall := time.Now()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make(
				[]time.Duration,
				0,
			)

			for q := range queryCh {
				start := time.Now()
				found, err := execute(
					ctx,
					pool,
					q,
				)

				elapsed := time.Since(start)
				if err != nil {
					errors.Add(1)
					continue
				}

				local = append(
					local,
					elapsed,
				)

				if found {
					hits.Add(1)
				} else {
					misses.Add(1)
				}
			}

			mu.Lock()			
			latencies = append(
				latencies,
				local...,
			)
			mu.Unlock()
		}()
	}

	for _, q := range queries {
		queryCh <- q
	}

	close(queryCh)
	wg.Wait()
	wall := time.Since(startWall)

	slices.Sort(latencies)

	var total time.Duration

	for _, v := range latencies {
		total += v
	}

	result := BenchmarkResult{
		Workers: workers,

		Queries: uint64(len(queries)),

		Hits:   hits.Load(),
		Misses: misses.Load(),
		Errors: errors.Load(),

		WallTime: wall,

		QPS: float64(len(queries)) / wall.Seconds(),
	}

	if len(latencies) > 0 {
		result.Avg = total / time.Duration(len(latencies))
		result.P50 = percentile(latencies,50)
		result.P95 = percentile(latencies,95)
		result.P99 = percentile(latencies,99)
		result.Max = latencies[len(latencies)-1]
	}
	return result
}

func execute(
	ctx context.Context,
	pool *pgxpool.Pool,
	query string,
) (bool, error) {

	var id int64
	err := pool.QueryRow(
		ctx,
		querySQL,
		query,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func percentile(
	values []time.Duration,
	p int,
) time.Duration {

	if len(values) == 0 {
		return 0
	}

	idx := (len(values) - 1) * p / 100

	return values[idx]

}

func printMedianResult(
	results []BenchmarkResult,
) {
	slices.SortFunc(
		results,
		func(a, b BenchmarkResult) int {

			if a.QPS < b.QPS {
				return -1
			}

			if a.QPS > b.QPS {
				return 1
			}

			return 0
		},
	)

	m:=results[len(results)/2]
	fmt.Println("---- median run ----")

	fmt.Printf("workers : %d\n", m.Workers)
	fmt.Printf("queries : %d\n", m.Queries)
	fmt.Printf("hits    : %d\n", m.Hits)
	fmt.Printf("misses  : %d\n", m.Misses)
	fmt.Printf("errors  : %d\n", m.Errors)

	fmt.Printf("avg     : %v\n", m.Avg)
	fmt.Printf("p50     : %v\n", m.P50)
	fmt.Printf("p95     : %v\n", m.P95)
	fmt.Printf("p99     : %v\n", m.P99)
	fmt.Printf("max     : %v\n", m.Max)

	fmt.Printf("wall    : %v\n", m.WallTime)
	fmt.Printf("qps     : %.0f\n", m.QPS)
}

func loadQueries(path string)[]string{
	file,err:=os.Open(path)
	if err!=nil{
		log.Fatal(err)
	}
	defer file.Close()

	rows,err:=csv.NewReader(file).ReadAll()
	if err!=nil{
		log.Fatal(err)
	}
	result:=make([]string,0,len(rows))

	for _,row:=range rows{
		if len(row)==0{
			continue
		}
		result=append(
			result,
			row[0],
		)
	}
	return result
}