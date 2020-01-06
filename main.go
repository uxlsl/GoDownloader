package main

import (
	"os"

	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetFormatter(&log.TextFormatter{
		DisableColors: true,
		FullTimestamp: true,
	})
}

func main() {
	if len(os.Args) == 1 {
		log.Fatalf("请提供配置文件参数")
	}
	downloader := NewDownloader(os.Args[1])
	downloader.run()
}
