package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const indexName = "people"

var elasticURL string

type Person struct {
	ID         int    `json:"id"`
	LastName   string `json:"last_name"`
	FirstName  string `json:"first_name"`
	MiddleName string `json:"middle_name"`
	FullName   string `json:"full_name"`
}

func main() {
	port := flag.String("port", "9202", "Elasticsearch port")
	recreate := flag.Bool("recreate", true, "delete and recreate the index before loading data")
	flag.Parse()

	elasticURL = "http://127.0.0.1:" + *port
	fmt.Println("Start Elasticsearch loader")
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
	req, err := http.NewRequest(http.MethodDelete, elasticURL+"/"+indexName, nil)
	if err != nil {
		log.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		failResponse("delete index", resp)
	}
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
          "filter": ["lowercase"]
        }
      }
    }
  },
  "mappings": {
    "properties": {
      "id": { "type": "integer" },
      "last_name": { "type": "text", "analyzer": "name_analyzer", "search_analyzer": "name_analyzer" },
      "first_name": { "type": "text", "analyzer": "name_analyzer", "search_analyzer": "name_analyzer" },
      "middle_name": { "type": "text", "analyzer": "name_analyzer", "search_analyzer": "name_analyzer" },
      "full_name": { "type": "text", "analyzer": "name_analyzer", "search_analyzer": "name_analyzer" }
    }
  }
}`
	req, err := http.NewRequest(http.MethodPut, elasticURL+"/"+indexName, bytes.NewBufferString(body))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		failResponse("create index", resp)
	}
	fmt.Println("Index created")
}

func loadCSV() {
	file, err := os.Open("data/people.csv")
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
		person := Person{ID: atoi(row[0]), LastName: row[1], FirstName: row[2], MiddleName: row[3], FullName: row[1] + " " + row[2] + " " + row[3]}
		fmt.Fprintf(&buffer, `{"index":{"_index":"%s","_id":"%d"}}`+"\n", indexName, person.ID)
		doc, err := json.Marshal(person)
		if err != nil {
			log.Fatal(err)
		}
		buffer.Write(doc)
		buffer.WriteByte('\n')
		count++
		if count%5000 == 0 {
			sendBulk(&buffer)
			fmt.Printf("Loaded %d documents\n", count)
		}
	}
	if buffer.Len() > 0 {
		sendBulk(&buffer)
	}
}

func sendBulk(buffer *bytes.Buffer) {
	req, err := http.NewRequest(http.MethodPost, elasticURL+"/_bulk", buffer)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		failResponse("bulk index", resp)
	}
	var result struct {
		Errors bool `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Fatal(err)
	}
	if result.Errors {
		log.Fatal("bulk index returned item errors")
	}
	buffer.Reset()
}

func setRefreshInterval(value string) {
	body := fmt.Sprintf(`{"index":{"refresh_interval":"%s"}}`, value)
	req, err := http.NewRequest(http.MethodPut, elasticURL+"/"+indexName+"/_settings", bytes.NewBufferString(body))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		failResponse("update index settings", resp)
	}
}

func refreshIndex() {
	req, err := http.NewRequest(http.MethodPost, elasticURL+"/"+indexName+"/_refresh", nil)
	if err != nil {
		log.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		failResponse("refresh index", resp)
	}
}

func failResponse(action string, resp *http.Response) {
	body, _ := io.ReadAll(resp.Body)
	log.Fatalf("%s failed: %s: %s", action, resp.Status, string(body))
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func init() {
	http.DefaultClient.Timeout = 30 * time.Second
}