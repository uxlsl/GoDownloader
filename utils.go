package main

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
)

// ESFHandle 房天下网站处理
func ESFHandle(c *colly.Collector) {
	old := c.RedirectHandler
	new := func(req *http.Request, via []*http.Request) error {
		if strings.Contains(req.URL.String(), "fang.com") {
			return nil
		}
		return old(req, via)
	}
	c.RedirectHandler = new

	c.OnResponse(func(r *colly.Response) {
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(r.Body))
		if err != nil {
			return
		}
		url, exists := doc.Find(".btn-redir").First().Attr("href")
		if !exists {
			return
		}
		urlExtra[url] = urlExtra[r.Request.URL.String()]
		delete(urlExtra, r.Request.URL.String())
		c.Visit(url)
	})
}
