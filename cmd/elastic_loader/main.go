package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"flag"
)

const (
	indexName  = "people"
)
var elasticURL string

type Person struct {
	ID         int    `json:"id"`
	LastName   string `json:"last_name"`
	FirstName  string `json:"first_name"`
	MiddleName string `json:"middle_name"`
	FullName   string `json:"full_name"`
}

func main() {
	port := flag.String("port", "9200", "Elasticsearch port")
	flag.Parse()
	elasticURL = "http://127.0.0.1:" + *port
	fmt.Println("Start Elasticsearch loader")
	createIndex()
	loadCSV()
	fmt.Println("Loading completed")
}


func createIndex() {
	body := `
	{
	  "mappings": {
	    "properties": {
	      "id": {
	        "type": "integer"
	      },
	      "last_name": {
	        "type": "text"
	      },
	      "first_name": {
	        "type": "text"
	      },
	      "middle_name": {
	        "type": "text"
	      },
	      "full_name": {
	        "type": "text"
	      }
	    }
	  }
	}
	`
	req, err := http.NewRequest(
		"PUT",
		elasticURL+"/"+indexName,
		bytes.NewBuffer([]byte(body)),
	)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		fmt.Println("Index already exists or error")
	} else {
		fmt.Println("Index created")
	}
}


func loadCSV() {
	file, err := os.Open("data/people.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		log.Fatal(err)
	}
	var buffer bytes.Buffer
	count := 0
	for i := 1; i < len(rows); i++ {
		person := Person{
			ID: atoi(rows[i][0]),
			LastName: rows[i][1],
			FirstName: rows[i][2],
			MiddleName: rows[i][3],
			FullName:
				rows[i][1] + " " +
				rows[i][2] + " " +
				rows[i][3],
		}
		meta := []byte(
			fmt.Sprintf(
				`{"index":{"_index":"%s","_id":"%d"}}`,
				indexName,
				person.ID,
			),
		)
		doc, err := json.Marshal(person)
		if err != nil {
			log.Fatal(err)
		}
		buffer.Write(meta)
		buffer.WriteByte('\n')

		buffer.Write(doc)
		buffer.WriteByte('\n')

		count++
		// отправляем пачками по 5000 документов
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
	req, err := http.NewRequest(
		"POST",
		elasticURL+"/_bulk",
		buffer,
	)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set(
		"Content-Type",
		"application/x-ndjson",
	)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fmt.Println(
			"Bulk error:",
			resp.Status,
		)
	}
	buffer.Reset()
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}