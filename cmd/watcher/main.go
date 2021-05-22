package main

import (
	"bufio"
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

func parseItemList(doc *goquery.Document) {
	doc.Find(".list-view").Each(func(i int, selection *goquery.Selection) {
		selection.Find(".talalati-sor").Each(func(i int, selection *goquery.Selection) {
			title := selection.Find("H3").Text()
			price := selection.Find(".vetelar").Last().Text()
			infos := selection.Find(".talalatisor-info").Filter(".adatok")
			var infoStrings []string
			infos.Each(func(i int, s *goquery.Selection) {
				stuff := s.Find("SPAN").Text()
				infoStrings = append(infoStrings, stuff)
			})
			var car Car
			car.name = title
			value, err := extractNumberFromString(price)
			if err != nil {
				fmt.Println("Couldn't parse price")
			}
			car.price = value
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
						car.year = month
					}
				case Enginesize:
					value, err := extractNumberFromString(trimmed)
					if err != nil {
						fmt.Println("Couldn't parse enginesize")
					}
					car.enginesize = value
				case PowerHP:
					value, err := extractNumberFromString(trimmed)
					if err != nil {
						fmt.Println("Couldn't parse powerHP")
					}
					car.powerHP = value
				case PowerKW:
					value, err := extractNumberFromString(trimmed)
					if err != nil {
						fmt.Println("Couldn't parse powerKW")
					}
					car.powerKW = value
				case Kilometers:
					value, err := extractNumberFromString(trimmed)
					if err != nil {
						fmt.Println("Couldn't parse kilometers")
					}
					car.kilometers = value
				}
			}
			fmt.Println(title)
			fmt.Println(price, infoStrings)
		})
	})
}

func refreshList(url string, jobSubmitter chan<- string) {
	htmlBody := getSite(url)
	doc, err := goquery.NewDocumentFromReader(htmlBody)
	if err != nil {
		log.Fatal(err)
	}
	htmlBody.Close()
	paginationItem := doc.Find(".pagination")
	numberOfPages := func() int {
		if len(paginationItem.Nodes) == 0 {
			return 1
		}
		lastElemNum := paginationItem.Find(".last").First().Text()
		fmt.Println(lastElemNum)
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

func processSite(htmlBody io.ReadCloser) {
	doc, err := goquery.NewDocumentFromReader(htmlBody)
	if err != nil {
		log.Fatal(err)
	}
	htmlBody.Close()
	parseItemList(doc)
}

func worker(jobs <-chan string) {
	for {
		select {
		case job := <-jobs:
			processSite(getSite(job))
		}
	}
}

func startScraping(period uint, url string) chan string {
	stop := make(chan string)
	jobs := make(chan string)
	intervalTicker := time.NewTicker(time.Minute * time.Duration(period))

	go worker(jobs)
	refreshList(url, jobs)

	go func() {
		for {
			select {
			case <-intervalTicker.C:
				refreshList(url, jobs)
			case stop := <-stop:
				fmt.Println("Stop" + stop)
				return
			}
		}
	}()

	return stop
}

func main() {
	var timervalue int = 0
	var url string = ""
	var worker chan string
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Site watcher")
	fmt.Println("---------------------")
	fmt.Println("Add a time interval and a url to watch")

	for {
		fmt.Print("-> ")
		text, _ := reader.ReadString('\n')
		cmd := strings.Fields(text)
		if len(cmd) > 0 {
			switch cmd[0] {
			case "run":
				worker = startScraping(uint(timervalue), url)
			case "interval":
				value, err := strconv.Atoi(cmd[1])
				if err != nil {
					return
				}
				timervalue = value
			case "url":
				url = cmd[1]
			case "stop":
				worker <- "stop message"
			}
		}
	}
}
