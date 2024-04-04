install:
	go build -o ~/go/bin/ctfify
install-windows:
	go build -o $env:USERPROFILE\go\bin\ctfify.exe