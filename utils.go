package main

import (
	"bytes"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
)

// ESFHandle 房天下网站处理
func ESFHandle(d *Downloader, c *colly.Collector) {
	old := c.RedirectHandler
	new := func(req *http.Request, via []*http.Request) error {
		if strings.Contains(req.URL.String(), "fang.com") {
			return nil
		}
		return old(req, via)
	}
	c.RedirectHandler = new

	c.OnResponse(func(r *colly.Response) {
		if !strings.Contains(r.Request.URL.String(), "search.fang.com/captcha") {
			return
		}
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(r.Body))
		if err != nil {
			return
		}
		esf, exists := doc.Find(".btn-redir").First().Attr("href")
		if !exists {
			return
		}
		u, err := url.Parse(esf)
		if err != nil {
			return
		}
		r.Request.URL = u
		r.Ctx.Put("url", esf)
		d.RetrySeed = append(d.RetrySeed, r.Ctx)
	})
}
