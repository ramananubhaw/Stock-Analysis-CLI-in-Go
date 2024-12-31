package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	// "sync"
)

type Stock struct {
	Ticker string
	Gap float64
	OpeningPrice float64
}

func Load(path string) ([]Stock, error) {
	file, err := os.Open(path)
	if (err != nil) {
		fmt.Println(err)
		return nil, err
	}
	
	defer file.Close() // always close the file before ending execution in case of any error in the program ahead
	
	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if (err != nil) {
		fmt.Println(err)
		return nil, err
	}

	rows = slices.Delete(rows, 0, 1)
	
	var stocks []Stock
	
	for _, row := range rows {
		ticker := row[0]
		gap, err := strconv.ParseFloat(row[1], 64)
		if (err!=nil) {
			continue
		}
		openingPrice, err := strconv.ParseFloat(row[2], 64)
		if (err!=nil) {
			continue
		}
		stocks = append(stocks, Stock{
			Ticker: ticker,
			Gap: gap,
			OpeningPrice: openingPrice,
		})
	}
	
	return stocks, nil
}

var accountBalance float64 = 10000.0 // balance in account
var lossTolerance float64 = 0.2 // percentage of loss that can be tolerated
var maxLossPerTrade = accountBalance * lossTolerance // maximum amount of loss that can be tolerated
var profitPercent float64 = 0.8 // percentage of gap I want to take as profit

type Position struct {
	EntryPrice float64 // price at which to buy/sell
	Shares int // no. of shares to buy/sell
	TakeProfitPrice float64 // price at which to exit and book profit
	StopLossPrice float64 // price at which to stop my loss if stock doesn't go my way
	Profit float64 // expected final profit
}

func Calculate(gapPercent, openingPrice float64) Position {
	closingPrice := openingPrice / (1 + gapPercent)
	gapValue := closingPrice - openingPrice
	profitFromGap := profitPercent * gapValue

	stopLoss := openingPrice - profitFromGap
	takeProfit := openingPrice + profitFromGap

	shares := int(maxLossPerTrade / math.Abs(stopLoss - openingPrice))

	profit := math.Abs(openingPrice - takeProfit) * float64(shares)
	profit = math.Round(profit*100) / 100

	return Position{
		EntryPrice: math.Round(openingPrice*100) / 100,
		Shares: shares,
		TakeProfitPrice: math.Round(takeProfit*100) / 100,
		StopLossPrice: math.Round(stopLoss*100) / 100,
		Profit: math.Round(profit*100) / 100,
	}
}

type Selection struct {
	Ticker string
	Position
	Articles []Article
}


var (
	url string
	apiKeyHeader string
	apiKey string
)

type Attributes struct {
	PublishOn time.Time `json:"publishOn"` // to store the 'publishOn' field value from the response data
	Title string `json:"title"` // to store the 'title' field value from the response data
}

type SeekingAlphaNews struct {
	Attributes `json:"attributes"` // to store the 'attributes' field value from the response data
}

type SeekingAlphaResponse struct {
	Data []SeekingAlphaNews `json:"data"` // to store the 'data' field value from the response data
}

type Article struct {
	PublishOn time.Time
	Headline string
}

func FetchNews(ticker string) ([]Article, error) {
	req, err := http.NewRequest(http.MethodGet, url+ticker, nil)
	if (err!=nil) {
		return nil, err
	}
	req.Header.Add(apiKeyHeader, apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if (err!=nil) {
		return nil, err
	}
	if (resp.StatusCode<200 || resp.StatusCode>299) {
		return nil, fmt.Errorf("unsuccessful response code - %v received", resp.StatusCode)
	}
	// response contains 3 fields, data, included and meta

	res := &SeekingAlphaResponse{}
	json.NewDecoder(resp.Body).Decode(res) // decode JSON into Go type and store into 'res'

	var articles []Article

	for _, item := range res.Data {
		art := Article{
			PublishOn: item.Attributes.PublishOn,
			Headline: item.Attributes.Title,
		}
		articles = append(articles, art)
	}

	return articles, nil
}

func Deliver(filePath string, selections []Selection) error {
	file, err := os.Create(filePath)
	if (err!=nil) {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file) // encode Go type into JSON
	err = encoder.Encode(selections)
	if (err!=nil) {
		return fmt.Errorf("error encoding selections: %v", err)
	}
	return nil
}

func main() {

	godotenv.Load()

	stocks, err := Load("./opg.csv")
	if (err!=nil) {
		fmt.Println(err)
		return
	}

	// filter out unworthy stocks - stocks with difference less than 10%

	stocks = slices.DeleteFunc(stocks, func(s Stock) bool {
		return math.Abs(s.Gap) < 0.1
	})

	url = os.Getenv("SEEKING_ALPHA_URL")
	apiKeyHeader = os.Getenv("API_KEY_HEADER")
	apiKey = os.Getenv("API_KEY")

	var selections []Selection

	// var wg sync.WaitGroup

	selectionChan := make(chan Selection, len(stocks))
	for _, stock := range stocks {
		// wg.Add(1)
		go func(s Stock, selected chan<-Selection) {
			// defer wg.Done()
			position := Calculate(s.Gap, s.OpeningPrice)
			articles, err := FetchNews(s.Ticker)
			if (err!=nil) {
				fmt.Printf("error loading news about %v, %v\n", s.Ticker, err)
			}
			fmt.Printf("Found %d articles about %v\n", len(articles), s.Ticker)
			sel := Selection{
				Ticker: s.Ticker,
				Position: position,
				Articles: articles,
			}
			selected<-sel
			// selections = append(selections, sel)
		} (stock, selectionChan) // calling the above anonymous function on 'stock'
	}

	// wg.Wait()

	for sel := range selectionChan {
		selections = append(selections, sel)
		if (len(selections)==len(stocks)) {
			close(selectionChan)
		}
	}

	outputPath := "./opg.json"
	err = Deliver(outputPath, selections)
	if (err!=nil) {
		fmt.Printf("Error writing output: %v\n", err)
		return
	}
	fmt.Printf("Finished writing output to %v\n", outputPath)

}