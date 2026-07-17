package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

const (
	host = "127.0.0.1"
	port = 9201
)

const batchSize = 100


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

	defer info.Body.Close()



	fmt.Println(
		"Connected to Elasticsearch (Fuzzy Search)",
	)



	queries := loadQueries(
		"data/queries_fuzzy.csv",
	)



	var (
		total time.Duration
		min   = time.Hour
		max   time.Duration
	)



	for batchStart := 0; batchStart < len(queries); batchStart += batchSize {


		batchEnd := batchStart + batchSize


		if batchEnd > len(queries) {
			batchEnd = len(queries)
		}



		var body bytes.Buffer



		for _, q := range queries[batchStart:batchEnd] {


			// msearch header
			body.WriteString("{}\n")



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

			if err != nil {
				log.Fatal(err)
			}



			body.Write(payload)

			body.WriteByte('\n')
		}




		req := esapi.MsearchRequest{

			Index: []string{
				"people",
			},

			Body: bytes.NewReader(
				body.Bytes(),
			),
		}




		start := time.Now()



		resp, err := req.Do(
			context.Background(),
			es,
		)


		if err != nil {
			log.Fatal(err)
		}



		if resp.IsError() {


			data, _ := io.ReadAll(resp.Body)

			resp.Body.Close()


			log.Fatalf(
				"msearch failed: %s",
				string(data),
			)
		}




		var result MultiSearchResponse



		err = json.NewDecoder(resp.Body).
			Decode(&result)



		resp.Body.Close()



		if err != nil {
			log.Fatal(err)
		}





		expected :=
			batchEnd - batchStart



		if len(result.Responses) != expected {

			log.Fatalf(
				"expected %d responses, got %d",
				expected,
				len(result.Responses),
			)
		}




		for j, item := range result.Responses {


			if item.Status >= 400 ||
				len(item.Error) > 0 {


				log.Fatalf(
					"search failed for query %q status=%d error=%s",
					queries[batchStart+j],
					item.Status,
					string(item.Error),
				)
			}




			if len(item.Hits.Hits) == 0 {

				log.Fatalf(
					"Nothing found for query: %s",
					queries[batchStart+j],
				)
			}
		}




		elapsed := time.Since(start)



		total += elapsed



		perQuery :=
			elapsed /
			time.Duration(expected)



		if perQuery < min {
			min = perQuery
		}


		if perQuery > max {
			max = perQuery
		}



		fmt.Printf(
			"%d/%d completed\n",
			batchEnd,
			len(queries),
		)
	}





	qps :=
		float64(len(queries)) /
		total.Seconds()



	avg :=
		total /
		time.Duration(len(queries))





	fmt.Println()

	fmt.Println(
		"-------------- RESULT --------------",
	)


	fmt.Printf(
		"Queries : %d\n",
		len(queries),
	)


	fmt.Printf(
		"Average : %v\n",
		avg,
	)


	fmt.Printf(
		"Minimum : %v\n",
		min,
	)


	fmt.Printf(
		"Maximum : %v\n",
		max,
	)


	fmt.Printf(
		"Total : %v\n",
		total,
	)


	fmt.Printf(
		"QPS : %.2f\n",
		qps,
	)
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



	queries := make(
		[]string,
		0,
		len(rows),
	)



	for _, row := range rows {


		if len(row) == 0 {
			continue
		}


		queries = append(
			queries,
			row[0],
		)
	}



	return queries
}