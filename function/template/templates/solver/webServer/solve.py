import asyncio
import httpx
from pyngrok import ngrok
from flask import Flask
from threading import Thread

PORT = 6666
TUNNEL = ngrok.connect(PORT, "http").public_url

print("TUNNEL:", TUNNEL)

URL = "https://94.237.48.219:41733/"

class BaseAPI:
    def __init__(self, url=URL) -> None:
        self.c = httpx.AsyncClient(base_url=url, verify=False)
        self.session = ""

class API(BaseAPI):
    ...

def webServer():
    app = Flask(__name__)
    @app.get("/")
    def home():
        return "ok"
    return Thread(target=app.run, args=('0.0.0.0', PORT))



async def main():
    api = API()
    server = webServer()
    server.start()
    server.join()

if __name__ == "__main__":
    asyncio.run(main())
