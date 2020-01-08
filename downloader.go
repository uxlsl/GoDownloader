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
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v7"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/debug"
	"github.com/gocolly/colly/extensions"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// Conf 配置结构
type Conf struct {
	Path       string `yaml:"path"`
	Redis      string `yaml:"redis"`
	Proxy      bool   `yaml:"proxy"`
	Num        int    `yaml:"num"`
	Debug      bool   `yaml:"debug"`
	Log        string `yaml:"log"`
	Retry      bool   `yaml:"retry"`
	RetryTimes int    `yaml:"retry_times"`
	SeedKey    string `yaml:"SeedKey"`
	Limit      bool   `yaml:"limit"`
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
	URL   string `json:"url"`
	Data  string `json:"data"`
	Check string // 用来检查下载的是否有效
}

// Downloader 结构
type Downloader struct {
	conf      Conf
	client    *redis.Client
	log       *log.Logger
	RetrySeed []*colly.Context
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

func (d *Downloader) download(seeds []Seed) {
	randomProxySwitcher := func(req *http.Request) (*url.URL, error) {
		return d.randomProxySwitcher(req)
	}
	RetryFunc := func(r *colly.Response) {
		d.log.Debugf("重试请求 %s", r.Ctx.Get("url"))
		count := r.Ctx.Get("retry_times")
		if count == "" {
			r.Ctx.Put("retry_times", "1")
			d.RetrySeed = append(d.RetrySeed, r.Ctx)
		} else {
			c, err := strconv.Atoi(count)
			if err != nil {
				return
			}
			if c <= d.conf.RetryTimes {
				r.Ctx.Put("retry_times", strconv.FormatInt(int64(c)+1, 10))
				d.RetrySeed = append(d.RetrySeed, r.Ctx)
			}
		}
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
		d.log.SetLevel(log.DebugLevel)
	}

	c.OnResponse(func(r *colly.Response) {
		d.log.Debug(r.StatusCode, r.Request.URL, r.Ctx.Get("url"))
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
		if !strings.Contains(string(r.Body), r.Ctx.Get("Check")) {
			RetryFunc(r)
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
	c.RedirectHandler = func(req *http.Request, via []*http.Request) error {
		d.log.Debug("redirect")
		return errors.New("不能重定向")
	}
	if d.conf.Proxy {
		d.log.Info("使用代理！")
		c.SetProxyFunc(randomProxySwitcher)
	}
	c.SetRequestTimeout(time.Duration(10) * time.Second)
	extensions.RandomUserAgent(c)
	if d.conf.Retry {
		// Set error handler
		c.OnError(func(r *colly.Response, err error) {
			d.log.Debug("Request URL:", r.Request.URL, "\nError:", err)
			RetryFunc(r)
		})
	}
	ESFHandle(d, c)
	if d.conf.Limit {
		c.Limit(&colly.LimitRule{
			DomainGlob:  "*",
			Parallelism: 8,
			RandomDelay: time.Second,
		})
	}
	for _, seed := range seeds {
		ctx := colly.NewContext()
		ctx.Put("data", seed.Data)
		ctx.Put("url", seed.URL)
		ctx.Put("retry_times", "0")
		ctx.Put("Check", seed.Check)
		c.Request("GET", seed.URL, nil, ctx, nil)
	}
	RetrySeed := d.RetrySeed
	d.RetrySeed = make([]*colly.Context, 0)
	for _, ctx := range RetrySeed {
		c.Request("GET", ctx.Get("url"), nil, ctx, nil)
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
	logpath, _ := filepath.Abs(downloader.conf.Log)
	logf, err := rotatelogs.New(
		logpath+".%Y%m%d",
		rotatelogs.WithLinkName(logpath))
	if err != nil {
		log.Printf("failed to create rotatelogs: %s", err)
	}
	downloader.log.Out = logf
	return downloader
}

func (d Downloader) run() {
	os.MkdirAll(path.Dir(d.conf.Log), os.ModePerm)
	for {
		var seeds []Seed
		if d.conf.Num > len(d.RetrySeed) {
			seeds = d.getSeeds(d.conf.Num - len(d.RetrySeed))
		} else {
			seeds = make([]Seed, 0)
		}
		d.log.Infof("从队列中取出种子数量 %d,重试种子 %d", len(seeds), len(d.RetrySeed))
		if len(seeds) > 0 || len(d.RetrySeed) > 0 {
			start := time.Now()
			d.download(seeds)
			end := time.Now()
			elapsed := end.Sub(start)
			d.log.Infof("种子数量%d, 重试种子数%d, 总共花费 %v下载!", len(seeds), len(d.RetrySeed),
				elapsed)
		} else {
			time.Sleep(time.Duration(3) * time.Second)
		}
	}
}

func (d Downloader) getSeeds(num int) []Seed {
	type INFO struct {
		Check string `json:"detail_available_check"`
	}
	type T struct {
		SourceURL string `json:"source_url"`
		Info      INFO
	}
	seeds := make([]Seed, 0)
	for i := 0; i < num; i++ {
		v, err := d.client.LPop(d.conf.SeedKey).Result()
		if err != nil {
			break
		}
		var t T
		err = json.Unmarshal([]byte(v), &t)
		if err != nil {
			break
		}
		seed := Seed{URL: t.SourceURL, Data: v, Check: t.Info.Check}
		seeds = append(seeds, seed)
	}
	return seeds
}
