import httpx

URL = "new-housing-portal.chall.lac.tf:443"

class BaseAPI:
    def __init__(self, url=URL) -> None:
        self.c = httpx.Client(base_url=url)
class API(BaseAPI):
    ...

if __name__ == "__main__":
    api = API()