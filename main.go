package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
	"github.com/go-redis/redis/v7"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/debug"
	"github.com/gocolly/colly/extensions"
)

// PATH 下载地址
var PATH = "/home/lin/work/test/"

// REDISHOST 地址
var REDISHOST = "localhost:6379"

// CLIENT redis 客户端
var CLIENT = redis.NewClient(&redis.Options{
	Addr:     REDISHOST,
	Password: "", // no password set
	DB:       0,  // use default DB
})

var urlExtra = make(map[string]string)

func randomProxySwitcher(_ *http.Request) (*url.URL, error) {
	host, err := CLIENT.SRandMember("GZYF_Test:Proxy_Pool:H").Result()
	if err != nil {
		return &url.URL{Host: "10.30.1.18:3128"}, nil
	}
	return &url.URL{Host: host}, nil
}

func genFilename(url string) string {
    h := md5.New()
    io.WriteString(h, url)
    io.WriteString(h, time.Now().String())
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Seed 种子格式
type Seed struct {
	URL  string `json:url`
	Data string `json:data`
}

func main() {
	c := colly.NewCollector(
		colly.Debugger(&debug.LogDebugger{}),
		colly.Async(true),
	)
	extensions.RandomUserAgent(c)

	c.SetProxyFunc(randomProxySwitcher)
	c.OnResponse(func(r *colly.Response) {
		fmt.Printf(r.Request.URL.String())
		fmt.Println(genFilename(r.Request.URL.String()))

		ioutil.WriteFile(
			fmt.Sprintf("%s/%s.html", PATH, genFilename(r.Request.URL.String())),
			append(r.Body[:], []byte(urlExtra[r.Request.URL.String()])...),
			0644)
	})
	// Set error handler
	c.OnError(func(r *colly.Response, err error) {
		fmt.Println("Request URL:", r.Request.URL, "failed with response:", r, "\nError:", err)
	})

	hostSet := make(map[string]bool)
	for {
		v, err := CLIENT.LPop("GoDownloader:start_urls").Result()
		if err != nil {
			time.Sleep(time.Duration(2) * time.Second)
			continue
		}
		var seed Seed
		json.Unmarshal([]byte(v), &seed)
		urlExtra[seed.URL] = seed.Data
		u, err := url.Parse(seed.URL)
		if err != nil {
			continue
		}
		// TODO 没用通用规则，只能这样!!!
		_, ok := hostSet[u.Host]
		if !ok {
			c.Limit(&colly.LimitRule{
				DomainGlob:  fmt.Sprintf("*%s*", u.Host),
				Parallelism: 2,
				RandomDelay: time.Second,
			})
			hostSet[u.Host] = true
		}
		c.Visit(seed.URL)
	}
}
