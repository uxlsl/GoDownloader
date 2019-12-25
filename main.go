package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-redis/redis/v7"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/debug"
	"github.com/gocolly/colly/extensions"
)

// PATH 下载地址
var PATH = "/yf/Downloads"

// REDISHOST 地址
var REDISHOST = "10.30.1.20:6379"

// CLIENT redis 客户端
var CLIENT = redis.NewClient(&redis.Options{
	Addr:     REDISHOST,
	Password: "", // no password set
	DB:       0,  // use default DB
})

var urlExtra = make(map[string]string)

// 下载文件完成,通知的服务地址
var notifyPath = "http://localhost:9015/notify?"

func isServer(url string) bool {
	if strings.Contains(url, notifyPath) {
		return true
	}
	return false
}
func randomProxySwitcher(req *http.Request) (*url.URL, error) {
	if isServer(req.URL.String()) {
		return nil, nil
	}
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
	return fmt.Sprintf("%x.html", h.Sum(nil))
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
		colly.AllowURLRevisit(),
	)
	extensions.RandomUserAgent(c)
	//c.SetProxyFunc(randomProxySwitcher)
	c.OnResponse(func(r *colly.Response) {
		reqURL := r.Request.URL.String()
		if isServer(reqURL) {
			return
		}
		if r.StatusCode != 200 {
			fmt.Println("返回状态码不对!")
			return
		}
		filename := genFilename(reqURL)

		err := ioutil.WriteFile(
			fmt.Sprintf("%s/%s", PATH, filename),
			append(r.Body[:], []byte(
				fmt.Sprintf("\nEND\nSEEDINFO\n %s \nSEEDINFO", urlExtra[r.Request.URL.String()]))...),
			0644)
		if err != nil {
			fmt.Println(err)
			return
		}
		params := url.Values{}
		params.Add("filepath", PATH)
		params.Add("filename", filename)
		params.Add("url", reqURL)
		c.Visit(notifyPath + params.Encode())
	})
	// Set error handler
	c.OnError(func(r *colly.Response, err error) {
		fmt.Println("Request URL:", r.Request.URL, "\nError:", err)
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
		l := len(strings.Split(u.Host, "."))
		key := strings.Join(
			strings.Split(u.Host, ".")[l-2:], ".")
		_, ok := hostSet[key]
		if !ok {
			c.Limit(&colly.LimitRule{
				DomainGlob:  fmt.Sprintf("*%s*", u.Host),
				Parallelism: 2,
				RandomDelay: time.Second,
			})
			hostSet[key] = true
		}
		c.Visit(seed.URL)
		// TODO 不能一下子把所有url放进去!
		time.Sleep(time.Duration(1) * time.Microsecond)
	}
}
