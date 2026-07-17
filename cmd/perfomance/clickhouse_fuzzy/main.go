package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

const (
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
	QPS      float64
}

func main() {
	ctx := context.Background()

	queries := loadQueries("data/queries_fuzzy.csv")

	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(queries), func(i, j int) {
		queries[i], queries[j] = queries[j], queries[i]
	})

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{"127.0.0.1:9001"},
		Auth: clickhouse.Auth{
			Database: "default",
			Username: "test-user",
			Password: "test-password",
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := conn.Ping(ctx); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected to ClickHouse (Fuzzy Search)")
	fmt.Println("warmup...")

	for i := 0; i < min(warmupCount, len(queries)); i++ {
		_, _ = execute(ctx, conn, queries[i])
	}

	fmt.Println()

	for _, workers := range workerConfigs {
		fmt.Printf("========== %d workers ==========\n", workers)

		var runs []BenchmarkResult

		for run := 1; run <= runsPerLevel; run++ {
			result := runBenchmark(
				ctx, 
				conn, 
				queries, 
				workers,
			)

			runs = append(runs, result)

			fmt.Printf(
				"run=%d workers=%d qps=%.0f p50=%v p95=%v p99=%v hits=%d miss=%d err=%d\n",
				run,
				result.Workers,
				result.QPS,
				result.P50,
				result.P95,
				result.P99,
				result.Hits,
				result.Misses,
				result.Errors,
			)
		}

		printMedianResult(runs)
		fmt.Println()
	}
}

func runBenchmark(
	ctx context.Context,
	conn clickhouse.Conn,
	queries []string,
	workers int,
) BenchmarkResult {

	var (
		hits   atomic.Uint64
		misses atomic.Uint64
		errs   atomic.Uint64
	)

	queryCh := make(chan string, len(queries))

	var (
		wg sync.WaitGroup

		mu        sync.Mutex
		latencies []time.Duration
	)

	startWall := time.Now()

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			localLatencies := make([]time.Duration, 0, len(queries)/workers+1)
			for q := range queryCh {
				start := time.Now()
				hit, err := execute(ctx, conn, q)
				elapsed := time.Since(start)
				if err != nil {
					errs.Add(1)
					continue
				}
				localLatencies = append(localLatencies, elapsed)
				if hit {
					hits.Add(1)
				} else {
					misses.Add(1)
				}
			}
			mu.Lock()
			latencies = append(latencies, localLatencies...)
			mu.Unlock()
		}()
	}

	for _, q := range queries {
		queryCh <- q
	}

	close(queryCh)
	wg.Wait()
	wallTime := time.Since(startWall)
	if len(latencies) == 0 {
		return BenchmarkResult{
			Workers:  workers,
			Queries:  uint64(len(queries)),
			Errors:   errs.Load(),
			WallTime: wallTime,
		}
	}

	slices.Sort(latencies)
	var totalLatency time.Duration
	for _, l := range latencies {
		totalLatency += l
	}

	return BenchmarkResult{
		Workers: workers,

		Queries: uint64(len(queries)),

		Hits:   hits.Load(),
		Misses: misses.Load(),
		Errors: errs.Load(),

		Avg: totalLatency / time.Duration(len(latencies)),
		P50: percentile(latencies, 50),
		P95: percentile(latencies, 95),
		P99: percentile(latencies, 99),
		Max: latencies[len(latencies)-1],

		WallTime: wallTime,

		QPS: float64(len(queries)) / wallTime.Seconds(),
	}
}

func execute(
	ctx context.Context,
	conn clickhouse.Conn,
	q string,
) (bool, error) {

	var id uint64

	err := conn.QueryRow(
		ctx,
		`
		SELECT id
		FROM people
		WHERE ngramDistanceUTF8(full_name, ?) <= 0.45
		ORDER BY ngramDistanceUTF8(full_name, ?)
		LIMIT 1
		`,
		q,
		q,
	).Scan(&id)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func printMedianResult(results []BenchmarkResult) {
	if len(results) == 0 {
		return
	}

	slices.SortFunc(results, func(a, b BenchmarkResult) int {
		switch {
		case a.QPS < b.QPS:
			return -1
		case a.QPS > b.QPS:
			return 1
		default:
			return 0
		}
	})

	mid := results[len(results)/2]

	fmt.Println("---- median run ----")

	fmt.Printf("workers : %d\n", mid.Workers)
	fmt.Printf("queries : %d\n", mid.Queries)
	fmt.Printf("hits    : %d\n", mid.Hits)
	fmt.Printf("misses  : %d\n", mid.Misses)
	fmt.Printf("errors  : %d\n", mid.Errors)

	fmt.Printf("avg     : %v\n", mid.Avg)
	fmt.Printf("p50     : %v\n", mid.P50)
	fmt.Printf("p95     : %v\n", mid.P95)
	fmt.Printf("p99     : %v\n", mid.P99)
	fmt.Printf("max     : %v\n", mid.Max)

	fmt.Printf("wall    : %v\n", mid.WallTime)
	fmt.Printf("qps     : %.0f\n", mid.QPS)
}

func percentile(values []time.Duration, p int) time.Duration {
	if len(values) == 0 {
		return 0
	}

	idx := (len(values) - 1) * p / 100
	return values[idx]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func loadQueries(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		log.Fatal(err)
	}

	result := make([]string, 0, len(rows))

	for _, row := range rows {
		if len(row) == 0 {
			continue
		}

		result = append(result, row[0])
	}

	return result
}