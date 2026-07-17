package main

import (
	"context"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"
	"net/http"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

const indexName = "people"

type Person struct {
	ID         int    `json:"id"`
	LastName   string `json:"last_name"`
	FirstName  string `json:"first_name"`
	MiddleName string `json:"middle_name"`
	FullName   string `json:"full_name"`
}

var client *elasticsearch.Client


func main() {
	port := flag.String(
		"port",
		"9201",
		"Elasticsearch port",
	)

	recreate := flag.Bool(
		"recreate",
		true,
		"delete and recreate index",
	)
	flag.Parse()
	var err error
	client, err = elasticsearch.NewClient(
		elasticsearch.Config{
			Addresses: []string{
				fmt.Sprintf(
					"http://127.0.0.1:%s",
					*port,
				),
			},
		},
	)

	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.Info()
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	fmt.Println("Connected to Elasticsearch")

	if *recreate {
		deleteIndex()
	}

	createIndex()
	loadCSV()
	setRefreshInterval("1s")
	refreshIndex()
	fmt.Println("Loading completed")
}

func deleteIndex() {
	req := esapi.IndicesDeleteRequest{
		Index: []string{indexName},
	}

	resp, err := req.Do(
		context.Background(),
		client,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return
	}

	if resp.IsError() {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf(
			"delete index failed: %s",
			string(body),
		)
	}
	fmt.Println("Index deleted")
}

func createIndex() {
	body := `
{
  "settings": {
    "refresh_interval": "-1",
    "number_of_replicas": 0,
    "analysis": {
      "analyzer": {
        "name_analyzer": {
          "type": "custom",
          "tokenizer": "standard",
          "filter": [
            "lowercase"
          ]
        }
      }
    }
  },
  "mappings": {
    "properties": {
      "id": {
        "type": "integer"
      },
      "last_name": {
        "type": "text",
        "analyzer": "name_analyzer",
        "search_analyzer": "name_analyzer"
      },
      "first_name": {
        "type": "text",
        "analyzer": "name_analyzer",
        "search_analyzer": "name_analyzer"
      },
      "middle_name": {
        "type": "text",
        "analyzer": "name_analyzer",
        "search_analyzer": "name_analyzer"
      },
      "full_name": {
        "type": "text",
        "analyzer": "name_analyzer",
        "search_analyzer": "name_analyzer"
      }
    }
  }
}
`
	req := esapi.IndicesCreateRequest{
		Index: indexName,
		Body: bytes.NewReader(
			[]byte(body),
		),
	}
	resp, err := req.Do(
		context.Background(),
		client,
	)

	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf(
			"create index failed: %s",
			string(body),
		)
	}
	fmt.Println("Index created")
}

func loadCSV() {
	file, err := os.Open(
		"data/people.csv",
	)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	var buffer bytes.Buffer
	count := 0
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		person := Person{
			ID: atoi(row[0]),
			LastName: row[1],
			FirstName: row[2],
			MiddleName: row[3],
			FullName:
				row[1]+" "+
				row[2]+" "+
				row[3],
		}
		meta := fmt.Sprintf(
			`{"index":{"_index":"%s","_id":"%d"}}`,
			indexName,
			person.ID,
		)
		buffer.WriteString(meta)
		buffer.WriteByte('\n')

		doc, err := json.Marshal(person)
		if err != nil {
			log.Fatal(err)
		}
		buffer.Write(doc)
		buffer.WriteByte('\n')

		count++
		if count%5000 == 0 {
			sendBulk(&buffer)
			fmt.Printf(
				"Loaded %d documents\n",
				count,
			)
		}
	}
	if buffer.Len() > 0 {
		sendBulk(&buffer)
	}
}

func sendBulk(buffer *bytes.Buffer) {
	req := esapi.BulkRequest{
		Body: bytes.NewReader(
			buffer.Bytes(),
		),
	}
	resp, err := req.Do(
		context.Background(),
		client,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		body,_ := io.ReadAll(resp.Body)
		log.Fatalf(
			"bulk failed: %s",
			string(body),
		)
	}

	var result struct {
		Errors bool `json:"errors"`
	}

	err = json.NewDecoder(resp.Body).
		Decode(&result)

	if err != nil {
		log.Fatal(err)
	}

	if result.Errors {
		log.Fatal(
			"bulk contains errors",
		)
	}
	buffer.Reset()
}

func setRefreshInterval(value string) {
	body := fmt.Sprintf(
		`
{
 "index":{
   "refresh_interval":"%s"
 }
}
`,
		value,
	)

	req := esapi.IndicesPutSettingsRequest{
		Index: []string{indexName},
		Body: bytes.NewReader(
			[]byte(body),
		),
	}

	resp, err := req.Do(
		context.Background(),
		client,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()


	if resp.IsError() {
		body,_ := io.ReadAll(resp.Body)
		log.Fatalf(
			"settings failed: %s",
			string(body),
		)
	}
}

func refreshIndex() {
	req := esapi.IndicesRefreshRequest{
		Index: []string{indexName},
	}

	resp, err := req.Do(
		context.Background(),
		client,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.IsError() {
		log.Fatal(
			"refresh failed",
		)
	}
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func init() {
	httpClient := &http.Client{
		Timeout: 30*time.Second,
	}
	_ = httpClient
}