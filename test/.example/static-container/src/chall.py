FLAG = open('flag.txt').read().strip().lstrip('TCF{').rstrip("}")

if __name__ == '__main__':
    print(FLAG)
