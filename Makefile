project=auth

# build the source to native OS and platform
.PHONY: build
build:
	go build -ldflags '-extldflags "-static"' -o ${project} ./example/main.go

# go update libraries
.PHONY: update
update:
	go get -u ./...
	go mod tidy

# clean all the binary and the generated code
# note: leave restapi/configure_telesales_admin.go untouch as it is supposed to be customized by user
.PHONY: clean
clean:
	rm -f ${project}
