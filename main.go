package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/debug"
    "github.com/gocolly/colly/extensions"
    "github.com/go-redis/redis/v7"
)

// PATH 下载地址
var PATH = "/home/lin/test/"

func randomProxySwitcher(_ *http.Request) (*url.URL, error) {
	return &url.URL{Host: "10.30.1.18:3128"}, nil
}

func main() {
	c := colly.NewCollector(
		colly.Debugger(&debug.LogDebugger{}),
		colly.Async(true),
	)
	extensions.RandomUserAgent(c)

	c.SetProxyFunc(randomProxySwitcher)
	c.OnResponse(func(r *colly.Response) {
		ioutil.WriteFile(PATH+"/a.html", r.Body, 0644)
	})
	// Set error handler
	c.OnError(func(r *colly.Response, err error) {
		fmt.Println("Request URL:", r.Request.URL, "failed with response:", r, "\nError:", err)
	})

	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
    hostSet := make(map[string]bool)
	for {
		v, err := client.LPop("GODownloader:start_urls").Result()
		if err != nil {
			time.Sleep(time.Duration(2) * time.Second)
			continue
		}
		u, err := url.Parse(v)
		if err != nil {
			continue
        }
        // TODO 没用通用规则，只能这样!!!
        _, ok := hostSet[u.Host]
        if !ok{
            c.Limit(&colly.LimitRule{
                DomainGlob:  fmt.Sprintf("*%s*", u.Host),
                Parallelism: 2,
                RandomDelay: time.Second,
            })
            hostSet[u.Host] = true
        }
		c.Visit(v)
	}
}
