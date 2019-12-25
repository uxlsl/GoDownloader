import time
from celery import Celery
from flask import Flask, escape, request

app = Flask(__name__)
celery = Celery(
            broker="redis://10.30.1.19/2", backend="redis://10.30.1.19/3")

@app.route('/notify')
def hello():
    file_name = request.args.get("filename")
    file_path = request.args.get("filepath")
    url = request.args.get("url")
    celery.send_task(
                "anjuke_upload_service.upload_service.upload_new_file",
                args=[
                    "10.30.1.18", "30168", "root", "xxxx", 
                    file_path,
                    file_name,
                    int(time.time()), 
                    "GoDownloader:start_urls", 
                    url
                ],
                queue='upload_queue_anjuke')
    return 'ok'