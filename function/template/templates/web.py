import httpx
from pwn import *

URL = "http://localhost:80"

context.log_level = logging.DEBUG


class BaseAPI:
    def __init__(self, url=URL) -> None:
        self.c = httpx.Client(base_url=url)


class UtilsAPI(BaseAPI):
    def make_raw_request(self, method, path, **kwargs):
        request = self.c.build_request(method, path, **kwargs)
        raw_request = self._format_raw_request(request)
        return raw_request

    def _format_raw_request(self, request: httpx.Request):
        if query := request.url.query:
            query = b"?"+query
        request_str = f"{request.method} {request.url.path}{query.decode()} HTTP/1.1\r\n"
        for name, value in request.headers.items():
            request_str += f"{name}: {value}\r\n"
        request_str += "\r\n"
        request_byte = request_str.encode()
        if request.content:
            request_byte += request.content
        return request_byte

    def receive_chunks(s, sock: remote):
        chunks = []
        while True:
            chunk_size_str = b""
            while chunk_size_str == b"":
                while True:
                    char = sock.recv(1)
                    if char == b"\r":
                        continue
                    if char == b"\n":
                        break
                    chunk_size_str += char
            chunk_size = int(chunk_size_str, 16)
            if chunk_size == 0:
                break
            chunk_data = b""
            while chunk_size > 0:
                data = sock.recv(chunk_size)
                chunk_data += data
                chunk_size -= len(data)
            chunks.append(chunk_data)
        return b"".join(chunks)


class API(UtilsAPI):
    ...


if __name__ == "__main__":
    api = API()
