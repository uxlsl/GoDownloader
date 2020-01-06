# 种子下载器 golang实现

## 目的
实现高性能下载

## 运行

```

export GOPROXY=https://goproxy.cn
go run main.go ./dev.yaml|./prod.yaml

```

```

env FLASK_APP=api.py flask run --port 9015

```
or 
```

gunicorn -w 4 -b 127.0.0.1:9015 api:app

```

## 思考

+ 怎样调用私有方法，通过继承!

## TODO

+ 统计速度


## 参考
https://github.com/goproxy/goproxy.cn/blob/master/README.zh-CN.md
