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

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

const (
	host = "127.0.0.1"
	port = 9201
	batchSize = 100
	warmupCount  = 100
	runsPerLevel = 3
)

var workerConfigs = []int{
	1,
	4,
	16,
	64,
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


type MultiSearchResponse struct {
	Responses []struct {
		Hits struct {
			Hits []struct {
				ID string `json:"_id"`
			} `json:"hits"`
		} `json:"hits"`

		Error json.RawMessage `json:"error,omitempty"`
		Status int `json:"status"`
	} `json:"responses"`
}

func main() {
	es, err := elasticsearch.NewClient(
		elasticsearch.Config{
			Addresses: []string{
				fmt.Sprintf(
					"http://%s:%d",
					host,
					port,
				),
			},
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	info, err := es.Info()
	if err != nil {
		log.Fatal(err)
	}
	info.Body.Close()

	fmt.Println("Connected to Elasticsearch")
	queries := loadQueries(
		"data/queries.csv",
	)
	fmt.Println("warmup...")
	for i := 0; i < warmupCount && i < len(queries); i++ {
		_, _ = executeBatch(
			es,
			queries[i:i+1],
		)
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
				es,
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
	es *elasticsearch.Client,
	queries []string,
	workers int,
) BenchmarkResult {
	queryCh := make(
		chan []string,
	)
	var wg sync.WaitGroup
	var (
		totalHits atomic.Uint64
		totalMiss atomic.Uint64
		totalErr atomic.Uint64
	)
	var (
		mu sync.Mutex
		latencies []time.Duration
	)
	startWall := time.Now()
	for i:=0;i<workers;i++ {
		wg.Add(1)
		go func(){
			defer wg.Done()
			var local []time.Duration
			for batch := range queryCh {
				start := time.Now()
				hits, err := executeBatch(
					es,
					batch,
				)
				elapsed := time.Since(start)
				if err != nil {
					totalErr.Add(
						uint64(len(batch)),
					)
					continue
				}
				local = append(
					local,
					elapsed / time.Duration(len(batch)),
				)
				if hits {

					totalHits.Add(
						uint64(len(batch)),
					)
				} else {
					totalMiss.Add(
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
	for i:=0;i<len(queries);i+=batchSize {
		end:=i+batchSize
		if end>len(queries){
			end=len(queries)
		}
		queryCh <- queries[i:end]
	}
	close(queryCh)
	wg.Wait()
	wall := time.Since(startWall)
	slices.Sort(latencies)
	var total time.Duration
	for _,v:=range latencies{
		total+=v
	}
	result:=BenchmarkResult{
		Workers: workers,

		Queries:uint64(len(queries)),

		Hits:totalHits.Load(),
		Misses:totalMiss.Load(),
		Errors:totalErr.Load(),

		WallTime:wall,

		QPS:float64(len(queries))/wall.Seconds(),
	}

	if len(latencies)>0 {
		result.Avg =
			total/time.Duration(len(latencies))
		result.P50 =
			percentile(latencies,50)
		result.P95 =
			percentile(latencies,95)
		result.P99 =
			percentile(latencies,99)
		result.Max =
			latencies[len(latencies)-1]
	}
	return result
}

func executeBatch(
	es *elasticsearch.Client,
	queries []string,
)(bool,error){
	var body bytes.Buffer
	for _,q:=range queries{
		body.WriteString("{}\n")
		query:=map[string]interface{}{
			"size":1,
			"_source":false,
			"track_total_hits":false,
			"query":map[string]interface{}{
				"match":map[string]interface{}{
					"full_name":map[string]interface{}{
						"query":q,
						"operator": "and",
						"fuzziness": 1,
						"prefix_length": 0,
						"max_expansions": 5,
					},
				},
			},
		}
		data,_:=json.Marshal(query)
		body.Write(data)
		body.WriteByte('\n')
	}

	req:=esapi.MsearchRequest{
		Index:[]string{"people"},
		Body:&body,
	}
	resp,err:=req.Do(
		context.Background(),
		es,
	)

	if err!=nil{
		return false,err
	}

	defer resp.Body.Close()
	if resp.IsError(){
		return false,
			fmt.Errorf(
				"msearch error: %s",
				resp.Status(),
			)
	}

	var result MultiSearchResponse
	err=json.NewDecoder(resp.Body).
		Decode(&result)
	if err!=nil{
		return false,err
	}

	for _,item:=range result.Responses{
		if item.Status>=400 ||
			len(item.Error)>0{
			return false,
				fmt.Errorf(
					"search error",
				)
		}
		if len(item.Hits.Hits)==0{
			return false,nil
		}
	}
	return true,nil
}

func percentile(
	values []time.Duration,
	p int,
)time.Duration{
	if len(values)==0{
		return 0
	}
	idx:=(len(values)-1)*p/100
	return values[idx]
}

func printMedianResult(
	results []BenchmarkResult,
){
	slices.SortFunc(
		results,
		func(a,b BenchmarkResult)int{
			if a.QPS<b.QPS{
				return -1
			}
			if a.QPS>b.QPS{
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