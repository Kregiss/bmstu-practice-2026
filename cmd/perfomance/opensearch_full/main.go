package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

const (
	host         = "127.0.0.1"
	port         = 9202
	indexName    = "people"
	batchSize    = 100
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

	ctx := context.Background()

	fmt.Println("Connected to OpenSearch")

	queries := loadQueries("data/queries.csv")

	fmt.Println("warmup...")

	for i := 0; i < warmupCount && i < len(queries); i++ {
		_, _ = executeBatch(
			client,
			ctx,
			queries[i:i+1],
		)
	}

	fmt.Println()

	for _, workers := range workerConfigs {

		fmt.Printf(
			"========== %d workers ==========\n",
			workers,
		)

		var runs []BenchmarkResult

		for run := 1; run <= runsPerLevel; run++ {

			result := runBenchmark(
				client,
				ctx,
				queries,
				workers,
			)

			runs = append(
				runs,
				result,
			)

			fmt.Printf(
				"run=%d workers=%d qps=%.0f avg=%v p50=%v p95=%v p99=%v errors=%d\n",
				run,
				result.Workers,
				result.QPS,
				result.Avg,
				result.P50,
				result.P95,
				result.P99,
				result.Errors,
			)
		}

		printMedianResult(runs)

		fmt.Println()
	}
}

func runBenchmark(
	client *opensearchapi.Client,
	ctx context.Context,
	queries []string,
	workers int,
) BenchmarkResult {

	queryCh := make(chan []string)

	var wg sync.WaitGroup

	var (
		hits   atomic.Uint64
		misses atomic.Uint64
		errs   atomic.Uint64
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
				len(queries)/workers+1,
			)

			for batch := range queryCh {

				start := time.Now()

				found, err := executeBatch(
					client,
					ctx,
					batch,
				)

				elapsed := time.Since(start)

				if err != nil {
					errs.Add(
						uint64(len(batch)),
					)
					continue
				}

				local = append(
					local,
					elapsed/time.Duration(len(batch)),
				)

				if found {
					hits.Add(
						uint64(len(batch)),
					)
				} else {
					misses.Add(
						uint64(len(batch)),
					)
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

	for i := 0; i < len(queries); i += batchSize {

		end := i + batchSize

		if end > len(queries) {
			end = len(queries)
		}

		queryCh <- queries[i:end]
	}

	close(queryCh)

	wg.Wait()

	wall := time.Since(startWall)

	result := BenchmarkResult{
		Workers: workers,

		Queries: uint64(len(queries)),

		Hits:   hits.Load(),
		Misses: misses.Load(),
		Errors: errs.Load(),

		WallTime: wall,

		QPS: float64(len(queries)) / wall.Seconds(),
	}

	if len(latencies) == 0 {
		return result
	}

	slices.Sort(latencies)

	var total time.Duration

	for _, v := range latencies {
		total += v
	}

	result.Avg =
		total / time.Duration(len(latencies))

	result.P50 =
		percentile(latencies, 50)

	result.P95 =
		percentile(latencies, 95)

	result.P99 =
		percentile(latencies, 99)

	result.Max =
		latencies[len(latencies)-1]

	return result
}

func executeBatch(
	client *opensearchapi.Client,
	ctx context.Context,
	queries []string,
) (bool, error) {

	var body bytes.Buffer

	for _, q := range queries {

		body.WriteString("{}\n")

		query := map[string]interface{}{
			"size":              1,
			"_source":           false,
			"track_total_hits":  false,
			"query": map[string]interface{}{
				"match": map[string]interface{}{
					"full_name": map[string]interface{}{
						"query":     q,
						"operator":  "and",
						"auto_generate_synonyms_phrase_query": false,
					},
				},
			},
		}

		data, err := json.Marshal(query)
		if err != nil {
			return false, err
		}

		body.Write(data)
		body.WriteByte('\n')
	}

	resp, err := client.MSearch(
		ctx,
		opensearchapi.MSearchReq{
			Indices: []string{
				indexName,
			},
			Body: bytes.NewReader(body.Bytes()),
		},
	)

	if err != nil {
		return false, err
	}

	if len(resp.Responses) != len(queries) {
		return false, fmt.Errorf("unexpected response count")
	}

	for _, item := range resp.Responses {

		if item.Error != nil {
			return false, fmt.Errorf("%v", item.Error)
		}

		if len(item.Hits.Hits) == 0 {
			return false, nil
		}
	}

	return true, nil
}
func printMedianResult(
	results []BenchmarkResult,
) {
	if len(results) == 0 {
		return
	}

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