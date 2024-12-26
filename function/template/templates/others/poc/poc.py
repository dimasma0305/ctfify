import httpx
import asyncio

URL = "http://localhost:80"

class BaseAPI:
    def __init__(self, url=URL) -> None:
        self.c = httpx.AsyncClient(base_url=url)

    def wp_login(self, username: str, password: str) -> None:
        return self.c.post("/wp-login.php", data={
            "log": username,
            "pwd": password
        })

class API(BaseAPI):
    ...

async def main():
    api = API()
    username = "Subscriber"
    password = "Subscriber"
    res = await api.wp_login(username, password)
    print(res)

if __name__ == "__main__":
    asyncio.run(main())
