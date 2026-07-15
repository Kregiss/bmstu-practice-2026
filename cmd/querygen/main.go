package main

import (
	"encoding/csv"
	"flag"
	"log"
	"math/rand"
	"os"
	"time"
)

type Person struct {
	LastName   string
	FirstName  string
	MiddleName string
}

func main() {
	fuzzy := flag.Bool("fuzzy", false, "Generate fuzzy queries")
	flag.Parse()

	people := loadPeople()
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	rnd.Shuffle(len(people), func(i, j int) {
		people[i], people[j] = people[j], people[i]
	})
	
	filename := "data/queries.csv"
	if *fuzzy {
		filename = "data/queries_fuzzy.csv"
	}

	file, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()
	count_query := 0
	for _, person := range people {
		last := person.LastName
		first := person.FirstName
		middle := person.MiddleName
		if *fuzzy {
			switch rnd.Intn(3) {
				case 0:
					last = mutate(last, rnd)
				case 1:
					first = mutate(first, rnd)
				case 2:
					middle = mutate(middle, rnd)
				}
		}
		query := last + " " + first + " " + middle
		count_query += 1
		if (count_query == 1001) {break}
		writer.Write([]string{query})
	}
	log.Println("Query generated")
}

func loadPeople() []Person {
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
	var people []Person
	for i := 0; i < len(rows); i++ {
		people = append(people, Person{
			LastName:   rows[i][1],
			FirstName:  rows[i][2],
			MiddleName: rows[i][3],
		})
	}
	return people
}

func mutate(word string, rnd *rand.Rand) string {
	letters := []rune("ёйцукенгшщзхъэждлорпавыфячсмитьбю")
	r := []rune(word)
	switch rnd.Intn(4) {
	// удаление
	case 0:
		pos := rnd.Intn(len(r))
		r = append(r[:pos], r[pos+1:]...)
	// вставка
	case 1:
		pos := rnd.Intn(len(r) + 1)
		ch := letters[rnd.Intn(len(letters))]
		r = append(r[:pos], append([]rune{ch}, r[pos:]...)...)
	// замена
	case 2:
		pos := rnd.Intn(len(r))
		r[pos] = letters[rnd.Intn(len(letters))]
	// перестановка
	case 3:
		pos := rnd.Intn(len(r) - 1)
		r[pos], r[pos+1] = r[pos+1], r[pos]
	}
	return string(r)
}