package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

const indexName = "people"

var client *opensearchapi.Client

type Person struct {
	ID         int    `json:"id"`
	LastName   string `json:"last_name"`
	FirstName  string `json:"first_name"`
	MiddleName string `json:"middle_name"`
	FullName   string `json:"full_name"`
}


func main() {
	port := flag.String(
		"port",
		"9202",
		"OpenSearch port",
	)
	recreate := flag.Bool(
		"recreate",
		true,
		"recreate index",
	)
	flag.Parse()
	var err error

	client, err = opensearchapi.NewClient(
		opensearchapi.Config{
			Client: opensearch.Config{
				Addresses: []string{
					fmt.Sprintf(
						"http://127.0.0.1:%s",
						*port,
					),
				},
			},
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Connected to OpenSearch")
	if *recreate {
		deleteIndex()
	}
	createIndex()
	loadCSV()
	refreshIndex()
	fmt.Println("Loading completed")
}

func deleteIndex() {
	_, err := client.Indices.Delete(
		context.Background(),
		opensearchapi.IndicesDeleteReq{
			Indices: []string{
				indexName,
			},
		},
	)
	if err != nil {
		if strings.Contains(
			err.Error(),
			"index_not_found_exception",
		) {
			fmt.Println("Index does not exist")
			return
		}
		log.Fatal(err)
	}
	fmt.Println("Index deleted")
}

func createIndex() {
	body := `
{
 "settings":{
   "number_of_replicas":0,
   "analysis":{
     "analyzer":{
       "name_analyzer":{
         "type":"custom",
         "tokenizer":"standard",
         "filter":["lowercase"]
       }
     }
   }
 },
 "mappings":{
   "properties":{
     "id":{
       "type":"integer"
     },
     "last_name":{
       "type":"text",
       "analyzer":"name_analyzer"
     },
     "first_name":{
       "type":"text",
       "analyzer":"name_analyzer"
     },
     "middle_name":{
       "type":"text",
       "analyzer":"name_analyzer"
     },
     "full_name":{
       "type":"text",
       "analyzer":"name_analyzer"
     }
   }
 }
}
`
	_, err := client.Indices.Create(
		context.Background(),
		opensearchapi.IndicesCreateReq{
			Index:indexName,
			Body:bytes.NewReader(
				[]byte(body),
			),
		},
	)
	if err != nil {
		log.Fatal(err)
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

		buffer.WriteString(
			fmt.Sprintf(
				`{"index":{"_index":"%s","_id":"%d"}}`,
				indexName,
				person.ID,
			),
		)
		buffer.WriteByte('\n')
		doc,_ := json.Marshal(person)
		buffer.Write(doc)
		buffer.WriteByte('\n')

		count++
		if count%5000==0 {
			sendBulk(&buffer)
			fmt.Printf(
				"Loaded %d documents\n",
				count,
			)
		}
	}

	if buffer.Len()>0 {
		sendBulk(&buffer)
	}
}

func sendBulk(
	buffer *bytes.Buffer,
) {
	resp, err := client.Bulk(
		context.Background(),
		opensearchapi.BulkReq{
			Body:bytes.NewReader(
				buffer.Bytes(),
			),
		},
	)

	if err != nil {
		log.Fatal(err)
	}
	if resp.Errors {

		log.Fatal("bulk errors")
	}
	buffer.Reset()
}

func refreshIndex() {
	_, err := client.Indices.Refresh(
		context.Background(),
		&opensearchapi.IndicesRefreshReq{
			Index: []string{
				indexName,
			},
		},
	)
	if err != nil {
		log.Fatal(err)
	}
}

func atoi(s string) int {
	n:=0
	for _,c:=range s {
		n = n * 10 + int(c-'0')
	}
	return n
}
