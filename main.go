package main

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v7"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/debug"
	"github.com/gocolly/colly/extensions"
	"gopkg.in/yaml.v2"
)

// 配置结构
type Conf struct {
	Path  string `yaml:path`
	Redis string `yaml:redis`
}

var urlExtra = make(map[string]string)

// 下载文件完成,通知的服务地址
var notifyPath = "http://localhost:9015/notify?"

func isServer(url string) bool {
	if strings.Contains(url, notifyPath) {
		return true
	}
	return false
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
// Downloader 结构
type Downloader struct {
	conf   Conf
	client *redis.Client
}

func (d Downloader) randomProxySwitcher(req *http.Request) (*url.URL, error) {
	if isServer(req.URL.String()) {
		return nil, nil
	}
	host, err := d.client.SRandMember("GZYF_Test:Proxy_Pool:H").Result()
	if err != nil {
		return &url.URL{Host: "10.30.1.18:3128"}, nil
	}
	return &url.URL{Host: host}, nil
}

func (d Downloader) download(urls []string) {
	randomProxySwitcher := func(req *http.Request) (*url.URL, error) {
		return d.randomProxySwitcher(req)
	}
	c := colly.NewCollector(
		colly.Debugger(&debug.LogDebugger{}),
		colly.Async(true),
		colly.AllowURLRevisit(),
	)
	c.SetRequestTimeout(time.Duration(30) * time.Second)
	extensions.RandomUserAgent(c)

	c.SetProxyFunc(randomProxySwitcher)
	c.OnResponse(func(r *colly.Response) {
		fmt.Println(r.StatusCode, r.Request.URL)
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
			fmt.Sprintf("%s/%s", d.conf.Path, filename),
			append(r.Body[:], []byte(
				fmt.Sprintf("\nEND\nSEEDINFO\n %s \nSEEDINFO", urlExtra[r.Request.URL.String()]))...),
			0644)
		delete(urlExtra, r.Request.URL.String())
		if err != nil {
			fmt.Println(err)
			return
		}
		params := url.Values{}
		params.Add("filepath", d.conf.Path)
		params.Add("filename", filename)
		params.Add("url", reqURL)
		c.Visit(notifyPath + params.Encode())
	})
	// Set error handler
	c.OnError(func(r *colly.Response, err error) {
		fmt.Println("Request URL:", r.Request.URL, "\nError:", err)
		count := r.Ctx.Get("retry_times")
		if count == "" {
			r.Ctx.Put("retry_times", "1")
			r.Request.Retry()
		} else {
			c, err := strconv.Atoi(count)
			if err != nil {
				return
			}
			if c < 3 {
				r.Ctx.Put("retry_times", string(c+1))
				r.Request.Retry()
			}
		}
	})
	c.OnRequest(func(r *colly.Request) {
		fmt.Println("OnRequest")
		r.Ctx.Put("url", r.URL.String())
	})
	c.RedirectHandler = func(req *http.Request, via []*http.Request) error {
		fmt.Println("redirect")
		return errors.New("不能重定向")
	}
	hostSet := make(map[string]bool)
	for _, v := range urls {
		var seed Seed
		json.Unmarshal([]byte(v), &seed)
		urlExtra[seed.URL] = seed.Data
		u, err := url.Parse(seed.URL)
		if err != nil {
			continue
		}
		// TODO 没用通用规则，只能这样!!!
		// l := len(strings.Split(u.Host, "."))
		// key := strings.Join(
		// 	strings.Split(u.Host, ".")[l-2:], ".")
		key := u.Host
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
	}
	c.Wait()
}
// NewDownloader 初始化downloader
func NewDownloader(confPath string) Downloader{
	var downloader Downloader
	data, err := ioutil.ReadFile(confPath)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	err = yaml.Unmarshal(data, &downloader.conf)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	fmt.Println("读取配置为:")
	fmt.Println(downloader.conf)

	downloader.client = redis.NewClient(&redis.Options{
		Addr:     downloader.conf.Redis,
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	return downloader
}

func (d Downloader) run() {
	for {
		urls := d.getUrls(1000)
		fmt.Printf("从队列中取出url数量 %d\n", len(urls))
		if len(urls) > 0 {
			d.download(urls)
		} else {
			time.Sleep(time.Duration(3) * time.Second)
		}
	}
}

func(d Downloader) getUrls(num int)[]string{
	urls := make([]string, 0)
	for i := 0; i < num; i++ {
		v, err := d.client.LPop("GoDownloader:start_urls").Result()
		if err != nil {
			break
		}
		urls = append(urls, v)
	}
	return urls
}


func main() {
	if len(os.Args) == 1{
		log.Fatalf("请提供配置文件参数")
	}
	downloader := NewDownloader(os.Args[1])
	downloader.run()
} 