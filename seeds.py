import json
import time
import redis


class SeedWorker(object):
    def __init__(self, conn, from_key, to_key):
        self.__conn = conn
        self.__from_key = from_key
        self.__to_key = to_key

    def run(self):
        while True:
            v = self.__conn.lpop(self.__from_key)
            if v is None:
                time.sleep(1)
                continue
            v = json.loads(v)
            seed = {
                "url": v['source_url'],
                "data": json.dumps(v, ensure_ascii=False)
            }
            self.__conn.rpush(self.__to_key, json.dumps(seed))


if __name__ == '__main__':
    SeedWorker(redis.Redis('10.30.1.20'), 'foobar',
               'GoDownloader:start_urls').run()
