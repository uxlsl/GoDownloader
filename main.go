package main

import (
    "time"
    "io/ioutil"
    "net/http"
    "net/url"
	"github.com/go-redis/redis/v7"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/debug"
	"github.com/gocolly/colly/extensions"
)

func randomProxySwitcher(_ *http.Request) (*url.URL, error) {
    return &url.URL{Host: "10.30.1.18:3128"}, nil
}

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
    c.SetProxyFunc(randomProxySwitcher)
	c.OnResponse(func(r *colly.Response) {
        ioutil.WriteFile("a.html",r.Body, 0644)
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
