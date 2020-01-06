package main

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis/v7"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/debug"
	"github.com/gocolly/colly/extensions"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// Conf 配置结构
type Conf struct {
	Path  string `yaml:"path"`
	Redis string `yaml:"redis"`
	Proxy bool   `yaml:"proxy"`
	Num   int    `yaml:"num"`
	Debug bool   `yaml:"debug"`
	Log   string `yaml:"log"`
	Retry bool   `yaml:"retry"`
}

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
	URL  string `json:"url"`
	Data string `json:"data"`
}

// Downloader 结构
type Downloader struct {
	conf   Conf
	client *redis.Client
	log    *log.Logger
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

func (d Downloader) download(seeds []Seed) {
	randomProxySwitcher := func(req *http.Request) (*url.URL, error) {
		return d.randomProxySwitcher(req)
	}
	c := colly.NewCollector(
		colly.Async(true),
		colly.AllowURLRevisit(),
	)

	if d.conf.Debug {
		c = colly.NewCollector(
			colly.Debugger(&debug.LogDebugger{}),
			colly.Async(true),
			colly.AllowURLRevisit(),
		)
		log.SetLevel(log.DebugLevel)
	}

	c.OnResponse(func(r *colly.Response) {
		d.log.Info(r.StatusCode, r.Request.URL, r.Ctx.Get("url"))
		reqURL := r.Request.URL.String()
		if isServer(reqURL) {
			d.log.Debug(reqURL, "是请求本地地址!")
			return
		}
		if r.StatusCode != 200 {
			d.log.Debug(r.StatusCode, "返回状态码不对!")
			return
		}
		if r.Request.URL.String() != r.Ctx.Get("url") {
			d.log.Debug(r.Request.URL.String(), r.Ctx.Get("url"), "请求地址发生变化")
			return
		}
		filename := genFilename(reqURL)
		// 进行异步写文件
		go func() {
			extraHTML := fmt.Sprintf("\nEND\nSEEDINFO\n %s \nSEEDINFO", r.Ctx.Get("data"))
			err := ioutil.WriteFile(
				fmt.Sprintf("%s/%s", d.conf.Path, filename),
				append(r.Body[:], []byte(extraHTML)...),
				0644)
			if err != nil {
				log.Debug(err)
				return
			}
			params := url.Values{}
			params.Add("filepath", d.conf.Path)
			params.Add("filename", filename)
			params.Add("url", reqURL)
			params.Add("data", r.Ctx.Get("data"))
			c.Visit(notifyPath + params.Encode())
		}()
	})
	// Set error handler
	c.OnError(func(r *colly.Response, err error) {
		d.log.Info("Request URL:", r.Request.URL, "\nError:", err)
	})
	c.OnRequest(func(r *colly.Request) {
		d.log.Info("OnRequest")
		if isServer(r.URL.String()) {
			return
		}
		m, _ := url.ParseQuery(r.URL.RawQuery)
		if r.Ctx.Get("url") == "" {
			r.Ctx.Put("data", m["data"][0])
			r.URL.RawQuery = ""
			params := url.Values{}
			for k, v := range m {
				if k != "data" {
					params.Add(k, v[0])
				}
			}
			r.URL.RawQuery = params.Encode()
			r.Ctx.Put("url", r.URL.String())
		}
	})
	c.RedirectHandler = func(req *http.Request, via []*http.Request) error {
		d.log.Info("redirect")
		return errors.New("不能重定向")
	}
	if d.conf.Proxy {
		d.log.Info("使用代理！")
		c.SetProxyFunc(randomProxySwitcher)
	}
	c.SetRequestTimeout(time.Duration(10) * time.Second)
	extensions.RandomUserAgent(c)
	if d.conf.Retry {
		SetRetry(c)
	}
	ESFHandle(c)
	// c.Limit(&colly.LimitRule{
	// 	DomainGlob:  "*",
	// 	Parallelism: 8,
	// 	RandomDelay: time.Second,
	// })
	// create a request queue with 2 consumer threads
	for _, seed := range seeds {
		u, err := url.Parse(seed.URL)
		if err != nil {
			continue
		}
		params := url.Values{}
		params.Add("data", seed.Data)

		m, err := url.ParseQuery(u.RawQuery)
		if err != nil {
			continue
		}
		for k, v := range m {
			params.Add(k, v[0])
		}
		u.RawQuery = ""
		c.Visit(u.String() + "?" + params.Encode())
	}
	c.Wait()
}

// NewDownloader 初始化downloader
func NewDownloader(confPath string) Downloader {
	var downloader Downloader
	data, err := ioutil.ReadFile(confPath)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	err = yaml.Unmarshal(data, &downloader.conf)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	log.Println("读取配置为:")
	log.Println(downloader.conf)

	downloader.client = redis.NewClient(&redis.Options{
		Addr:     downloader.conf.Redis,
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	downloader.log = log.New()
	// You could set this to any `io.Writer` such as a file
	file, err := os.OpenFile(downloader.conf.Log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		downloader.log.Out = file
	} else {
		log.Info("Failed to log to file, using default stderr")
	}
	return downloader
}

func (d Downloader) run() {
	for {
		seeds := d.getSeeds(d.conf.Num)
		d.log.Printf("从队列中取出种子数量 %d", len(seeds))
		if len(seeds) > 0 {
			start := time.Now()
			d.download(seeds)
			end := time.Now()
			elapsed := end.Sub(start)
			d.log.Info(fmt.Sprintf("种子数量%d, 总共花费 %v下载!", len(seeds), elapsed))
		} else {
			time.Sleep(time.Duration(3) * time.Second)
		}
	}
}

func (d Downloader) getSeeds(num int) []Seed {
	seeds := make([]Seed, 0)
	for i := 0; i < num; i++ {
		v, err := d.client.LPop("GoDownloader:start_urls").Result()
		if err != nil {
			break
		}
		var seed Seed
		err = json.Unmarshal([]byte(v), &seed)
		if err != nil {
			break
		}
		seeds = append(seeds, seed)
	}
	return seeds
}
