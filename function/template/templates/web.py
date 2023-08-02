import httpx
from urllib.parse import urljoin

URL = ""


class API:
    def __init__(self, url=URL) -> None:
        self.url = url
        self.c = httpx.Client()

    def join(s, path):
        return httpx.URL(s).join(path)


if __name__ == "__main__":
    api = API()
