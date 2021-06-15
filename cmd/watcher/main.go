package main

import (
	"bufio"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	_ "github.com/mattn/go-sqlite3"
)

type DetailOrder int

const (
	Engine DetailOrder = iota
	Year
	Enginesize
	PowerKW
	PowerHP
	Kilometers
)

type Car struct {
	id         int
	name       string
	price      int
	engine     string
	year       int
	month      int
	enginesize int
	powerKW    int
	powerHP    int
	kilometers int
}

type DB struct {
	db *sql.DB
}

func (db *DB) insertCar(car *Car) {
	queryResult, err := db.db.Query(`SELECT * FROM cars WHERE id=?`, car.id)
	if err != nil {
		fmt.Println(err)
		fmt.Println(car)
		fmt.Println("Preparing SELECT statement failed")
	}
	if !queryResult.Next() {
		stmt, err := db.db.Prepare(`INSERT INTO cars VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			fmt.Println("Preparing INSERT statement failed")
		}
		_, err = stmt.Exec(car.id, car.name, car.price, car.engine, car.year, car.month, car.enginesize, car.powerKW, car.powerHP, car.kilometers)
		if err != nil {
			fmt.Println("Failed to INSERT")
			fmt.Println(car)
		}
	} else {
		queryResult.Close()
		_, err := db.db.Exec(`UPDATE cars SET name=?, price=?, engine=?, year=?, month=?, enginesize=?, powerKW=?, powerHP=?, kilometers=? WHERE id=?`,
			car.name,
			car.price,
			car.engine,
			car.year,
			car.month,
			car.enginesize,
			car.powerKW,
			car.powerHP,
			car.kilometers,
			car.id)
		if err != nil {
			fmt.Println("Failed to UPDATE")
			fmt.Print(err)
			fmt.Println(car)
		}
	}
}

func (db *DB) createCarTable() {
	stmt, err := db.db.Prepare(`CREATE TABLE IF NOT EXISTS cars (
		id INTEGER PRIMARY KEY,
		name TEXT,
		price INTEGER,
		engine TEXT,
		year INTEGER,
		month INTEGER,
		enginesize INTEGER,
		powerKW INTEGER,
		powerHP INTEGER,
		kilometers INTEGER)
		`)
	if err != nil {
		log.Fatal(err)
	}
	_, execErr := stmt.Exec()
	if execErr != nil {
		log.Fatal(err)
	}
}

var findDigit = regexp.MustCompile(`[\d]+`)

func getSite(url string) io.ReadCloser {
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Http GET failed")
	}
	return resp.Body
}

func extractNumberFromString(str string) (int, error) {
	stringArr := findDigit.FindAllString(str, -1)
	joinedArr := strings.Join(stringArr, "")
	parsed, err := strconv.Atoi(joinedArr)
	return parsed, err
}

func refreshList(url string, site io.ReadCloser, jobSubmitter chan<- string) {
	doc, err := goquery.NewDocumentFromReader(site)
	if err != nil {
		log.Fatal(err)
	}
	site.Close()
	paginationItem := doc.Find(".pagination")
	numberOfPages := func() int {
		if len(paginationItem.Nodes) == 0 {
			return 1
		}
		lastElemNum := paginationItem.Find(".last").First().Text()
		value, err := strconv.Atoi(lastElemNum)
		if err != nil {
			return 1
		}
		return value
	}()
	jobSubmitter <- url
	if numberOfPages > 1 {
		for i := 2; i <= numberOfPages; i++ {
			job := string(url + fmt.Sprintf("/page%s", strconv.Itoa(i)))
			jobSubmitter <- job
		}
	}
}

func processSite(htmlBody io.ReadCloser, db *DB) {
	doc, err := goquery.NewDocumentFromReader(htmlBody)
	if err != nil {
		log.Fatal(err)
	}
	htmlBody.Close()
	doc.Find(".list-view").Each(func(i int, selection *goquery.Selection) {
		selection.Find(".talalati-sor").Each(func(i int, selection *goquery.Selection) {
			var car Car
			idText := selection.Find(".talalatisor-hirkod").Text()
			id, err := extractNumberFromString(idText)
			if err != nil {
				log.Fatal("Could not parse ID")
				return
			}
			car.id = id

			title := selection.Find("H3").Text()
			car.name = title

			price := selection.Find(".vetelar").Last().Text()
			value, err := extractNumberFromString(price)
			if err != nil {
				return
			}
			car.price = value

			infos := selection.Find(".talalatisor-info").Filter(".adatok")

			var infoString string
			infos.Each(func(i int, s *goquery.Selection) {
				infoString = s.Find("SPAN").Text()
			})
			infoStrings := strings.Split(infoString, ",")
			for idx, str := range infoStrings {
				trimmed := strings.TrimSpace(str)
				switch DetailOrder(idx) {
				case Engine:
					car.engine = trimmed
				case Year:
					dates := strings.Split(trimmed, "/")
					year, err := strconv.Atoi(dates[0])
					if err != nil {
						fmt.Println("Couldn't parse made year")
					}
					car.year = year
					if len(dates) == 2 {
						month, err := strconv.Atoi(dates[1])
						if err != nil {
							fmt.Println("Couldn't parse made month")
						}
						car.month = month
					}
				case Enginesize:
					enginesize, err := extractNumberFromString(trimmed)
					if err != nil {
						fmt.Println("Couldn't parse enginesize")
					}
					car.enginesize = enginesize
				case PowerHP:
					powerHP, err := extractNumberFromString(trimmed)
					if err != nil {
						fmt.Println("Couldn't parse powerHP")
					}
					car.powerHP = powerHP
				case PowerKW:
					powerKW, err := extractNumberFromString(trimmed)
					if err != nil {
						fmt.Println("Couldn't parse powerKW")
					}
					car.powerKW = powerKW
				case Kilometers:
					kilometers, err := extractNumberFromString(trimmed)
					if err != nil {
						fmt.Println("Couldn't parse kilometers")
					}
					car.kilometers = kilometers
				}
			}
			db.insertCar(&car)
		})
	})
}

func worker(jobs <-chan string, db *DB) {
	rateLimiter := time.Tick(time.Millisecond * 200)
	for {
		select {
		case job := <-jobs:
			<-rateLimiter
			site := getSite(job)
			processSite(site, db)
		}
	}
}

func startScraping(period uint, url string, db *DB) chan string {
	stop := make(chan string)
	jobs := make(chan string)
	intervalTicker := time.NewTicker(time.Minute * time.Duration(period))

	go worker(jobs, db)
	site := getSite(url)
	refreshList(url, site, jobs)

	go func() {
		for {
			select {
			case <-intervalTicker.C:
				site := getSite(url)
				refreshList(url, site, jobs)
			case stop := <-stop:
				fmt.Println("Stop" + stop)
				return
			}
		}
	}()

	return stop
}

func dumpTable(rows *sql.Rows, out io.Writer) error {
	colNames, err := rows.Columns()
	if err != nil {
		panic(err)
	}
	writer := csv.NewWriter(out)
	writer.Comma = '\t'
	readCols := make([]interface{}, len(colNames))
	writeCols := make([]string, len(colNames))
	for i, _ := range writeCols {
		readCols[i] = &writeCols[i]
	}
	for rows.Next() {
		err := rows.Scan(readCols...)
		if err != nil {
			panic(err)
		}
		writer.Write(writeCols)
	}
	if err = rows.Err(); err != nil {
		panic(err)
	}
	writer.Flush()
	return nil
}

func main() {
	var timervalue int = 0
	var url string = ""
	var worker chan string
	reader := bufio.NewReader(os.Stdin)

	//Crate the Database, I'm going for in memory here
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		log.Fatal(err)
	}

	dbContext := &DB{db: db}
	dbContext.createCarTable()

	fmt.Println("Site watcher")
	fmt.Println("---------------------")
	fmt.Println("Add a time interval and a url to watch")

	for {
		fmt.Print("-> ")
		text, _ := reader.ReadString('\n')
		cmd := strings.Fields(text)
		if len(cmd) > 0 {
			switch cmd[0] {
			case "start":
				worker = startScraping(uint(timervalue), url, dbContext)
			case "interval":
				value, err := strconv.Atoi(cmd[1])
				if err != nil {
					return
				}
				timervalue = value
			case "url":
				url = cmd[1]
			case "query":
				queryStr := strings.SplitN(text, " ", 2)
				rows, err := db.Query(queryStr[1])
				if err != nil {
					log.Fatal(err)
				}
				dumpTable(rows, os.Stdout)
			case "stop":
				worker <- "stop message"
			}
		}
	}
}
