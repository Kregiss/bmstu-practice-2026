package main

import (
	"bufio"
	"encoding/csv"
	"log"
	"os"
	"strconv"
)

func main() {
	lastNames := readLines("cmd/generator/lastnames.txt")
	firstNames := readLines("cmd/generator/firstnames.txt")
	middleNames := readLines("cmd/generator/middlenames.txt")
	file, err := os.Create("data/people.csv")

	if err != nil {
		log.Fatal(err)
	}

	defer file.Close()
	writer := csv.NewWriter(file)
	defer writer.Flush()
	id := 1
	for _, last := range lastNames {
		for _, first := range firstNames {
			for _, middle := range middleNames {
				writer.Write([]string{
					intToString(id),
					last,
					first,
					middle,
				})
				id++
			}
		}
	}
	log.Printf("Generate %d people\n", id-1)
}

func readLines(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

func intToString(v int) string {
	return strconv.Itoa(v)
}