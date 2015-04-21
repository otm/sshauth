build:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o sshauth-linux-amd64 main.go
	GOOS=linux GOARCH=386 CGO_ENABLED=0 go build -o sshauth-linux-386 main.go

clean:
	rm sshauth-linux-386
	rm sshauth-linux-amd64
