package main

import "fmt"
import "strconv"
import "github.com/gocolly/colly"

// SetRetry 重试在错误的请求
func SetRetry(c *colly.Collector) {
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
}
