package main

import (
	"log"
	"time"
	"github.com/go-redis/redis/v7"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/debug"
	"github.com/gocolly/colly/extensions"
)

func main() {
	c := colly.NewCollector(
		colly.Debugger(&debug.LogDebugger{}),
		colly.Async(true),
	)
	extensions.RandomUserAgent(c)

	c.Limit(&colly.LimitRule{
		DomainGlob:  "*httpbin.*",
		Parallelism: 2,
		RandomDelay: time.Second,
	})

	c.OnResponse(func(r *colly.Response) {
		log.Println(string(r.Body))
	})

	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	for {
        v,err:= client.LPop("GODownloader:start_urls").Result()
        if err != nil{
            break
        }
		c.Visit(v)
	}
	c.Wait()
}
