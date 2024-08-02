import httpx
import asyncio

URL = "http://localhost:80"

class BaseAPI:
    def __init__(self, url=URL) -> None:
        self.c = httpx.AsyncClient(base_url=url)

class API(BaseAPI):
    ...

async def main():
    api = API()

if __name__ == "__main__":
    asyncio.run(main())
