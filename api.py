import time
from celery import Celery
from flask import Flask, escape, request

app = Flask(__name__)
celery = Celery(
            broker="redis://10.30.1.18/2", 
            backend="redis://10.30.1.18/3")

@app.route('/notify')
def hello():
    file_name = request.args.get("filename")
    file_path = request.args.get("filepath")
    url = request.args.get("url")
    data = request.args.get("data")
    celery.send_task(
                "anjuke_upload_service.upload_service.upload_new_file",
                args=[
                    "10.30.1.18", "30168", "root", "xxxx", 
                    file_path,
                    file_name,
                    int(time.time()), 
                    data,
                    url
                ],
                queue='upload_queue_anjuke')
    return 'ok'


if __name__ == '__main__':
    from gevent.pywsgi import WSGIServer
    http_server = WSGIServer(('', 9015), app)
    http_server.serve_forever()